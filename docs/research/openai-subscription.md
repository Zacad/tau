# OpenAI Subscription (ChatGPT Plus/Pro) — Research & Learning

## Overview

OpenAI offers ChatGPT Plus/Pro subscribers access to Codex models through an OAuth-based authentication flow, rather than requiring a standalone API key. Both Pi and OpenCode support this mechanism.

## How It Works

### OAuth Flow (PKCE)

The subscription access uses the OAuth 2.0 PKCE (Proof Key for Code Exchange) flow against OpenAI's auth infrastructure:

- **Authorization URL**: `https://auth.openai.com/oauth/authorize`
- **Token URL**: `https://auth.openai.com/oauth/token`
- **Client ID**: `app_EMoamEEZ73f0CkXaXp7hrann` (shared by both Pi and OpenCode)
- **Redirect URI**: `http://localhost:1455/auth/callback`
- **Scope**: `openid profile email offline_access`
- **Special params**: `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `originator=<app_name>`

### Auth Methods

1. **Browser mode**: Local HTTP server on port 1455 receives the OAuth callback
2. **Device authorization flow**: POST to `https://auth.openai.com/api/accounts/deviceauth/usercode`, user visits `https://auth.openai.com/codex/device`, poll `https://auth.openai.com/api/accounts/deviceauth/token`
3. **Manual paste**: User pastes the authorization code/redirect URL (tertiary fallback)

### Token Management

- Access token (JWT) used as Bearer token in API requests
- Refresh token for automatic token renewal
- `chatgpt_account_id` extracted from JWT claims for multi-org routing
- Tokens stored in auth file with type `"oauth"`

### JWT Account ID Extraction

The `chatgpt_account_id` is extracted from the JWT access token at claim path:
```
payload["https://api.openai.com/auth"].chatgpt_account_id
```

Fallback checks:
1. `claims.chatgpt_account_id` (root level)
2. `claims.organizations[0].id` (fallback)

Try `id_token` first, then fall back to `access_token`.

### API Endpoint Rewriting

Subscription requests are NOT sent to `api.openai.com/v1/responses`. Instead, they go to:
```
https://chatgpt.com/backend-api/codex/responses
```

The URL is rewritten from the standard OpenAI Responses API path.

### Available Models (Subscription)

Models available through ChatGPT Plus/Pro subscription:
- `gpt-5.5` (default, 400K context)
- `gpt-5.4`
- `gpt-5.4-mini`
- `gpt-5.3-codex`
- `gpt-5.3-codex-spark`
- `gpt-5.2`
- Any GPT version > 5.4

### Pricing

- All model costs are **$0** for subscription users (covered by subscription)
- Service tier options: `flex` (0.5x), `default` (1x), `priority` (2x-2.5x)

### Request Differences

Subscription (Codex) endpoint has special requirements:
- System prompts passed via `instructions` field (not as system messages)
- `store: false` required
- `include: ["reasoning.encrypted_content"]` for reasoning models
- `prompt_cache_key` for session caching
- Additional headers: `chatgpt-account-id`, `originator`, `session_id`

### Error Handling

Subscription-specific error codes:
- `usage_limit_reached` — user hit their ChatGPT usage limit
- `usage_not_included` — subscription doesn't include Codex access
- `rate_limit_exceeded` — rate limited

Errors include `plan_type` and `resets_at` fields for friendly messages.

## Reference Implementations

### OpenCode (`~/Projects/opencode`)

**Key file**: `packages/opencode/src/plugin/codex.ts` (622 lines)

Architecture:
- Built-in plugin (`CodexAuthPlugin`) registered in the plugin system
- Three auth hooks: `auth.loader`, `provider.models`, `chat.headers`
- Custom `fetch` function intercepts all HTTP requests for token refresh and URL rewriting
- Uses `@ai-sdk/openai` as the underlying SDK
- Dummy API key (`opencode-oauth-dummy-key`) used to satisfy SDK requirements

Token refresh flow:
1. Check if access token is expired
2. If expired, POST to `https://auth.openai.com/oauth/token` with `grant_type: refresh_token`
3. Update stored tokens
4. Set real `Authorization: Bearer <access_token>` header
5. Add `ChatGPT-Account-Id` header
6. Rewrite URL from `/v1/responses` to `/backend-api/codex/responses`

### Pi (`~/Projects/pi`)

**Key files**:
- `packages/ai/src/utils/oauth/openai-codex.ts` (458 lines) — OAuth flow
- `packages/ai/src/providers/openai-codex-responses.ts` (1351 lines) — API provider

Architecture:
- Separate OAuth provider (`openaiCodexOAuthProvider`) registered in OAuth registry
- Dedicated API provider (`openai-codex-responses`) with SSE and WebSocket transports
- WebSocket support with connection pooling and caching (5min TTL)
- Retry logic for transient errors (3 retries, exponential backoff)
- Service tier support with cost multipliers

## Tau Current State

### Provider System

- `Provider` interface in `internal/provider/provider.go`
- `baseProvider` struct with `apiKey` field
- OpenAI provider uses `openai-responses` API type
- Auth resolution: CLI flag → auth.json → env var → config file
- Auth stored as `map[string]string` in `~/.tau/auth.json`

### Key Files to Modify

