package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(verifier) < 32 {
		t.Fatalf("verifier too short: %d bytes", len(verifier))
	}

	decoded, err := base64.RawURLEncoding.DecodeString(verifier)
	if err != nil {
		t.Fatalf("verifier is not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("verifier decoded length: expected 32, got %d", len(decoded))
	}

	challengeDecoded, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil {
		t.Fatalf("challenge is not valid base64url: %v", err)
	}
	if len(challengeDecoded) != 32 {
		t.Fatalf("challenge decoded length: expected 32, got %d", len(challengeDecoded))
	}

	v2, c2, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if verifier == v2 {
		t.Fatal("two consecutive PKCE generations produced the same verifier")
	}
	if challenge == c2 {
		t.Fatal("two consecutive PKCE generations produced the same challenge")
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	u := BuildAuthorizationURL("test-client", "http://localhost:1455/callback", "test-state", "test-challenge")

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	params := parsed.Query()

	required := map[string]string{
		"response_type":             "code",
		"client_id":                 "test-client",
		"redirect_uri":              "http://localhost:1455/callback",
		"scope":                     OAuthScope,
		"state":                     "test-state",
		"code_challenge":            "test-challenge",
		"code_challenge_method":     "S256",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow": "true",
		"originator":                "tau",
	}

	for key, expected := range required {
		got := params.Get(key)
		if got == "" {
			t.Errorf("URL missing parameter %q", key)
			continue
		}
		if got != expected {
			t.Errorf("URL has wrong value for %q: expected %q, got %q", key, expected, got)
		}
	}

	if !strings.HasPrefix(u, OAuthAuthURL+"?") {
		t.Errorf("URL should start with %s?, got %s", OAuthAuthURL, u)
	}
}

func TestStartCallbackServer(t *testing.T) {
	state := "test-state-12345"

	codeCh, port, shutdown, err := StartCallbackServer(1455, state)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer shutdown()

	if port < 1455 || port > 1457 {
		t.Fatalf("unexpected port: %d", port)
	}

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:1455/auth/callback?state=test-state-12345&code=test-auth-code")
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	select {
	case code := <-codeCh:
		if code != "test-auth-code" {
			t.Fatalf("expected test-auth-code, got %s", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for code")
	}
}

func TestStartCallbackServer_StateMismatch(t *testing.T) {
	state := "expected-state"

	codeCh, _, shutdown, err := StartCallbackServer(14550, state)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer shutdown()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:14550/auth/callback?state=wrong-state&code=some-code")
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for state mismatch, got %d", resp.StatusCode)
	}

	select {
	case <-codeCh:
		t.Fatal("should not receive code for state mismatch")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStartCallbackServer_MissingCode(t *testing.T) {
	state := "expected-state"

	codeCh, _, shutdown, err := StartCallbackServer(14551, state)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer shutdown()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:14551/auth/callback?state=expected-state")
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing code, got %d", resp.StatusCode)
	}

	select {
	case <-codeCh:
		t.Fatal("should not receive code when code is missing")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStartCallbackServer_PortFallback(t *testing.T) {
	state := "test-state"

	codeCh1, port1, shutdown1, err := StartCallbackServer(14560, state)
	if err != nil {
		t.Fatalf("failed to start first server: %v", err)
	}
	defer shutdown1()

	time.Sleep(50 * time.Millisecond)

	_, port2, shutdown2, err := StartCallbackServer(14560, state)
	if err != nil {
		t.Fatalf("failed to start second server (should have fallen back): %v", err)
	}
	defer shutdown2()

	if port1 == port2 {
		t.Fatalf("expected different ports, got %d and %d", port1, port2)
	}

	if port2 != 14561 {
		t.Fatalf("expected fallback port 14561, got %d", port2)
	}

	_ = codeCh1
}

func TestExtractAccountID_NestedClaim(t *testing.T) {
	payload := map[string]any{
		"sub": "user-123",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-nested-123",
		},
	}
	token := makeTestJWT(t, payload)

	accountID, err := ExtractAccountID(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accountID != "acct-nested-123" {
		t.Fatalf("expected acct-nested-123, got %s", accountID)
	}
}

func TestExtractAccountID_RootLevel(t *testing.T) {
	payload := map[string]any{
		"sub":                  "user-123",
		"chatgpt_account_id":   "acct-root-456",
	}
	token := makeTestJWT(t, payload)

	accountID, err := ExtractAccountID(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accountID != "acct-root-456" {
		t.Fatalf("expected acct-root-456, got %s", accountID)
	}
}

func TestExtractAccountID_Organizations(t *testing.T) {
	payload := map[string]any{
		"sub": "user-123",
		"organizations": []any{
			map[string]any{
				"id":   "org-789",
				"name": "Test Org",
			},
		},
	}
	token := makeTestJWT(t, payload)

	accountID, err := ExtractAccountID(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accountID != "org-789" {
		t.Fatalf("expected org-789, got %s", accountID)
	}
}

func TestExtractAccountID_EmptyToken(t *testing.T) {
	_, err := ExtractAccountID("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestExtractAccountID_InvalidJWT(t *testing.T) {
	_, err := ExtractAccountID("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for invalid JWT payload")
	}
}

func TestExtractAccountID_MissingAccountID(t *testing.T) {
	payload := map[string]any{
		"sub": "user-123",
	}
	token := makeTestJWT(t, payload)

	_, err := ExtractAccountID(token)
	if err == nil {
		t.Fatal("expected error when account_id is missing")
	}
}

func TestExtractAccountID_TooFewParts(t *testing.T) {
	_, err := ExtractAccountID("onlyonepart")
	if err == nil {
		t.Fatal("expected error for JWT with too few parts")
	}
}

func TestExchangeCodeForTokens_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if r.Form.Get("grant_type") != "authorization_code" {
			http.Error(w, "wrong grant type", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code") != "test-code" {
			http.Error(w, "wrong code", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code_verifier") != "test-verifier" {
			http.Error(w, "wrong verifier", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "mock-access-token",
			"refresh_token": "mock-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()

	access, refresh, expires, err := exchangeCodeForTokensWithURL("test-code", "test-verifier", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if access != "mock-access-token" {
		t.Fatalf("expected mock-access-token, got %s", access)
	}
	if refresh != "mock-refresh-token" {
		t.Fatalf("expected mock-refresh-token, got %s", refresh)
	}
	if expires < time.Now().Unix()+3500 {
		t.Fatalf("expires too soon: %d", expires)
	}
}

func TestExchangeCodeForTokens_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid grant", http.StatusBadRequest)
	}))
	defer server.Close()

	_, _, _, err := exchangeCodeForTokensWithURL("bad-code", "test-verifier", server.URL)
	if err == nil {
		t.Fatal("expected error for failed token exchange")
	}
}

func TestOAuthCredentials_Struct(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "access",
		RefreshToken: "refresh",
		Expires:      1234567890,
		AccountID:    "user-123",
	}

	if creds.AccessToken != "access" {
		t.Fatalf("expected access, got %s", creds.AccessToken)
	}
	if creds.RefreshToken != "refresh" {
		t.Fatalf("expected refresh, got %s", creds.RefreshToken)
	}
	if creds.Expires != 1234567890 {
		t.Fatalf("expected 1234567890, got %d", creds.Expires)
	}
	if creds.AccountID != "user-123" {
		t.Fatalf("expected user-123, got %s", creds.AccountID)
	}
}

func makeTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return "header." + encoded + ".signature"
}

func TestRequestDeviceCode_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if body["client_id"] != OAuthClientID {
			http.Error(w, "wrong client_id", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_auth_id": "mock-device-auth-id",
			"user_code":      "ABCD-1234",
			"interval":       "3",
			"expires_in":     900,
		})
	}))
	defer server.Close()

	deviceAuthID, userCode, interval, expiresIn, err := requestDeviceCodeWithURL(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deviceAuthID != "mock-device-auth-id" {
		t.Fatalf("expected mock-device-auth-id, got %s", deviceAuthID)
	}
	if userCode != "ABCD-1234" {
		t.Fatalf("expected ABCD-1234, got %s", userCode)
	}
	if interval != 3 {
		t.Fatalf("expected interval 3, got %d", interval)
	}
	if expiresIn != 900 {
		t.Fatalf("expected expires_in 900, got %d", expiresIn)
	}
}

func TestRequestDeviceCode_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, _, _, _, err := requestDeviceCodeWithURL(server.URL)
	if err == nil {
		t.Fatal("expected error for failed device code request")
	}
}

func TestRequestDeviceCode_MissingFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_auth_id": "some-id",
		})
	}))
	defer server.Close()

	_, _, _, _, err := requestDeviceCodeWithURL(server.URL)
	if err == nil {
		t.Fatal("expected error for missing user_code")
	}
}

