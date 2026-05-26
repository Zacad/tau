# Handoff: Subtask 049.2 — OAuth Browser Flow

## Context

This handoff is for continuing work on **Task 049: OpenAI Subscription Support**.
Subtask **049.1 (Auth Storage Extension)** is complete. This document provides everything needed to implement **049.2 (OAuth Browser Flow)**.

## What Was Completed (049.1)

### AuthValue struct (`internal/config/config.go`)
```go
type AuthValue struct {
    Type      string `json:"type,omitempty"`      // "oauth" or empty (means api_key)
    Value     string `json:"value,omitempty"`     // API key value
    Access    string `json:"access,omitempty"`    // OAuth access token
    Refresh   string `json:"refresh,omitempty"`   // OAuth refresh token
    Expires   int64  `json:"expires,omitempty"`   // Unix seconds
    AccountID string `json:"account_id,omitempty"` // From JWT
}
```

- Custom `MarshalJSON`: API keys → plain string (backward compat), OAuth → object
- Custom `UnmarshalJSON`: Detects string vs object at parse time
- Helper methods: `IsOAuth()`, `IsAPIKey()`, `IsEmpty()`, `APIKey()`
- `AuthStore` changed from `map[string]string` to `map[string]AuthValue`

### Files Modified
- `internal/config/config.go` — AuthValue, AuthStore, LoadAuth, SaveAuth
- `internal/provider/auth.go` — `readAuthKey` uses `AuthValue.APIKey()`
- `internal/tui/connect.go` — `saveProviderAuth` creates `AuthValue{Value: apiKey}`
- `internal/tui/providers.go` — `getProviderState` extracts via `.APIKey()`

### Tests
All pass: `go test ./internal/config/... ./internal/provider/...`

---

## Subtask 049.2: OAuth Browser Flow

### Goal
Implement the OAuth PKCE browser flow for OpenAI ChatGPT Plus/Pro subscription access.

### New File: `internal/provider/oauth.go`

### OAuth Constants
```go
const (
    OAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
    OAuthAuthURL      = "https://auth.openai.com/oauth/authorize"
    OAuthTokenURL     = "https://auth.openai.com/oauth/token"
    OAuthRedirectURI  = "http://localhost:1455/auth/callback"
    OAuthScope        = "openid profile email offline_access"
)
```

### Functions to Implement

#### 1. PKCE Generation
```go
func GeneratePKCE() (verifier, challenge string, err error)
```
- Generate 32-byte random verifier using `crypto/rand`
- Compute SHA-256 hash of verifier
- Base64url encode (no padding) for both verifier and challenge
- Reference: OpenCode uses same approach in `codex.ts`

#### 2. Authorization URL Builder
```go
func BuildAuthorizationURL(clientID, redirectURI, state, challenge string) string
```
Required query params:
- `response_type=code`
- `client_id=app_EMoamEEZ73f0CkXaXp7hrann`
- `redirect_uri=http://localhost:1455/auth/callback`
- `scope=openid profile email offline_access`
- `state=<random>`
- `code_challenge=<pkce_challenge>`
- `code_challenge_method=S256`
- `id_token_add_organizations=true`
- `codex_cli_simplified_flow=true`
- `originator=tau`

#### 3. Local HTTP Callback Server
```go
func StartCallbackServer(port int, expectedState string) (codeCh chan string, actualPort int, shutdown func(), err error)
```
- Start HTTP server on port 1455
- Fallback to 1456, 1457 if port in use (try up to 3 ports)
- Handle `/auth/callback` path
- Validate `state` param matches expectedState
- Extract `code` param from query string
- Send code to channel, show success HTML page
- Return shutdown function to stop server

#### 4. Token Exchange
```go
func ExchangeCodeForTokens(code, verifier string) (access, refresh string, expires int64, err error)
```
- POST to `https://auth.openai.com/oauth/token`
- Body: `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier`
- Parse response: `access_token`, `refresh_token`, `expires_in`
- Compute `expires` as `time.Now().Unix() + expires_in`

#### 5. JWT Account ID Extraction
```go
func ExtractAccountID(token string) (string, error)
```
- Split JWT on `.` and decode payload (base64url)
- Parse JSON payload
- Extract from `claims["https://api.openai.com/auth"].chatgpt_account_id`
- Fallbacks:
  1. `claims.chatgpt_account_id` (root level)
  2. `claims.organizations[0].id`
- Try `id_token` first, then `access_token`

