package provider

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	OAuthClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	OAuthAuthURL         = "https://auth.openai.com/oauth/authorize"
	OAuthTokenURL        = "https://auth.openai.com/oauth/token"
	OAuthRedirectURI     = "http://localhost:1455/auth/callback"
	OAuthScope           = "openid profile email offline_access"
	OAuthDeviceCodeURL   = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	OAuthDeviceTokenURL  = "https://auth.openai.com/api/accounts/deviceauth/token"
	OAuthDeviceVerifyURL = "https://auth.openai.com/codex/device"
)

const (
	successHTML = `<!DOCTYPE html>
<html><head><title>Authentication Successful</title></head>
<body>
<h2>Authentication Successful</h2>
<p>You can close this tab and return to Tau.</p>
</body></html>`
)

const oauthRefreshBuffer = 5 * time.Minute

// OAuthCredentials holds OAuth tokens and metadata.
type OAuthCredentials struct {
	AccessToken  string
	RefreshToken string
	Expires      int64
	AccountID    string
}

// GeneratePKCE generates a PKCE code verifier and challenge.
// Returns base64url-encoded (no padding) verifier and its SHA-256 challenge.
func GeneratePKCE() (verifier, challenge string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generating random bytes: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(bytes)

	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])

	return verifier, challenge, nil
}

// BuildAuthorizationURL constructs the OpenAI OAuth authorization URL.
func BuildAuthorizationURL(clientID, redirectURI, state, challenge string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", OAuthScope)
	params.Set("state", state)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	params.Set("originator", "tau")

	return OAuthAuthURL + "?" + params.Encode()
}

// StartCallbackServer starts a local HTTP server to receive the OAuth callback.
// Tries the given port, then falls back to port+1, port+2 if in use.
// Returns a channel that receives the authorization code, the actual port used,
// a shutdown function, and any startup error.
func StartCallbackServer(port int, expectedState string) (codeCh chan string, actualPort int, shutdown func(), err error) {
	codeCh = make(chan string, 1)

	var srv *http.Server
	shutdownOnce := sync.Once{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/callback" {
			http.NotFound(w, r)
			return
		}

		state := r.URL.Query().Get("state")
		if state != expectedState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(successHTML))

		select {
		case codeCh <- code:
		default:
		}
	})

	for try := 0; try < 3; try++ {
		addr := fmt.Sprintf(":%d", port+try)
		srv = &http.Server{
			Addr:    addr,
			Handler: handler,
		}

		listenErr := make(chan error, 1)
		go func() {
			listenErr <- srv.ListenAndServe()
		}()

		select {
		case err := <-listenErr:
			if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "bind: address already in use") {
				srv.Close()
				continue
			}
			return nil, 0, nil, fmt.Errorf("starting server: %w", err)
		case <-time.After(100 * time.Millisecond):
			actualPort = port + try
			shutdown = func() {
				shutdownOnce.Do(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					srv.Shutdown(ctx)
				})
			}
			return codeCh, actualPort, shutdown, nil
		}
	}

	return nil, 0, nil, fmt.Errorf("all ports %d-%d are in use", port, port+2)
}

// ExchangeCodeForTokens exchanges an authorization code for OAuth tokens.
func ExchangeCodeForTokens(code, verifier string) (access, refresh string, expires int64, err error) {
	return exchangeCodeForTokensWithURL(code, verifier, OAuthTokenURL)
}

// exchangeCodeForTokensWithURL is the internal implementation that accepts a configurable token URL.
func exchangeCodeForTokensWithURL(code, verifier, tokenURL string) (access, refresh string, expires int64, err error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", OAuthRedirectURI)
	form.Set("client_id", OAuthClientID)
	form.Set("code_verifier", verifier)

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return "", "", 0, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body[:n]))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", 0, fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", "", 0, fmt.Errorf("missing access_token in response")
	}

	return tokenResp.AccessToken, tokenResp.RefreshToken, time.Now().Unix() + tokenResp.ExpiresIn, nil
}