func TestRequestDeviceCode_DefaultInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_auth_id": "some-id",
			"user_code":      "CODE-123",
			"interval":       "invalid",
			"expires_in":     600,
		})
	}))
	defer server.Close()

	_, _, interval, _, err := requestDeviceCodeWithURL(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if interval != 5 {
		t.Fatalf("expected default interval 5, got %d", interval)
	}
}

func TestPollForDeviceToken_PendingThenSuccess(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollCount++
		if pollCount < 3 {
			http.Error(w, "authorization_pending", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"authorization_code": "mock-auth-code",
			"code_verifier":      "mock-code-verifier",
		})
	}))
	defer server.Close()

	authCode, codeVerifier, err := pollForDeviceTokenWithURL("device-auth-id", "USER-CODE", 0, 30, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authCode != "mock-auth-code" {
		t.Fatalf("expected mock-auth-code, got %s", authCode)
	}
	if codeVerifier != "mock-code-verifier" {
		t.Fatalf("expected mock-code-verifier, got %s", codeVerifier)
	}
	if pollCount != 3 {
		t.Fatalf("expected 3 polls, got %d", pollCount)
	}
}

func TestPollForDeviceToken_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "authorization_pending", http.StatusForbidden)
	}))
	defer server.Close()

	_, _, err := pollForDeviceTokenWithURL("device-auth-id", "USER-CODE", 0, 1, server.URL)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestPollForDeviceToken_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	_, _, err := pollForDeviceTokenWithURL("device-auth-id", "USER-CODE", 0, 30, server.URL)
	if err == nil {
		t.Fatal("expected error for server error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 in error, got: %v", err)
	}
}

