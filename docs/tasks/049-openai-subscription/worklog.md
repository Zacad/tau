# Worklog: Task 049 — OpenAI Subscription Support

## Subtask 049.1: Auth Storage Extension (Backward Compatible)

### Completed
- Added `AuthValue` struct with custom JSON marshaling in `internal/config/config.go`
- Changed `AuthStore` from `map[string]string` to `map[string]AuthValue`
- Updated `LoadAuth`/`SaveAuth` to use new type
- Updated `readAuthKey` in `internal/provider/auth.go` to extract API key from `AuthValue`
- Updated `saveProviderAuth` in `internal/tui/connect.go` to create `AuthValue{Value: apiKey}`
- Updated `getProviderState` in `internal/tui/providers.go` to use `.APIKey()` method
- Added comprehensive tests:
  - `TestAuthValue_MarshalJSON_APIKey` — API keys serialize as plain strings
  - `TestAuthValue_MarshalJSON_OAuth` — OAuth serializes as object
  - `TestAuthValue_UnmarshalJSON_String` — Backward compat string parsing
  - `TestAuthValue_UnmarshalJSON_Object` — OAuth object parsing
  - `TestAuthValue_RoundTrip_APIKey` / `TestAuthValue_RoundTrip_OAuth`
  - `TestLoadAuth_MixedFormat` — Mixed API key + OAuth in same file
  - `TestSaveAuth_BackwardCompatOutput` — Verifies output format compatibility
  - `TestAuthValue_IsEmpty` — Edge case testing
  - `TestResolveKey_AuthJSON_OAuth` — OAuth credential handling

### Verification
- `go vet` — clean
- `go build` — clean
- `go test ./internal/config/...` — all 40+ tests pass
- `go test ./internal/provider/...` — all auth tests pass

### Design Decision
Chose custom JSON marshaling over PI/OpenCode's explicit `type` field approach for all entries. This allows existing `auth.json` files with plain string API keys to work without migration. API keys serialize as `"sk-..."`, OAuth credentials serialize as `{"type":"oauth",...}`.

---

## Subtask 049.2: OAuth Browser Flow

### Completed
- Created `internal/provider/oauth.go` with:
  - OAuth constants (client ID, URLs, redirect URI, scope) as vars for testability
  - `OAuthCredentials` struct (AccessToken, RefreshToken, Expires, AccountID)
  - `GeneratePKCE()` — 32-byte random verifier via crypto/rand, SHA-256 challenge, base64url encoding
  - `BuildAuthorizationURL()` — URL with all required params (response_type, client_id, redirect_uri, scope, state, code_challenge, code_challenge_method, id_token_add_organizations, codex_cli_simplified_flow, originator)
  - `StartCallbackServer()` — Local HTTP server on port 1455 with fallback to 1456/1457, state validation, code extraction, success HTML page
  - `ExchangeCodeForTokens()` — POST to token endpoint with authorization_code grant
  - `ExtractAccountID()` — JWT payload decoding via base64url, nested claim + root + organizations fallbacks
  - `BrowserFlow()` — Full orchestrator (PKCE → URL → server → browser → callback → tokens → account_id)
- Created `internal/provider/oauth_test.go` with 16 tests:
  - `TestGeneratePKCE` — Verifier length, uniqueness, base64url validity
  - `TestBuildAuthorizationURL` — All required params present with correct values
  - `TestStartCallbackServer` — Server starts, receives code, validates state
  - `TestStartCallbackServer_StateMismatch` — 400 on wrong state
  - `TestStartCallbackServer_MissingCode` — 400 on missing code
  - `TestStartCallbackServer_PortFallback` — Fallback to next port on conflict
  - `TestExtractAccountID_NestedClaim` — `https://api.openai.com/auth.chatgpt_account_id`
  - `TestExtractAccountID_RootLevel` — Fallback to root-level claim
  - `TestExtractAccountID_Organizations` — Fallback to organizations[0].id
  - `TestExtractAccountID_EmptyToken` — Error on empty token
  - `TestExtractAccountID_InvalidJWT` — Error on malformed JWT
  - `TestExtractAccountID_MissingAccountID` — Error when no claim matches
  - `TestExtractAccountID_TooFewParts` — Error on JWT with <2 parts
  - `TestExchangeCodeForTokens_MockServer` — httptest mock token exchange
  - `TestExchangeCodeForTokens_ErrorResponse` — Error on non-200 response
  - `TestOAuthCredentials_Struct` — Struct field verification