#### 6. Main Browser Flow Orchestrator
```go
func BrowserFlow() (OAuthCredentials, error)
```
Steps:
1. Generate PKCE verifier + challenge
2. Generate random state
3. Build authorization URL
4. Start callback server (port 1455, fallback 1456, 1457)
5. Open browser to auth URL (use `xdg-open` on Linux, `open` on macOS)
6. Wait for callback with code
7. Shutdown server
8. Exchange code for tokens
9. Extract account_id from JWT
10. Return `OAuthCredentials{Access, Refresh, Expires, AccountID}`

### OAuthCredentials Struct
```go
type OAuthCredentials struct {
    AccessToken  string
    RefreshToken string
    Expires      int64  // Unix timestamp
    AccountID    string
}
```

### Testing Strategy

#### Unit Tests (`internal/provider/oauth_test.go`)
1. `TestGeneratePKCE` — Verifier is 32+ bytes, challenge is SHA-256 hash
2. `TestBuildAuthorizationURL` — URL contains all required params
3. `TestStartCallbackServer` — Server starts, receives code, validates state
4. `TestExchangeCodeForTokens` — Mock HTTP server, verify token parsing
5. `TestExtractAccountID` — Parse known JWT payload with account_id claim
6. `TestBrowserFlow_PortFallback` — Port conflict triggers fallback

#### Integration Test
- Use `httptest.Server` to mock OAuth endpoints
- Test full flow: URL generation → mock callback → token exchange → JWT parsing

### Key Implementation Notes

1. **State validation**: Generate cryptographically random state, validate on callback to prevent CSRF
2. **Port fallback**: Try 1455, then 1456, then 1457. Return actual port used.
3. **Browser opening**: Use `exec.Command("xdg-open", url)` on Linux. If fails, print URL for manual copy.
4. **Success page**: Return simple HTML page saying "Authentication successful. You can close this tab."
5. **Timeout**: Add context timeout (e.g., 5 minutes) for waiting on callback
6. **Error handling**: Return descriptive errors for each failure point

### Reference Implementations

#### OpenCode (`~/Projects/opencode/packages/opencode/src/plugin/codex.ts`)
- PKCE: `crypto.randomBytes(32).toString('base64url')`
- Challenge: `crypto.createHash('sha256').update(verifier).digest('base64url')`
- Callback server on port 1455
- Token exchange with `grant_type: authorization_code`

#### PI (`~/Projects/pi/packages/ai/src/utils/oauth/openai-codex.ts`)
- Same PKCE approach
- Callback server with state validation
- JWT parsing for `chatgpt_account_id`
- Token refresh with `grant_type: refresh_token`

### Dependencies (stdlib only)
- `crypto/rand` — PKCE verifier, state generation
- `crypto/sha256` — PKCE challenge
- `encoding/base64` — Base64url encoding, JWT decoding
- `encoding/json` — Token response, JWT payload parsing
- `net/http` — Callback server, token exchange
- `net/url` — URL building, query params
- `os/exec` — Browser opening
- `time` — Token expiry calculation
- `context` — Timeout handling
- `sync` — Server shutdown coordination

### Files to Create
- `internal/provider/oauth.go` — All OAuth flow implementation
- `internal/provider/oauth_test.go` — Unit + integration tests

### Files to Modify (later, for integration)
- `internal/provider/openai.go` — Add `NewOpenAIOAuthProvider` constructor (subtask 049.6)
- `internal/tui/connect.go` — Add OAuth flow option (subtask 049.7)

### Acceptance Criteria for 049.2
1. PKCE verifier/challenge generation works correctly
2. Authorization URL contains all required parameters
3. Local HTTP server starts on port 1455 (with fallback)
4. Callback handler validates state and extracts code
5. Token exchange returns access/refresh tokens
6. JWT parsing extracts account_id correctly
7. All unit tests pass
8. `go vet` / `go build` clean

---

## Next Steps After 049.2

- **049.3**: OAuth Device Authorization Flow (headless fallback)
- **049.4**: OAuth Manual Paste Fallback
- **049.5**: Token Refresh Middleware
- **049.6**: OpenAI Provider OAuth Integration
- **049.7**: /connect Command OAuth Flow
- **049.8**: Model Catalog Updates

## Research Documents
- `docs/research/openai-subscription.md` — Full research on Pi/OpenCode implementations
- `docs/tasks/049-openai-subscription/task.md` — Main task definition