func TestPollForDeviceToken_NotFoundIsPending(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollCount++
		if pollCount < 2 {
			http.Error(w, "not_found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"authorization_code": "auth-code",
			"code_verifier":      "verifier",
		})
	}))
	defer server.Close()

	authCode, codeVerifier, err := pollForDeviceTokenWithURL("device-auth-id", "USER-CODE", 0, 30, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authCode != "auth-code" {
		t.Fatalf("expected auth-code, got %s", authCode)
	}
	if codeVerifier != "verifier" {
		t.Fatalf("expected verifier, got %s", codeVerifier)
	}
}

func TestDeviceFlow_FullFlow(t *testing.T) {
	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_auth_id": "dev-auth-123",
			"user_code":      "CODE-XYZ",
			"interval":       "0",
			"expires_in":     60,
		})
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "deviceauth/token") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"authorization_code": "dev-auth-code",
				"code_verifier":      "dev-code-verifier",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  makeTestJWT(t, map[string]any{
				"chatgpt_account_id": "acct-device-123",
			}),
			"refresh_token": "dev-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	oldDeviceURL := OAuthDeviceCodeURL
	oldDeviceTokenURL := OAuthDeviceTokenURL
	oldTokenURL := OAuthTokenURL
	defer func() {
		OAuthDeviceCodeURL = oldDeviceURL
		OAuthDeviceTokenURL = oldDeviceTokenURL
		OAuthTokenURL = oldTokenURL
	}()

	OAuthDeviceCodeURL = deviceServer.URL
	OAuthDeviceTokenURL = tokenServer.URL + "/api/accounts/deviceauth/token"
	OAuthTokenURL = tokenServer.URL

	creds, err := DeviceFlow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.AccountID != "acct-device-123" {
		t.Fatalf("expected acct-device-123, got %s", creds.AccountID)
	}
	if creds.RefreshToken != "dev-refresh-token" {
		t.Fatalf("expected dev-refresh-token, got %s", creds.RefreshToken)
	}
}