### Verification
- `go vet ./internal/provider/...` — clean
- `go build ./...` — clean
- `go test ./internal/provider/...` — all 16 new tests pass, all existing tests pass
- `go mod tidy` — clean

### Design Decisions
- OAuth constants are `var` instead of `const` to allow test overrides (mock token URL)
- `exchangeCodeForTokensWithURL` internal function accepts configurable URL for testing
- Port fallback uses sequential try (1455→1456→1457) with 100ms listen detection
- Browser opening uses `xdg-open`/`open`/`cmd start` with silent error handling
- 5-minute timeout on callback wait via context.WithTimeout

---

## Subtask 049.3: OAuth Device Authorization Flow (Headless)

### Completed
- Added device flow constants to `internal/provider/oauth.go`:
  - `OAuthDeviceCodeURL` — `https://auth.openai.com/api/accounts/deviceauth/usercode`
  - `OAuthDeviceTokenURL` — `https://auth.openai.com/api/accounts/deviceauth/token`
  - `OAuthDeviceVerifyURL` — `https://auth.openai.com/codex/device`
- Added structs:
  - `deviceCodeResponse` — Parses device code initiation response
  - `deviceTokenResponse` — Parses device token polling response
- Implemented functions:
  - `requestDeviceCode()` — POST to deviceauth/usercode with client_id, returns device_auth_id, user_code, interval, expires_in
  - `requestDeviceCodeWithURL()` — Internal version with configurable URL for testing
  - `pollForDeviceToken()` — Polls deviceauth/token until authorized (200) or timeout, treats 403/404 as pending
  - `pollForDeviceTokenWithURL()` — Internal version with configurable URL
  - `DeviceFlow()` — Full orchestrator (request device code → poll → exchange tokens → extract account_id)
  - `parseInt()` — Helper for parsing interval string
- Added 9 tests to `internal/provider/oauth_test.go`:
  - `TestRequestDeviceCode_MockServer` — Mock device code initiation, verify all fields
  - `TestRequestDeviceCode_ErrorResponse` — Error on non-200 response
  - `TestRequestDeviceCode_MissingFields` — Error when user_code missing
  - `TestRequestDeviceCode_DefaultInterval` — Default interval 5 when parsing fails
  - `TestPollForDeviceToken_PendingThenSuccess` — 403 pending then 200 success
  - `TestPollForDeviceToken_Timeout` — Timeout when expires_in elapsed
  - `TestPollForDeviceToken_ServerError` — Non-403/404 error stops polling
  - `TestPollForDeviceToken_NotFoundIsPending` — 404 treated as pending (matching OpenCode behavior)
  - `TestDeviceFlow_FullFlow` — End-to-end with mock device + token servers

### Verification
- `go vet ./internal/provider/...` — clean
- `go build ./...` — clean
- `go test ./internal/provider/...` — all tests pass (9 new device flow tests + all existing tests)
- `go mod tidy` — clean

