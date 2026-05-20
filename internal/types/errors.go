package types

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel error types for the Tau system.
// These provide typed error categorization with Go error wrapping support.

// ProviderError indicates a failure in a provider (LLM API) operation.
// It wraps the underlying error for context.
type ProviderError struct {
	Op   string // Operation that failed, e.g., "stream", "complete"
	Err  error  // Underlying error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("provider error (%s): %v", e.Op, e.Err)
	}
	return fmt.Sprintf("provider error (%s)", e.Op)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NewProviderError creates a ProviderError wrapping the given error.
func NewProviderError(op string, err error) *ProviderError {
	return &ProviderError{Op: op, Err: err}
}

// ToolError indicates a failure during tool execution.
type ToolError struct {
	ToolName string // Name of the tool that failed
	Err      error  // Underlying error
}

func (e *ToolError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("tool error (%s): %v", e.ToolName, e.Err)
	}
	return fmt.Sprintf("tool error (%s)", e.ToolName)
}

func (e *ToolError) Unwrap() error {
	return e.Err
}

// NewToolError creates a ToolError wrapping the given error.
func NewToolError(toolName string, err error) *ToolError {
	return &ToolError{ToolName: toolName, Err: err}
}

// SessionError indicates a failure in session management operations.
type SessionError struct {
	Op  string // Operation that failed, e.g., "open", "append", "read"
	Err error  // Underlying error
}

func (e *SessionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("session error (%s): %v", e.Op, e.Err)
	}
	return fmt.Sprintf("session error (%s)", e.Op)
}

func (e *SessionError) Unwrap() error {
	return e.Err
}

// NewSessionError creates a SessionError wrapping the given error.
func NewSessionError(op string, err error) *SessionError {
	return &SessionError{Op: op, Err: err}
}

// IsProviderError reports whether err is or wraps a ProviderError.
func IsProviderError(err error) bool {
	var pErr *ProviderError
	return errors.As(err, &pErr)
}

// IsToolError reports whether err is or wraps a ToolError.
func IsToolError(err error) bool {
	var tErr *ToolError
	return errors.As(err, &tErr)
}

// IsSessionError reports whether err is or wraps a SessionError.
func IsSessionError(err error) bool {
	var sErr *SessionError
	return errors.As(err, &sErr)
}

// APIError represents a typed error from a provider API response.
// These provide structured error information for common failure modes.
type APIError struct {
	// Type categorizes the error for handling logic.
	Type APIErrorType
	// StatusCode is the HTTP status code from the response.
	StatusCode int
	// Message is the provider's error message (if available).
	Message string
	// Raw is the full error response body (for debugging).
	Raw string
}

// APIErrorType categorizes provider API errors.
type APIErrorType string

const (
	// ErrorTypeRateLimit indicates the provider is rate limiting requests.
	ErrorTypeRateLimit APIErrorType = "rate_limit"
	// ErrorTypeCreditExhausted indicates the account has no remaining credits.
	ErrorTypeCreditExhausted APIErrorType = "credit_exhausted"
	// ErrorTypeQuotaExceeded indicates a weekly/monthly quota has been reached.
	ErrorTypeQuotaExceeded APIErrorType = "quota_exceeded"
	// ErrorTypeAuthFailed indicates authentication failed (invalid/expired key).
	ErrorTypeAuthFailed APIErrorType = "auth_failed"
	// ErrorTypePermissionDenied indicates the account lacks permission for this model/feature.
	ErrorTypePermissionDenied APIErrorType = "permission_denied"
	// ErrorTypeModelUnavailable indicates the requested model is disabled or unavailable.
	ErrorTypeModelUnavailable APIErrorType = "model_unavailable"
	// ErrorTypeServerError indicates a provider-side server error (5xx).
	ErrorTypeServerError APIErrorType = "server_error"
	// ErrorTypeBadRequest indicates an invalid request (400).
	ErrorTypeBadRequest APIErrorType = "bad_request"
	// ErrorTypeUnknown indicates an uncategorized error.
	ErrorTypeUnknown APIErrorType = "unknown"
)

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (HTTP %d): %s", e.Type, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Type, e.StatusCode)
}