func TestParseCallbackInput_FullURL(t *testing.T) {
	input := "http://localhost:1455/auth/callback?code=test-code-123&state=abc123"
	code, state, err := ParseCallbackInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "test-code-123" {
		t.Fatalf("expected test-code-123, got %s", code)
	}
	if state != "abc123" {
		t.Fatalf("expected abc123, got %s", state)
	}
}

func TestParseCallbackInput_RawCode(t *testing.T) {
	input := "raw-auth-code-xyz"
	code, state, err := ParseCallbackInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "raw-auth-code-xyz" {
		t.Fatalf("expected raw-auth-code-xyz, got %s", code)
	}
	if state != "" {
		t.Fatalf("expected empty state, got %s", state)
	}
}

func TestParseCallbackInput_MissingCode(t *testing.T) {
	input := "http://localhost:1455/auth/callback?state=abc123"
	_, _, err := ParseCallbackInput(input)
	if err == nil {
		t.Fatal("expected error for missing code")
	}
	if !strings.Contains(err.Error(), "no authorization code") {
		t.Fatalf("expected 'no authorization code' error, got: %v", err)
	}
}

func TestParseCallbackInput_EmptyInput(t *testing.T) {
	_, _, err := ParseCallbackInput("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseCallbackInput_WhitespaceOnly(t *testing.T) {
	_, _, err := ParseCallbackInput("   \n\t  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only input")
	}
}

func TestParseCallbackInput_URLWithExtraParams(t *testing.T) {
	input := "http://localhost:1455/auth/callback?code=my-code&state=my-state&error=none&foo=bar"
	code, state, err := ParseCallbackInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "my-code" {
		t.Fatalf("expected my-code, got %s", code)
	}
	if state != "my-state" {
		t.Fatalf("expected my-state, got %s", state)
	}
}

func TestParseCallbackInput_InvalidURL(t *testing.T) {
	input := "http://localhost:1455/auth/callback?code=good&\x00invalid"
	_, _, err := ParseCallbackInput(input)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestManualFlow_FullFlow(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  makeTestJWT(t, map[string]any{
				"chatgpt_account_id": "acct-manual-123",
			}),
			"refresh_token": "manual-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	oldTokenURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldTokenURL }()
	OAuthTokenURL = tokenServer.URL

	input := "http://localhost:1455/auth/callback?code=test-code&state=ignored\n"
	var output strings.Builder

	creds, err := ManualFlow(strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.AccountID != "acct-manual-123" {
		t.Fatalf("expected acct-manual-123, got %s", creds.AccountID)
	}
	if creds.RefreshToken != "manual-refresh-token" {
		t.Fatalf("expected manual-refresh-token, got %s", creds.RefreshToken)
	}
	if !strings.Contains(output.String(), "Open the following URL") {
		t.Fatalf("expected URL prompt in output, got: %s", output.String())
	}
	if !strings.Contains(output.String(), "paste the full redirect URL") {
		t.Fatalf("expected paste instruction in output, got: %s", output.String())
	}
}

func TestManualFlow_RawCodeInput(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  makeTestJWT(t, map[string]any{
				"chatgpt_account_id": "acct-raw-456",
			}),
			"refresh_token": "raw-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	oldTokenURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldTokenURL }()
	OAuthTokenURL = tokenServer.URL

	input := "raw-auth-code-only\n"
	var output strings.Builder

	creds, err := ManualFlow(strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.AccountID != "acct-raw-456" {
		t.Fatalf("expected acct-raw-456, got %s", creds.AccountID)
	}
}

func TestManualFlow_NoInput(t *testing.T) {
	var output strings.Builder

	_, err := ManualFlow(strings.NewReader(""), &output)
	if err == nil {
		t.Fatal("expected error for no input")
	}
	if !strings.Contains(err.Error(), "no input received") {
		t.Fatalf("expected 'no input received' error, got: %v", err)
	}
}

func TestOAuthCredentials_IsExpired_NotExpired(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "valid-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	if creds.IsExpired() {
		t.Fatal("expected token to not be expired")
	}
}

func TestOAuthCredentials_IsExpired_Expired(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "expired-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(-1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	if !creds.IsExpired() {
		t.Fatal("expected token to be expired")
	}
}

func TestOAuthCredentials_IsExpired_WithinBuffer(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "near-expiry-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(2 * time.Minute).Unix(),
		AccountID:    "acct-123",
	}
	if !creds.IsExpired() {
		t.Fatal("expected token within buffer to be considered expired")
	}
}