### Design Decisions
- OpenAI uses a custom device flow (not standard OAuth 2.0): POST JSON body with `device_auth_id` + `user_code`, not `device_code` + `grant_type`
- 403 and 404 both treated as "authorization pending" (matching OpenCode's implementation)
- Polling uses `time.Sleep` with interval from server response (minimum 1s, default 5s)
- Reuses `exchangeCodeForTokensWithURL` and `ExtractAccountID` from 049.2 for token exchange
- All external URLs are `var` for testability, internal `*WithURL` functions accept configurable URLs

---

## Subtask 049.4: OAuth Manual Paste Fallback

### Completed
- Added `ParseCallbackInput()` to `internal/provider/oauth.go`:
  - Parses full redirect URLs (extracts `code` and `state` query params)
  - Accepts raw authorization codes (no `?` in input)
  - Trims whitespace, validates non-empty input
  - Returns error for URLs without code or unparseable URLs
- Added `ManualFlow(reader io.Reader, writer io.Writer)` to `internal/provider/oauth.go`:
  - Generates PKCE verifier/challenge
  - Builds authorization URL
  - Prints URL and instructions to writer
  - Reads single line from reader (bufio.Scanner)
  - Parses input via `ParseCallbackInput`
  - Exchanges code for tokens via `ExchangeCodeForTokens`
  - Extracts account ID via `ExtractAccountID`
  - Returns `OAuthCredentials`
- Added 10 tests to `internal/provider/oauth_test.go`:
  - `TestParseCallbackInput_FullURL` — URL with code + state
  - `TestParseCallbackInput_RawCode` — raw authorization code string
  - `TestParseCallbackInput_MissingCode` — URL without code → error
  - `TestParseCallbackInput_EmptyInput` — empty string → error
  - `TestParseCallbackInput_WhitespaceOnly` — whitespace only → error
  - `TestParseCallbackInput_URLWithExtraParams` — URL with extra query params
  - `TestParseCallbackInput_InvalidURL` — malformed URL → error
  - `TestManualFlow_FullFlow` — end-to-end with mock token server, URL input
  - `TestManualFlow_RawCodeInput` — end-to-end with raw code input
  - `TestManualFlow_NoInput` — empty reader → error

### Verification
- `go vet ./internal/provider/...` — clean
- `go build ./...` — clean
- `go test ./internal/provider/...` — all tests pass (10 new + all existing)
- `go mod tidy` — clean

### Design Decisions
- `ManualFlow` accepts `io.Reader`/`io.Writer` for testability (no direct stdin/stdout dependency)
- State validation is not enforced in manual flow (user may paste raw code without state)
- Reuses all existing OAuth primitives: `GeneratePKCE`, `BuildAuthorizationURL`, `ExchangeCodeForTokens`, `ExtractAccountID`
- `bufio.Scanner` reads single line (user pastes one URL or code)

---

## Subtask 049.5: Token Refresh Middleware

### Completed
- Added `oauthRefreshBuffer` constant (5 minutes) to `internal/provider/oauth.go`
- Added `IsExpired()` method on `OAuthCredentials`:
  - Returns true if `Expires == 0` or current time + buffer >= expiry
- Added `RefreshTokens()` and `refreshTokensWithURL()` functions:
  - POST to token endpoint with `refresh_token` grant type
  - Parses response for new access token, refresh token (may be rotated), expiry
  - Preserves original refresh token if server doesn't return a new one
- Added `PersistFunc` type definition for credential persistence callbacks
- Added `OAuthManager` struct with:
  - `sync.Mutex` for thread-safe concurrent access
  - `creds` field for current OAuth credentials
  - `persist` callback for credential persistence
- Added `NewOAuthManager(creds, persist)` constructor
- Added `Credentials()` method — returns copy of current credentials (mutex-protected)
- Added `EnsureValidToken()` method:
  - Mutex-based locking with double-check after acquisition
  - Checks `IsExpired()` before attempting refresh
  - Calls `refreshTokensWithURL` if expired
  - Updates credentials and calls persist callback on success
  - Returns error if refresh or persist fails
- Added `GetAccessToken()` method — calls `EnsureValidToken()` then returns access token
- Added 14 tests to `internal/provider/oauth_test.go`:
  - `TestOAuthCredentials_IsExpired_NotExpired` — token with 1 hour remaining
  - `TestOAuthCredentials_IsExpired_Expired` — token expired 1 hour ago
  - `TestOAuthCredentials_IsExpired_WithinBuffer` — token expires in 2 minutes (within 5min buffer)
  - `TestOAuthCredentials_IsExpired_ZeroExpiry` — token with zero expiry timestamp
  - `TestOAuthCredentials_IsExpired_JustOutsideBuffer` — token expires in 6 minutes
  - `TestRefreshTokens_MockServer` — mock token exchange, verify new tokens
  - `TestRefreshTokens_RotatedRefreshTokenMissing` — preserves old refresh token when server doesn't rotate
  - `TestRefreshTokens_ErrorResponse` — error on non-200 response
  - `TestOAuthManager_EnsureValidToken_NoRefreshNeeded` — valid token, no refresh
  - `TestOAuthManager_EnsureValidToken_RefreshNeeded` — expired token, refresh succeeds
  - `TestOAuthManager_EnsureValidToken_ConcurrentAccess` — 5 concurrent goroutines, only 1 refresh
  - `TestOAuthManager_EnsureValidToken_PersistError` — refresh succeeds but persist fails
  - `TestOAuthManager_Credentials` — verify credentials accessor
  - `TestOAuthManager_EnsureValidToken_RefreshError` — refresh fails with invalid token

### Verification
- `go vet ./internal/provider/...` — clean
- `go build ./...` — clean
- `go test ./internal/provider/...` — all tests pass (14 new + all existing)
- `go mod tidy` — clean

### Design Decisions
- 5-minute refresh buffer prevents requests with soon-to-expire tokens
- Mutex-based locking with double-check prevents redundant concurrent refreshes
- `PersistFunc` callback enables auth.json persistence without coupling OAuthManager to config package
- Refresh token rotation handled: if server doesn't return new refresh token, original is preserved

---

## Subtask 049.6: OpenAI Provider OAuth Integration

### Completed
- Added `codexBaseURL` constant (`https://chatgpt.com/backend-api`) to `internal/provider/openai.go`
- Extended `OpenAIProvider` struct with:
  - `oauthManager *OAuthManager` — OAuth credential manager
  - `codexBaseURL string` — configurable Codex base URL for testing
- Added constructors:
  - `NewOpenAIOAuthProvider(creds)` — OAuth provider with default HTTP client
  - `NewOpenAIOAuthProviderWithPersist(creds, persist)` — OAuth provider with persistence callback
  - `NewOpenAIOAuthProviderWithClientAndCodexURL(creds, client, codexURL)` — test helper
- Added `isOAuth()` method — checks if oauthManager is non-nil
- Added `getAccessToken()` method — returns API key or OAuth access token (with refresh if needed)
- Added `buildHeaders(model, accessToken)` method:
  - Sets `Authorization: Bearer <token>`
  - For OAuth mode: adds `originator: tau` and `chatgpt-account-id` headers
  - Merges model-specific headers
- Added `buildRequestURL(baseURL)` method:
  - For OAuth mode: returns `codexBaseURL + "/responses"`
  - For API key mode: returns `baseURL + "/responses"` (default: `https://api.openai.com/v1/responses`)
- Updated `Stream()` and `Complete()` methods:
  - Use `getAccessToken()` instead of `apiKeyOrErr()`
  - Use `buildRequestURL()` for URL construction
  - Use `buildHeaders()` for header construction
  - Use `classifyError()` for error classification
- Updated `buildStreamRequest()` for OAuth mode:
  - Sets `store: false` (Codex endpoint requirement)
  - Uses `instructions` field instead of system message in input array
- Added `classifyError(statusCode, body)` method:
  - For OAuth mode: detects rate_limit, insufficient_quota, quota_exceeded, invalid_token, token_expired patterns
  - Returns user-friendly messages for subscription-specific errors
  - Falls back to `types.ClassifyAPIError` for unrecognized errors
- Updated `openAIRequest` struct:
  - Added `Instructions string` field
  - Added `Store *bool` field (pointer for optional serialization)
  - Changed `Input` to `omitempty` (not used in OAuth mode)
- Added 14 tests to `internal/provider/openai_test.go`:
  - `TestOpenAIProvider_IsOAuth_APIKeyMode` — API key provider not in OAuth mode
  - `TestOpenAIProvider_IsOAuth_OAuthMode` — OAuth provider in OAuth mode
  - `TestOpenAIProvider_OAuth_URLRewriting` — requests go to `/responses` path
  - `TestOpenAIProvider_OAuth_Headers` — Bearer token, originator, chatgpt-account-id headers
  - `TestOpenAIProvider_OAuth_BodyInstructions` — system prompt in `instructions` field, not `input`
  - `TestOpenAIProvider_OAuth_StoreFalse` — `store: false` in OAuth mode
  - `TestOpenAIProvider_OAuth_NoStoreInAPIKeyMode` — no `store` field in API key mode
  - `TestOpenAIProvider_OAuth_ErrorClassification_RateLimit` — rate limit error with friendly message
  - `TestOpenAIProvider_OAuth_ErrorClassification_QuotaExceeded` — quota exceeded error
  - `TestOpenAIProvider_OAuth_ErrorClassification_InvalidToken` — invalid token error
  - `TestOpenAIProvider_OAuth_TokenRefresh` — expired token triggers refresh before request
  - `TestOpenAIProvider_OAuth_NoAccountIDHeader` — no chatgpt-account-id when AccountID empty
  - `TestOpenAIProvider_OAuth_OriginatorAlwaysSet` — originator header always set in OAuth mode
  - `TestOpenAIProvider_OAuth_Complete` — Complete method works with OAuth

### Verification
- `go vet ./internal/provider/...` — clean
- `go build ./...` — clean
- `go test ./internal/provider/...` — all tests pass (14 new + all existing)
- `go mod tidy` — clean

### Design Decisions
- `codexBaseURL` is configurable on the provider struct for testability (not a global var)
- `Store` field uses `*bool` to allow `omitempty` serialization (only set in OAuth mode)
- `classifyError` returns `*types.APIError` to enable `.UserMessage()` calls in Stream method
- OAuth mode uses `instructions` field for system prompts (Codex endpoint requirement)
- Error classification checks body content for subscription-specific error patterns

---

## Subtask 049.7: /connect Command OAuth Flow

### Completed
- Added `openai-oauth` to `providerCatalog` in `internal/tui/providers.go`:
  - Name: "openai-oauth", DisplayName: "ChatGPT Plus/Pro (OAuth)"
  - RequiresAPIKey: false, BaseURL: "https://chatgpt.com/backend-api"
  - TestConnection: `testOpenAIOAuth` (no-op, OAuth verified during flow)
  - DiscoverModels: `discoverOpenAIOAuthModels` (returns hardcoded Codex model list)
- Added `ConditionalListStep` and `ConditionalTaskStep` to `internal/tui/palette/step.go`:
  - Allows list and task steps to be conditionally skipped based on prior results
  - Updated `skipIfNeeded()` to support both input and list/task step skipping
- Redesigned `connectSteps()` in `internal/tui/connect.go` with conditional OAuth flow:
  - **Select Provider** (list) — always shown
  - **Auth Method** (conditional list) — shown only for openai-oauth (browser/device/manual)
  - **Generate Authorization URL** (conditional task) — PKCE generation for manual flow
  - **Paste Authorization URL** (conditional input) — shown only for manual auth method
  - **Exchange Code for Tokens** (conditional task) — token exchange for manual flow
  - **Browser Authentication** (conditional task) — runs `provider.BrowserFlow()`
  - **Device Authorization** (conditional task) — runs `provider.DeviceFlow()`
  - **API Key** (conditional input) — skipped for openai-oauth
  - **Test Connection** (conditional task) — skipped for openai-oauth
  - **Discover Models** (task) — returns hardcoded Codex models for OAuth
  - **Save** (confirm) — always shown
- Added `saveProviderOAuthAuth()` function:
  - Extracts OAuth credentials from results map
  - Creates `config.AuthValue` with Type: "oauth" and all OAuth fields
  - Saves to auth.json via `config.SaveAuth()`
- Added `registerOAuthProvider()` function:
  - Loads OAuth credentials from auth.json
  - Creates `provider.NewOpenAIOAuthProviderWithPersist()` with persistence callback
  - Registers provider with hardcoded Codex models into session
- Updated `registerConnectedProvider()` to handle openai-oauth case:
  - Loads OAuth credentials from auth.json
  - Creates OAuth provider with persistence callback for token refresh
- Added `handleOAuthConnectResult()` function:
  - Saves OAuth credentials to auth.json
  - Saves provider config (enabled, base URL)
  - Registers OAuth provider with persistence callback
  - Displays success message with auth method and account ID
- Added 15 new tests:
  - `TestConnectSteps_IncludesOAuthSteps` — verifies OAuth provider in list
  - `TestConnectSteps_HasAuthMethodStep` — verifies auth method selection step
  - `TestConnectSteps_HasConditionalAPIKeyStep` — verifies conditional API key step
  - `TestConnectSteps_HasOAuthTaskSteps` — verifies all OAuth task steps present
  - `TestConnectSteps_HasDiscoverModelsStep` — verifies discover models step
  - `TestConnectSteps_HasSaveStep` — verifies save confirmation step
  - `TestConnectSteps_AuthMethodSkipCondition` — verifies auth method skip logic
  - `TestConnectSteps_APIKeySkipCondition` — verifies API key skip logic
  - `TestDiscoverModelsTask_OAuthProvider` — verifies model discovery for OAuth
  - `TestSaveProviderOAuthAuth` — verifies OAuth credential saving
  - `TestSaveProviderOAuthAuth_MissingCredentials` — verifies error on missing creds
  - `TestFindProvider_OpenAIOAuth` — verifies provider catalog entry
  - `TestListAvailableProviders_IncludesOpenAIOAuth` — verifies provider in list
  - `TestTestOpenAIOAuth_NoAPIKey` — verifies no-op test for OAuth
  - `TestTestOpenAIOAuth_WithAPIKey` — verifies error when API key passed
  - `TestDiscoverOpenAIOAuthModels` — verifies hardcoded Codex model list

### Verification
- `go vet ./...` — clean
- `go build ./...` — clean
- `go test ./internal/tui/...` — all tests pass (15 new + all existing)
- `go test ./internal/config/...` — all tests pass
- `go test ./internal/provider -run "OAuth|OpenAI"` — all OAuth tests pass
- Binary rebuilt: `go build -o tau ./cmd/tau`

### Design Decisions
- OAuth provider uses separate name "openai-oauth" to keep it distinct from API key OpenAI provider
- Conditional steps use `skipIf` functions that check `select_provider` and `auth_method` results
- Manual flow split into three steps (PKCE generation → URL display → code paste → token exchange) to work within TUI palette constraints
- Codex models are hardcoded (gpt-5.5, gpt-5.4, gpt-5.4-mini, gpt-5.3-codex, gpt-5.3-codex-spark, gpt-5.2) since Codex endpoint has no model listing API
- OAuth provider registration includes persistence callback for automatic token refresh
- `ConditionalListStep` and `ConditionalTaskStep` added to palette for reusability

---

## Subtask 049.8: Model Catalog Updates

### Completed
- Added `CodexModels()` function to `internal/provider/openai.go`:
  - Returns 6 fully-configured Codex models with metadata
  - All models have $0 costs (covered by subscription)
  - All models have `Reasoning: true` and effort-based thinking level maps
  - gpt-5.5: 400K context window, 128K max output tokens
  - gpt-5.4, gpt-5.3-codex: 272K context window, 128K max output tokens
  - gpt-5.4-mini, gpt-5.3-codex-spark, gpt-5.2: 128K context window, 128K max output tokens
- Updated `RegisterProvider` in `internal/sdk/sdk.go`:
  - Detects "openai-oauth" provider name
  - Uses `provider.CodexModels()` instead of bare model creation
  - Sets provider name and base URL on each model
- Updated OAuth provider constructors to use "openai-oauth" as provider name:
  - `NewOpenAIOAuthProvider`, `NewOpenAIOAuthProviderWithPersist`
  - Test helpers: `NewOpenAIOAuthProviderWithClient`, `NewOpenAIOAuthProviderWithClientAndCodexURL`
  - Ensures models are visible in `ListModels()` (provider name must match connected set)
- Added 8 tests to `internal/provider/openai_test.go`:
  - `TestCodexModels_Count` — returns exactly 6 models
  - `TestCodexModels_ExpectedIDs` — verifies all 6 model IDs
  - `TestCodexModels_ZeroCost` — all costs are $0
  - `TestCodexModels_Gpt55ContextWindow` — 400K context, 128K output
  - `TestCodexModels_AllHaveReasoning` — all models have `Reasoning: true`
  - `TestCodexModels_ThinkingLevelMap` — all have effort-based thinking levels
  - `TestCodexModels_InputTypes` — all have `["text"]` input types
  - `TestCodexModels_API` — all use `openai-completions` API type
- Added 1 test to `internal/sdk/sdk_test.go`:
  - `TestSession_RegisterProvider_OAuthCodexModels` — verifies models registered with full metadata

### Verification
- `go vet ./...` — clean
- `go build ./...` — clean
- `go test ./internal/provider/...` — all tests pass (8 new + all existing)
- `go test ./internal/sdk/...` -run "RegisterProvider" — all tests pass (1 new + all existing)
- `go mod tidy` — clean
- Binary rebuilt: `go build -o tau ./cmd/tau`

### Design Decisions
- `CodexModels()` returns pre-configured models so `RegisterProvider` only needs to set `Provider` and `BaseURL`
- OAuth provider name changed from "openai" to "openai-oauth" to match the configuration name used in TUI/connect flow
- This ensures `ListModels()` correctly filters models (connected set must match model provider field)
- Context windows use reasonable defaults: 400K for gpt-5.5 (matching OpenCode), 272K for full models, 128K for mini/spark/older
- All models use effort-based thinking level maps (matching catalog's `thinkingLevelMapFor` for gpt-5.x)