// ExtractAccountID extracts the account ID from a JWT token.
// Tries id_token first, then access_token.
// Looks for chatgpt_account_id in nested claim, root level, or organizations[0].id.
func ExtractAccountID(token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("empty token")
	}

	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", fmt.Errorf("decoding JWT payload: %w", err)
		}
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parsing JWT payload: %w", err)
	}

	if id := extractAccountIDFromClaims(claims); id != "" {
		return id, nil
	}

	return "", fmt.Errorf("account_id not found in JWT claims")
}

func extractAccountIDFromClaims(claims map[string]any) string {
	if apiClaim, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if id, ok := apiClaim["chatgpt_account_id"].(string); ok && id != "" {
			return id
		}
	}

	if id, ok := claims["chatgpt_account_id"].(string); ok && id != "" {
		return id
	}

	if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if id, ok := org["id"].(string); ok && id != "" {
				return id
			}
		}
	}

	return ""
}

// BrowserFlow runs the full OAuth browser flow.
// Returns OAuthCredentials on success.
func BrowserFlow() (OAuthCredentials, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("generating PKCE: %w", err)
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return OAuthCredentials{}, fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	authURL := BuildAuthorizationURL(OAuthClientID, OAuthRedirectURI, state, challenge)

	codeCh, actualPort, shutdown, err := StartCallbackServer(1455, state)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("starting callback server: %w", err)
	}
	defer shutdown()

	if actualPort != 1455 {
		authURL = strings.Replace(authURL, "localhost:1455", fmt.Sprintf("localhost:%d", actualPort), 1)
	}

	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case code := <-codeCh:
		access, refresh, expires, err := ExchangeCodeForTokens(code, verifier)
		if err != nil {
			return OAuthCredentials{}, fmt.Errorf("exchanging code for tokens: %w", err)
		}

		accountID, err := ExtractAccountID(access)
		if err != nil {
			return OAuthCredentials{}, fmt.Errorf("extracting account ID: %w", err)
		}

		return OAuthCredentials{
			AccessToken:  access,
			RefreshToken: refresh,
			Expires:      expires,
			AccountID:    accountID,
		}, nil

	case <-ctx.Done():
		return OAuthCredentials{}, fmt.Errorf("authentication timed out")
	}
}

// IsExpired returns true if the access token is expired or will expire within
// the refresh buffer window (5 minutes).
func (c OAuthCredentials) IsExpired() bool {
	if c.Expires == 0 {
		return true
	}
	return time.Now().Add(oauthRefreshBuffer).Unix() >= c.Expires
}

// RefreshTokens exchanges a refresh token for a new access token.
// Returns new access token, refresh token (may be rotated), new expiry, and any error.
func RefreshTokens(refreshToken string) (access, newRefresh string, expires int64, err error) {
	return refreshTokensWithURL(refreshToken, OAuthTokenURL)
}

// refreshTokensWithURL is the internal implementation with configurable token URL.
func refreshTokensWithURL(refreshToken, tokenURL string) (access, newRefresh string, expires int64, err error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", OAuthClientID)

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return "", "", 0, fmt.Errorf("refresh token request failed with status %d: %s", resp.StatusCode, string(body[:n]))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", 0, fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", "", 0, fmt.Errorf("missing access_token in response")
	}

	newRefreshToken := tokenResp.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}

	return tokenResp.AccessToken, newRefreshToken, time.Now().Unix() + tokenResp.ExpiresIn, nil
}

// PersistFunc is a callback that persists updated OAuthCredentials.
type PersistFunc func(creds OAuthCredentials) error

// OAuthManager manages OAuth credentials with thread-safe token refresh.
// It handles concurrent requests safely using a mutex with double-check
// after lock acquisition to prevent redundant refreshes.
type OAuthManager struct {
	mu      sync.Mutex
	creds   OAuthCredentials
	persist PersistFunc
}