// IsRateLimit reports whether the error is a rate limit error.
func (e *APIError) IsRateLimit() bool {
	return e.Type == ErrorTypeRateLimit
}

// IsCreditExhausted reports whether the error indicates no remaining credits.
func (e *APIError) IsCreditExhausted() bool {
	return e.Type == ErrorTypeCreditExhausted
}

// IsQuotaExceeded reports whether a quota has been reached.
func (e *APIError) IsQuotaExceeded() bool {
	return e.Type == ErrorTypeQuotaExceeded
}

// IsAuthFailed reports whether authentication failed.
func (e *APIError) IsAuthFailed() bool {
	return e.Type == ErrorTypeAuthFailed
}

// UserMessage returns a user-friendly error message.
func (e *APIError) UserMessage() string {
	switch e.Type {
	case ErrorTypeRateLimit:
		if e.Message != "" {
			return e.Message
		}
		return "Rate limit reached. Please wait before sending more requests."
	case ErrorTypeCreditExhausted:
		if e.Message != "" {
			return e.Message
		}
		return "Account credits exhausted. Please add credits to continue."
	case ErrorTypeQuotaExceeded:
		if e.Message != "" {
			return e.Message
		}
		return "Usage quota exceeded. Please wait for the quota period to reset or upgrade your plan."
	case ErrorTypeAuthFailed:
		if e.Message != "" {
			return e.Message
		}
		return "Authentication failed. Please check your API key."
	case ErrorTypePermissionDenied:
		if e.Message != "" {
			return e.Message
		}
		return "Permission denied. Your account does not have access to this model or feature."
	case ErrorTypeModelUnavailable:
		if e.Message != "" {
			return fmt.Sprintf("Model unavailable: %s. Try selecting a different model.", e.Message)
		}
		return "This model is currently unavailable. Try selecting a different model."
	case ErrorTypeServerError:
		if e.Message != "" {
			return e.Message
		}
		return "Provider server error. Please try again later."
	case ErrorTypeBadRequest:
		if e.Message != "" {
			return fmt.Sprintf("Bad request: %s", e.Message)
		}
		return "Bad request. Please check your model and parameters."
	default:
		if e.Message != "" {
			return e.Message
		}
		return fmt.Sprintf("Provider error (HTTP %d)", e.StatusCode)
	}
}