| File | Purpose |
|------|---------|
| `internal/provider/openai.go` | Add OAuth token management, URL rewriting |
| `internal/provider/auth.go` | Extend auth types to support OAuth credentials |
| `internal/config/config.go` | Extend AuthStore to support structured auth data |
| `internal/provider/registry.go` | Register new provider variant |
| `internal/types/provider.go` | May need auth type field on Model |
| `tui/connect.go` | Add OAuth connection flow |
| `internal/types/errors.go` | Add Codex-specific error types |

### Constraints

- Go language (no Node.js crypto/http modules)
- CLI application (no browser runtime)
- Must support both API key and OAuth auth methods
- Should maintain backward compatibility with existing API key auth
- No external dependencies needed (stdlib for crypto, JWT parsing, HTTP server)

## Implementation Approaches

### Approach A: Separate Provider (`openai-codex`)

Create a new `OpenAICodexProvider` that handles subscription-based access alongside the existing `OpenAIProvider`.

**Pros**:
- Clean separation of concerns
- No changes to existing OpenAI provider
- Easy to test independently
- Matches Pi's architecture

**Cons**:
- Code duplication between providers
- Two provider registrations needed
- User must choose which provider to configure

### Approach B: Unified Provider with Auth Mode

Extend the existing `OpenAIProvider` to support both API key and OAuth auth modes internally.

**Pros**:
- Single provider interface for users
- Shared request building logic
- Less code duplication
- Matches OpenCode's plugin approach

**Cons**:
- More complex provider implementation
- Requires auth type detection
- Needs token refresh middleware

### Approach C: Middleware/Interceptor Pattern

Keep the existing OpenAI provider but add an OAuth interceptor that handles token management and request rewriting.

**Pros**:
- Minimal changes to existing provider
- Clean separation of auth concerns
- Reusable for other OAuth providers

**Cons**:
- More architectural complexity
- Requires new middleware abstraction
- May be overkill for a single provider

## Recommended Approach

**Approach B (Unified Provider with Auth Mode)** is recommended because:

1. Best user experience — single "openai" provider, auth method is transparent
2. Matches OpenCode's pattern (plugin-based auth loading)
3. Minimal code duplication
4. Easy to extend for future OAuth providers (GitHub Copilot, etc.)
5. Backward compatible — existing API key configs work unchanged

## Key Implementation Details

### Auth Storage Format (Backward Compatible)

Define `AuthValue` with custom JSON marshaling:

```go
type AuthValue struct {
    Type      string `json:"type,omitempty"`      // "api_key" or "oauth"
    Value     string `json:"value,omitempty"`     // for api_key type
    Access    string `json:"access,omitempty"`    // for oauth type
    Refresh   string `json:"refresh,omitempty"`
    Expires   int64  `json:"expires,omitempty"`
    AccountID string `json:"account_id,omitempty"`
}

type AuthStore map[string]AuthValue
```

Custom `UnmarshalJSON`/`MarshalJSON`:
- When serializing an API key value, write it as a plain string (backward compatible)
- When serializing OAuth credentials, write as an object with `type: "oauth"`
- When deserializing, detect string vs object and populate accordingly

Example auth.json:
```json
{
  "openai": {
    "type": "oauth",
    "access": "<jwt_access_token>",
    "refresh": "<refresh_token>",
    "expires": 1234567890,
    "account_id": "<chatgpt_account_id>"
  },
  "anthropic": "sk-ant-..."
}
```

### OAuth Credentials Struct

```go
type OAuthCredentials struct {
    AccessToken  string
    RefreshToken string
    Expires      int64
    AccountID    string
}
```

### OpenAIProvider Changes

```go
type OpenAIProvider struct {
    baseProvider
    authMode string            // "api_key" or "oauth"
    oauth    *OAuthCredentials
    mu       sync.Mutex        // for token refresh
}
```

### OAuth Flow Steps

1. Generate PKCE verifier and challenge (crypto/rand + sha256 + base64)
2. Create authorization URL with all required parameters
3. Start local HTTP server on port 1455 (with fallback to 1456, 1457)
4. Open browser to authorization URL
5. Wait for callback with authorization code
6. Exchange code for access/refresh tokens
7. Extract account_id from JWT
8. Store credentials in auth.json

### Token Refresh

Before each request:
1. Check if `expires` timestamp is in the past (with 5min buffer)
2. If expired, POST to token URL with refresh token
3. Update stored credentials (with mutex locking)
4. Persist refreshed tokens to auth.json
5. Use new access token for request

### Request Modifications for Codex

When using OAuth auth:
- BaseURL: `https://chatgpt.com/backend-api`
- URL path: `/codex/responses`
- Headers: `Authorization: Bearer <token>`, `chatgpt-account-id: <id>`, `originator: tau`
- Body: `instructions` field for system prompt, `store: false`

### Error Classification

Detect subscription-specific errors:
- Parse error response for `usage_limit_reached`, `usage_not_included`
- Show friendly message with plan type and reset time
- Distinguish from standard API key auth errors

### Go Dependencies (stdlib only)

- `crypto/rand` — PKCE verifier generation
- `crypto/sha256` — PKCE challenge computation
- `encoding/base64` — PKCE encoding, JWT parsing
- `encoding/json` — JWT payload parsing, auth storage
- `net/http` — OAuth callback server, token exchange
- `sync` — mutex for token refresh
- `strings` — JWT token splitting