func TestOAuthCredentials_IsExpired_ZeroExpiry(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "no-expiry-token",
		RefreshToken: "refresh",
		Expires:      0,
		AccountID:    "acct-123",
	}
	if !creds.IsExpired() {
		t.Fatal("expected token with zero expiry to be considered expired")
	}
}

func TestOAuthCredentials_IsExpired_JustOutsideBuffer(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "just-outside-buffer",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(6 * time.Minute).Unix(),
		AccountID:    "acct-123",
	}
	if creds.IsExpired() {
		t.Fatal("expected token just outside buffer to not be considered expired")
	}
}

func TestRefreshTokens_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if r.Form.Get("grant_type") != "refresh_token" {
			http.Error(w, "wrong grant type", http.StatusBadRequest)
			return
		}
		if r.Form.Get("refresh_token") != "old-refresh-token" {
			http.Error(w, "wrong refresh token", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()
	OAuthTokenURL = server.URL

	access, refresh, expires, err := refreshTokensWithURL("old-refresh-token", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if access != "new-access-token" {
		t.Fatalf("expected new-access-token, got %s", access)
	}
	if refresh != "new-refresh-token" {
		t.Fatalf("expected new-refresh-token, got %s", refresh)
	}
	if expires < time.Now().Unix()+3500 {
		t.Fatalf("expires too soon: %d", expires)
	}
}

func TestRefreshTokens_RotatedRefreshTokenMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access-token",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	access, refresh, _, err := refreshTokensWithURL("old-refresh-token", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if access != "new-access-token" {
		t.Fatalf("expected new-access-token, got %s", access)
	}
	if refresh != "old-refresh-token" {
		t.Fatalf("expected old-refresh-token (preserved), got %s", refresh)
	}
}

func TestRefreshTokens_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
	}))
	defer server.Close()

	_, _, _, err := refreshTokensWithURL("bad-refresh-token", server.URL)
	if err == nil {
		t.Fatal("expected error for failed refresh")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 in error, got: %v", err)
	}
}

func TestOAuthManager_EnsureValidToken_NoRefreshNeeded(t *testing.T) {
	var persisted OAuthCredentials
	creds := OAuthCredentials{
		AccessToken:  "valid-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	mgr := NewOAuthManager(creds, func(c OAuthCredentials) error {
		persisted = c
		return nil
	})

	err := mgr.EnsureValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if persisted.AccessToken != "" {
		t.Fatal("persist should not have been called")
	}

	token, err := mgr.GetAccessToken()
	if err != nil {
		t.Fatalf("unexpected error from GetAccessToken: %v", err)
	}
	if token != "valid-token" {
		t.Fatalf("expected valid-token, got %s", token)
	}
}