// ClassifyAPIError examines an HTTP response and classifies the error.
// It parses the response body for provider-specific error messages.
func ClassifyAPIError(statusCode int, body []byte) *APIError {
	apiErr := &APIError{
		StatusCode: statusCode,
		Raw:        string(body),
	}

	// Try to extract error message from common response formats
	bodyStr := string(body)
	message := extractErrorMessage(bodyStr)
	if message != "" {
		apiErr.Message = message
	}

	// Classify by status code and message content
	switch {
	case statusCode == 429:
		if containsAny(message, "insufficient_quota", "quota_exceeded", "quota", "weekly limit", "monthly limit", "daily limit", "usage limit") {
			apiErr.Type = ErrorTypeQuotaExceeded
		} else if containsAny(message, "credit", "balance", "billing", "payment") {
			apiErr.Type = ErrorTypeCreditExhausted
		} else {
			apiErr.Type = ErrorTypeRateLimit
		}
	case statusCode == 401 || statusCode == 403:
		apiErr.Type = ErrorTypeAuthFailed
		if containsAny(message, "model", "disabled", "not found", "not available", "model_not_found", "invalid_model") {
			apiErr.Type = ErrorTypeModelUnavailable
		} else if containsAny(message, "credit", "balance", "quota", "billing", "payment", "insufficient_quota") {
			apiErr.Type = ErrorTypeCreditExhausted
		} else if containsAny(message, "invalid_api_key", "api_key_invalid", "incorrect_api_key") {
			apiErr.Type = ErrorTypeAuthFailed
		} else if containsAny(message, "maximum context length") {
			apiErr.Type = ErrorTypeBadRequest
		}
	case statusCode == 400:
		apiErr.Type = ErrorTypeBadRequest
		if containsAny(message, "quota", "limit", "exceeded", "insufficient_quota") {
			apiErr.Type = ErrorTypeQuotaExceeded
		} else if containsAny(message, "credit", "balance", "billing", "payment") {
			apiErr.Type = ErrorTypeCreditExhausted
		} else if containsAny(message, "model_not_found", "invalid_model", "model does not exist") {
			apiErr.Type = ErrorTypeModelUnavailable
		} else if containsAny(message, "maximum context length") {
			apiErr.Type = ErrorTypeBadRequest
		}
	case statusCode >= 500:
		apiErr.Type = ErrorTypeServerError
	default:
		// Check message content for specific error types
		switch {
		case containsAny(message, "credit", "balance", "billing", "payment", "top up", "add funds", "insufficient_credit"):
			apiErr.Type = ErrorTypeCreditExhausted
		case containsAny(message, "quota", "weekly limit", "monthly limit", "usage limit", "exceeded", "insufficient_quota"):
			apiErr.Type = ErrorTypeQuotaExceeded
		case containsAny(message, "permission", "access denied", "not allowed", "unauthorized"):
			apiErr.Type = ErrorTypePermissionDenied
		case containsAny(message, "model_not_found", "invalid_model", "model does not exist", "model_disabled"):
			apiErr.Type = ErrorTypeModelUnavailable
		case containsAny(message, "invalid_api_key", "api_key_invalid", "incorrect_api_key", "api key expired"):
			apiErr.Type = ErrorTypeAuthFailed
		case containsAny(message, "maximum context length"):
			apiErr.Type = ErrorTypeBadRequest
		default:
			apiErr.Type = ErrorTypeUnknown
		}
	}

	return apiErr
}


// extractErrorMessage tries to extract a human-readable error message from
// common provider API error response formats.
func extractErrorMessage(body string) string {
	// Try common JSON error formats:
	// {"error": {"message": "..."}}
	// {"error": "message"}
	// {"message": "..."}
	// {"detail": "..."}

	// Simple string-based extraction (avoiding full JSON parse for performance)
	// Look for "message" field
	if idx := findJSONString(body, "message"); idx != "" {
		return idx
	}
	if idx := findJSONString(body, "error"); idx != "" {
		return idx
	}
	if idx := findJSONString(body, "detail"); idx != "" {
		return idx
	}
	return ""
}

// findJSONString looks for a JSON string value for the given key.
// Handles nested objects like {"error": {"message": "..."}}.
func findJSONString(body, key string) string {
	// Look for "key": "value"
	searchKey := `"` + key + `"`
	idx := strings.Index(body, searchKey)
	if idx == -1 {
		return ""
	}

	rest := body[idx+len(searchKey):]
	// Skip whitespace and colon
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && rest[0] == ':' {
		rest = strings.TrimSpace(rest[1:])
	}

	if len(rest) == 0 {
		return ""
	}

	// If value is a nested object, look for "message" inside
	if rest[0] == '{' {
		return findJSONString(rest, "message")
	}

	// If value is a string, extract it
	if rest[0] == '"' {
		return extractJSONString(rest)
	}

	return ""
}

// extractJSONString extracts a JSON string value starting with ".
func extractJSONString(s string) string {
	if len(s) < 2 || s[0] != '"' {
		return ""
	}
	// Find closing quote (handle escaped quotes)
	for i := 1; i < len(s); i++ {
		if s[i] == '"' && s[i-1] != '\\' {
			return s[1:i]
		}
	}
	return ""
}

func containsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