// NewOAuthManager creates a new OAuthManager with the given credentials
// and persistence callback.
func NewOAuthManager(creds OAuthCredentials, persist PersistFunc) *OAuthManager {
	return &OAuthManager{
		creds:   creds,
		persist: persist,
	}
}

// Credentials returns a copy of the current credentials.
func (m *OAuthManager) Credentials() OAuthCredentials {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.creds
}

// EnsureValidToken checks if the access token is expired and refreshes it if needed.
// Uses mutex-based locking with double-check after acquisition to handle concurrent
// requests safely. The persistence callback is called after a successful refresh.
func (m *OAuthManager) EnsureValidToken() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.creds.IsExpired() {
		return nil
	}

	access, refresh, expires, err := refreshTokensWithURL(m.creds.RefreshToken, OAuthTokenURL)
	if err != nil {
		return fmt.Errorf("refreshing token: %w", err)
	}

	m.creds.AccessToken = access
	m.creds.RefreshToken = refresh
	m.creds.Expires = expires

	if m.persist != nil {
		if persistErr := m.persist(m.creds); persistErr != nil {
			return fmt.Errorf("persisting refreshed credentials: %w", persistErr)
		}
	}

	return nil
}

// GetAccessToken returns the current access token, refreshing it if expired.
func (m *OAuthManager) GetAccessToken() (string, error) {
	if err := m.EnsureValidToken(); err != nil {
		return "", err
	}
	return m.creds.AccessToken, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
}

// deviceCodeResponse holds the response from the device code initiation endpoint.
type deviceCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	Interval     string `json:"interval"`
	ExpiresIn    int    `json:"expires_in"`
}

// deviceTokenResponse holds the response when the device is authorized.
type deviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

// requestDeviceCode initiates the device authorization flow.
// Returns device_auth_id, user_code, polling interval (seconds), expiry (seconds), and any error.
func requestDeviceCode() (deviceAuthID, userCode string, interval, expiresIn int, err error) {
	return requestDeviceCodeWithURL(OAuthDeviceCodeURL)
}

// requestDeviceCodeWithURL is the internal implementation with configurable URL.
func requestDeviceCodeWithURL(url string) (deviceAuthID, userCode string, interval, expiresIn int, err error) {
	body := map[string]string{"client_id": OAuthClientID}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("marshaling request body: %w", err)
	}

	resp, err := http.Post(url, "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, 0, fmt.Errorf("device code request failed with status %d", resp.StatusCode)
	}

	var dcResp deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcResp); err != nil {
		return "", "", 0, 0, fmt.Errorf("parsing device code response: %w", err)
	}

	if dcResp.DeviceAuthID == "" || dcResp.UserCode == "" {
		return "", "", 0, 0, fmt.Errorf("missing required fields in device code response")
	}

	intervalSecs := 5
	if dcResp.Interval != "" {
		if parsed, err := parseInt(dcResp.Interval); err == nil && parsed > 0 {
			intervalSecs = parsed
		}
	}

	if dcResp.ExpiresIn <= 0 {
		dcResp.ExpiresIn = 900
	}

	return dcResp.DeviceAuthID, dcResp.UserCode, intervalSecs, dcResp.ExpiresIn, nil
}

// pollForDeviceToken polls the device token endpoint until authorized or timeout.
// Returns authorization_code and code_verifier on success.
func pollForDeviceToken(deviceAuthID, userCode string, intervalSecs, expiresInSecs int) (authCode, codeVerifier string, err error) {
	return pollForDeviceTokenWithURL(deviceAuthID, userCode, intervalSecs, expiresInSecs, OAuthDeviceTokenURL)
}