func TestOAuthManager_EnsureValidToken_RefreshNeeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed-access-token",
			"refresh_token": "refreshed-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()
	OAuthTokenURL = server.URL

	var persisted OAuthCredentials
	creds := OAuthCredentials{
		AccessToken:  "expired-token",
		RefreshToken: "old-refresh",
		Expires:      time.Now().Add(-1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	mgr := NewOAuthManager(creds, func(c OAuthCredentials) error {
		persisted = c
		return nil
	})

	err := mgr.EnsureValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if persisted.AccessToken != "refreshed-access-token" {
		t.Fatalf("expected refreshed-access-token in persist, got %s", persisted.AccessToken)
	}
	if persisted.RefreshToken != "refreshed-refresh-token" {
		t.Fatalf("expected refreshed-refresh-token in persist, got %s", persisted.RefreshToken)
	}

	token, err := mgr.GetAccessToken()
	if err != nil {
		t.Fatalf("unexpected error from GetAccessToken: %v", err)
	}
	if token != "refreshed-access-token" {
		t.Fatalf("expected refreshed-access-token, got %s", token)
	}
}

func TestOAuthManager_EnsureValidToken_ConcurrentAccess(t *testing.T) {
	refreshCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCount++
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fmt.Sprintf("token-%d", refreshCount),
			"refresh_token": "refreshed-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()
	OAuthTokenURL = server.URL

	persistCount := 0
	creds := OAuthCredentials{
		AccessToken:  "expired-token",
		RefreshToken: "old-refresh",
		Expires:      time.Now().Add(-1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	mgr := NewOAuthManager(creds, func(c OAuthCredentials) error {
		persistCount++
		return nil
	})

	var wg sync.WaitGroup
	results := make([]string, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token, err := mgr.GetAccessToken()
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", idx, err)
				return
			}
			results[idx] = token
		}(i)
	}
	wg.Wait()

	if refreshCount != 1 {
		t.Fatalf("expected 1 refresh, got %d", refreshCount)
	}
	if persistCount != 1 {
		t.Fatalf("expected 1 persist, got %d", persistCount)
	}

	for i, token := range results {
		if token == "" {
			t.Errorf("goroutine %d: got empty token", i)
		}
	}
}

func TestOAuthManager_EnsureValidToken_PersistError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed-access-token",
			"refresh_token": "refreshed-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()
	OAuthTokenURL = server.URL

	creds := OAuthCredentials{
		AccessToken:  "expired-token",
		RefreshToken: "old-refresh",
		Expires:      time.Now().Add(-1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	mgr := NewOAuthManager(creds, func(c OAuthCredentials) error {
		return fmt.Errorf("disk full")
	})

	err := mgr.EnsureValidToken()
	if err == nil {
		t.Fatal("expected persist error")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected 'disk full' error, got: %v", err)
	}
}

func TestOAuthManager_Credentials(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		Expires:      1234567890,
		AccountID:    "acct-123",
	}
	mgr := NewOAuthManager(creds, nil)

	got := mgr.Credentials()
	if got.AccessToken != "test-token" {
		t.Fatalf("expected test-token, got %s", got.AccessToken)
	}
	if got.RefreshToken != "test-refresh" {
		t.Fatalf("expected test-refresh, got %s", got.RefreshToken)
	}
	if got.Expires != 1234567890 {
		t.Fatalf("expected 1234567890, got %d", got.Expires)
	}
	if got.AccountID != "acct-123" {
		t.Fatalf("expected acct-123, got %s", got.AccountID)
	}
}

func TestOAuthManager_EnsureValidToken_RefreshError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()
	OAuthTokenURL = server.URL

	creds := OAuthCredentials{
		AccessToken:  "expired-token",
		RefreshToken: "invalid-refresh",
		Expires:      time.Now().Add(-1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	mgr := NewOAuthManager(creds, nil)

	err := mgr.EnsureValidToken()
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if !strings.Contains(err.Error(), "refreshing token") {
		t.Fatalf("expected 'refreshing token' error, got: %v", err)
	}
}
