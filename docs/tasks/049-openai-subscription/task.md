# Task 049: OpenAI Subscription Support (ChatGPT Plus/Pro)

## Why

OpenAI offers ChatGPT Plus/Pro subscribers access to Codex models through OAuth-based authentication. Both Pi and OpenCode support this mechanism. Tau should also support subscription-based access so users can leverage their existing ChatGPT subscriptions without needing a separate API key.

## Constraints

- Go language implementation (no Node.js modules)
- CLI application (no browser runtime dependencies)
- Must support both API key and OAuth auth methods simultaneously
- Backward compatible with existing API key configurations
- No external dependencies (stdlib only: crypto/rand, crypto/sha256, net/http, encoding/base64)
- Must work on Linux with AMD iGPU (local testing via Ollama only)

## Comparison with PI and OpenCode

| Aspect | Pi | OpenCode | Tau (proposed) |
|--------|----|----------|----------------|
| Architecture | Separate OAuth provider + dedicated API provider | Built-in plugin with auth hooks | Unified provider with auth mode |
| OAuth client ID | `app_EMoamEEZ73f0CkXaXp7hrann` | `app_EMoamEEZ73f0CkXaXp7hrann` | Same (shared by OpenAI) |
| Auth storage | Structured JSON with type field | Structured JSON with type field | Custom JSON type (backward compat) |
| Token refresh | Per-request with file locking | Per-request via custom fetch | Per-request with mutex |
| API endpoint | `chatgpt.com/backend-api/codex/responses` | Same (URL rewriting) | Same |
| Transports | SSE + WebSocket | SSE only (via AI SDK) | SSE only (matching current Tau) |
| Originator header | `pi` | `opencode` | `tau` |

## Main Constraints

1. **Auth storage format**: Current `AuthStore` is `map[string]string`. Need custom `UnmarshalJSON`/`MarshalJSON` to support both plain strings (backward compat) and structured OAuth objects.
2. **Token refresh**: Must handle concurrent requests safely (mutex-based locking with auth.json persistence).
3. **OAuth flow**: Three methods — browser callback (port 1455 with fallback), device authorization (polling), manual paste (tertiary fallback).
4. **Request differences**: Codex endpoint requires different body format (`instructions` vs system messages), different headers, and `store: false`.
5. **Connect command redesign**: Current `/connect` flow is tightly coupled to API key input. Needs conditional OAuth flow.

## Subtasks

### 049.1: Auth Storage Extension (Backward Compatible)
- Define `AuthValue` struct with custom `UnmarshalJSON`/`MarshalJSON`
- Plain string values serialize as-is (backward compatible with existing auth.json)
- OAuth values serialize as `{"type": "oauth", "access": "...", "refresh": "...", "expires": N, "account_id": "..."}`
- Update `config.LoadAuth` and `config.SaveAuth` to use new type
- Update `provider/auth.go:readAuthKey` to handle both formats
- Update `tui/connect.go` and `tui/providers.go` for new auth type
- Tests: backward compat loading, OAuth saving, round-trip serialization
- Files: `internal/config/config.go`, `internal/provider/auth.go`, `tui/connect.go`, `tui/providers.go`

### 049.2: OAuth Browser Flow (Local HTTP Server)
- PKCE verifier generation (`crypto/rand`, 32 bytes, base64url)
- PKCE challenge computation (`sha256`, base64url)
- Authorization URL creation with all required params
- Local HTTP server on port 1455 (fallback to 1456, 1457 if in use)
- Callback handler: validate state, extract code, show success page
- Authorization code exchange for tokens (POST to token URL)
- JWT parsing for account_id extraction (stdlib: base64 + json)
- Store credentials in auth.json
- Tests: URL generation, PKCE, code exchange, JWT parsing, server callback
- Files: `internal/provider/oauth.go` (new)

### 049.3: OAuth Device Authorization Flow (Headless)
- Device code request (POST to `https://auth.openai.com/api/accounts/deviceauth/usercode`)
- Display user code and verification URL to user
- Poll token endpoint until authorized or timeout
- Extract account_id from JWT
- Store credentials in auth.json
- Tests: device code request, polling logic, timeout handling
- Files: `internal/provider/oauth.go`

### 049.4: OAuth Manual Paste Fallback
- Prompt user to paste authorization code or full redirect URL
- Parse input (URL extraction or raw code)
- Validate state if URL format
- Exchange code for tokens
- Tests: URL parsing, raw code handling, state validation
- Files: `internal/provider/oauth.go`

### 049.5: Token Refresh Middleware
- Define `OAuthCredentials` struct
- Token expiry checking (with 5min buffer)
- Refresh token flow (POST to token URL)
- Concurrent request handling (`sync.Mutex`)
- Credential persistence after refresh (update auth.json)
- Tests: expiry detection, refresh flow, concurrent access, persistence
- Files: `internal/provider/oauth.go`