// pollForDeviceTokenWithURL is the internal implementation with configurable URL.
func pollForDeviceTokenWithURL(deviceAuthID, userCode string, intervalSecs, expiresInSecs int, tokenURL string) (authCode, codeVerifier string, err error) {
	deadline := time.Now().Add(time.Duration(expiresInSecs) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(intervalSecs) * time.Second)

		body := map[string]string{
			"device_auth_id": deviceAuthID,
			"user_code":      userCode,
		}
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return "", "", fmt.Errorf("marshaling poll body: %w", err)
		}

		resp, err := http.Post(tokenURL, "application/json", strings.NewReader(string(jsonBody)))
		if err != nil {
			return "", "", fmt.Errorf("poll request failed: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var tokenResp deviceTokenResponse
			if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
				resp.Body.Close()
				return "", "", fmt.Errorf("parsing device token response: %w", err)
			}
			resp.Body.Close()

			if tokenResp.AuthorizationCode == "" || tokenResp.CodeVerifier == "" {
				return "", "", fmt.Errorf("missing required fields in device token response")
			}

			return tokenResp.AuthorizationCode, tokenResp.CodeVerifier, nil
		}

		resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
			return "", "", fmt.Errorf("device token request failed with status %d", resp.StatusCode)
		}
	}

	return "", "", fmt.Errorf("device authorization timed out after %ds", expiresInSecs)
}

// DeviceFlow runs the full device authorization flow.
// Returns OAuthCredentials on success.
func DeviceFlow() (OAuthCredentials, error) {
	deviceAuthID, userCode, intervalSecs, expiresInSecs, err := requestDeviceCode()
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("requesting device code: %w", err)
	}

	authCode, codeVerifier, err := pollForDeviceToken(deviceAuthID, userCode, intervalSecs, expiresInSecs)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("polling for device token: %w", err)
	}

	access, refresh, expires, err := exchangeCodeForTokensWithURL(authCode, codeVerifier, OAuthTokenURL)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("exchanging code for tokens: %w", err)
	}

	accountID, err := ExtractAccountID(access)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("extracting account ID: %w", err)
	}

	return OAuthCredentials{
		AccessToken:  access,
		RefreshToken: refresh,
		Expires:      expires,
		AccountID:    accountID,
	}, nil
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// ParseCallbackInput parses user input from the manual paste flow.
// If the input contains a "?" it is treated as a redirect URL and the "code"
// and "state" query parameters are extracted. Otherwise the entire input is
// treated as a raw authorization code (state will be empty).
// Returns an error if no code can be extracted.
func ParseCallbackInput(input string) (code, state string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("empty input")
	}

	if strings.Contains(input, "?") {
		parsed, parseErr := url.Parse(input)
		if parseErr != nil {
			return "", "", fmt.Errorf("parsing URL: %w", parseErr)
		}
		code = parsed.Query().Get("code")
		state = parsed.Query().Get("state")
		if code == "" {
			return "", "", fmt.Errorf("no authorization code found in URL")
		}
		return code, state, nil
	}

	return input, "", nil
}

// ManualFlow runs the manual paste authorization flow.
// Prints the authorization URL to the provided writer, reads user input from
// the provided reader, and exchanges the code for tokens.
// Returns OAuthCredentials on success.
func ManualFlow(reader io.Reader, writer io.Writer) (OAuthCredentials, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("generating PKCE: %w", err)
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return OAuthCredentials{}, fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	authURL := BuildAuthorizationURL(OAuthClientID, OAuthRedirectURI, state, challenge)

	fmt.Fprintf(writer, "Open the following URL in your browser:\n\n%s\n\n", authURL)
	fmt.Fprintf(writer, "After authorizing, paste the full redirect URL or just the authorization code:\n")

	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return OAuthCredentials{}, fmt.Errorf("reading input: %w", err)
		}
		return OAuthCredentials{}, fmt.Errorf("no input received")
	}
	input := scanner.Text()

	code, _, err := ParseCallbackInput(input)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("parsing input: %w", err)
	}

	access, refresh, expires, err := ExchangeCodeForTokens(code, verifier)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("exchanging code for tokens: %w", err)
	}

	accountID, err := ExtractAccountID(access)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("extracting account ID: %w", err)
	}

	return OAuthCredentials{
		AccessToken:  access,
		RefreshToken: refresh,
		Expires:      expires,
		AccountID:    accountID,
	}, nil
}