### 049.6: OpenAI Provider OAuth Integration
- Add `authMode` field and `oauth` credentials to `OpenAIProvider`
- Add `NewOpenAIOAuthProvider(creds OAuthCredentials)` constructor
- Auth type detection in `Stream()` and `Complete()` methods
- URL rewriting for Codex endpoint (`chatgpt.com/backend-api/codex/responses`)
- Header modifications (`chatgpt-account-id`, `originator: tau`)
- Body modifications (`instructions` field, `store: false`, `include` for reasoning)
- Token refresh check before each request
- Error classification for subscription-specific errors
- Tests: auth type detection, URL rewriting, headers, body, error handling
- Files: `internal/provider/openai.go`

### 049.7: /connect Command OAuth Flow
- Redesign `/connect` to support conditional auth flows
- Add "ChatGPT Plus/Pro (OAuth)" option in provider selection
- Auth method selection: browser / device / manual paste
- Run appropriate OAuth flow
- Verify token by extracting account_id
- Save structured auth to auth.json
- Register provider with OAuth credentials
- Tests: connection flow integration, auth method selection
- Files: `tui/connect.go`

### 049.8: Model Catalog Updates
- Add Codex models to catalog (gpt-5.5, gpt-5.4, gpt-5.4-mini, gpt-5.3-codex, gpt-5.3-codex-spark, gpt-5.2)
- Set costs to $0 for subscription models
- Configure context windows (gpt-5.5: 400K context, 272K input, 128K output)
- Configure reasoning support and thinking level maps
- Tests: model resolution, cost calculation, context windows
- Files: `internal/provider/catalog.go` or model catalog source

## Acceptance Criteria

1. User can authenticate with ChatGPT Plus/Pro via `/connect` command (browser, device, or manual paste)
2. OAuth tokens are stored securely in auth.json with structured format
3. Existing API key configurations continue to work unchanged (backward compatible)
4. Tokens are automatically refreshed when expired (with concurrent request safety)
5. Requests are correctly routed to Codex endpoint with proper headers
6. System prompts are passed via `instructions` field (not system messages)
7. Subscription-specific errors show friendly messages with plan type and reset time
8. All tests pass (unit + integration)
9. go vet / go build / go mod tidy clean
10. Binary rebuilt and manually tested

## Testing Strategy

- **Unit tests**: OAuth flow (URL generation, PKCE, token exchange, JWT parsing), token refresh (expiry detection, concurrent access), provider modifications (URL rewriting, headers, body), auth storage (backward compat, round-trip)
- **Integration tests**: Mock HTTP server for OAuth callback, token refresh, Codex endpoint
- **E2E testing**: Not possible without real ChatGPT subscription — document manual test steps
- **Edge cases**: Port conflict (fallback), expired refresh token, concurrent refresh requests, malformed JWT, network errors during token exchange

## Risks

1. **OAuth server port conflict**: Port 1455 may be in use. Implement fallback to 1456, 1457.
2. **Token refresh race conditions**: Multiple concurrent requests may trigger multiple refreshes. Use `sync.Mutex` with double-check after lock acquisition.
3. **JWT parsing failures**: Token format may change. Add robust error handling with fallback claim paths.
4. **Codex endpoint changes**: OpenAI may change the backend API. Monitor for breaking changes.
5. **Auth.json migration**: Existing users have plain string auth values. Custom JSON marshaling must handle both formats transparently.
6. **Device flow polling timeout**: User may not complete authorization in time. Add configurable timeout and user feedback.

## Research

See `docs/research/openai-subscription.md` for detailed analysis of Pi and OpenCode implementations.

## Review Notes

Reviewed by subagent ses_1aabaaba5ffe64SLEaHcfD3uQh. Critical findings addressed:
- Auth storage format: Custom JSON type with backward-compatible marshaling
- /connect command: Redesigned with conditional OAuth flow
- Device authorization flow: Split into separate subtask (049.3)
- Client ID: Corrected to `app_EMoamEEZ73f0CkXaXp7hrann` (same for Pi and OpenCode)
- Provider constructor: Added `NewOpenAIOAuthProvider` with `OAuthCredentials`
- Token refresh: Mutex-based with auth.json persistence
- Error classification: Added to subtask 049.6
- Originator header: Set to `tau`

## Progress

- [x] 049.1: Auth Storage Extension — Complete (see worklog.md)
- [x] 049.2: OAuth Browser Flow — Complete (see worklog.md)
- [x] 049.3: OAuth Device Authorization Flow — Complete (see worklog.md)
- [x] 049.4: OAuth Manual Paste Fallback
- [x] 049.5: Token Refresh Middleware
- [x] 049.6: OpenAI Provider OAuth Integration
- [x] 049.7: /connect Command OAuth Flow
- [x] 049.8: Model Catalog Updates
