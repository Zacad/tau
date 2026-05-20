package types

import (
	"testing"
)

func TestClassifyAPIError_RateLimit(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantType   APIErrorType
	}{
		{
			name:       "429 no body",
			statusCode: 429,
			body:       "",
			wantType:   ErrorTypeRateLimit,
		},
		{
			name:       "429 with message",
			statusCode: 429,
			body:       `{"error": {"message": "Rate limit exceeded"}}`,
			wantType:   ErrorTypeRateLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyAPIError(tt.statusCode, []byte(tt.body))
			if err.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", err.Type, tt.wantType)
			}
			if !err.IsRateLimit() {
				t.Error("IsRateLimit() should be true")
			}
		})
	}
}

func TestClassifyAPIError_AuthFailed(t *testing.T) {
	err := ClassifyAPIError(401, []byte(`{"error": "Invalid API key"}`))
	if err.Type != ErrorTypeAuthFailed {
		t.Errorf("Type = %v, want %v", err.Type, ErrorTypeAuthFailed)
	}
	if !err.IsAuthFailed() {
		t.Error("IsAuthFailed() should be true")
	}
}

func TestClassifyAPIError_CreditExhausted(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "403 with credit message",
			statusCode: 403,
			body:       `{"error": {"message": "Insufficient credits"}}`,
		},
		{
			name:       "400 with billing message",
			statusCode: 400,
			body:       `{"error": "Your account balance is zero"}`,
		},
		{
			name:       "402 with payment message",
			statusCode: 402,
			body:       `{"error": {"message": "Payment required"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyAPIError(tt.statusCode, []byte(tt.body))
			if err.Type != ErrorTypeCreditExhausted {
				t.Errorf("Type = %v, want %v", err.Type, ErrorTypeCreditExhausted)
			}
			if !err.IsCreditExhausted() {
				t.Error("IsCreditExhausted() should be true")
			}
		})
	}
}

func TestClassifyAPIError_QuotaExceeded(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantType   APIErrorType
	}{
		{
			name:       "400 with quota message",
			statusCode: 400,
			body:       `{"error": {"message": "Weekly quota exceeded"}}`,
			wantType:   ErrorTypeQuotaExceeded,
		},
		{
			name:       "429 with monthly limit",
			statusCode: 429,
			body:       `{"error": "Monthly limit reached"}`,
			wantType:   ErrorTypeQuotaExceeded,
		},
		{
			name:       "429 with weekly limit",
			statusCode: 429,
			body:       `{"error": "Weekly limit reached"}`,
			wantType:   ErrorTypeQuotaExceeded,
		},
		{
			name:       "429 generic rate limit",
			statusCode: 429,
			body:       `{"error": "Too many requests"}`,
			wantType:   ErrorTypeRateLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyAPIError(tt.statusCode, []byte(tt.body))
			if err.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", err.Type, tt.wantType)
			}
			if tt.wantType == ErrorTypeQuotaExceeded && !err.IsQuotaExceeded() {
				t.Error("IsQuotaExceeded() should be true")
			}
		})
	}
}

func TestClassifyAPIError_ServerError(t *testing.T) {
	tests := []int{500, 502, 503, 504}
	for _, code := range tests {
		t.Run(string(rune(code)), func(t *testing.T) {
			err := ClassifyAPIError(code, []byte(`{"error": "Internal server error"}`))
			if err.Type != ErrorTypeServerError {
				t.Errorf("status %d: Type = %v, want %v", code, err.Type, ErrorTypeServerError)
			}
		})
	}
}

func TestClassifyAPIError_BadRequest(t *testing.T) {
	err := ClassifyAPIError(400, []byte(`{"error": {"message": "Invalid model"}}`))
	if err.Type != ErrorTypeBadRequest {
		t.Errorf("Type = %v, want %v", err.Type, ErrorTypeBadRequest)
	}
}

func TestAPIError_UserMessage(t *testing.T) {
	tests := []struct {
		err  *APIError
		want string
	}{
		{
			err:  &APIError{Type: ErrorTypeRateLimit, StatusCode: 429},
			want: "Rate limit reached. Please wait before sending more requests.",
		},
		{
			err:  &APIError{Type: ErrorTypeCreditExhausted, StatusCode: 403},
			want: "Account credits exhausted. Please add credits to continue.",
		},
		{
			err:  &APIError{Type: ErrorTypeQuotaExceeded, StatusCode: 400},
			want: "Usage quota exceeded. Please wait for the quota period to reset or upgrade your plan.",
		},
		{
			err:  &APIError{Type: ErrorTypeAuthFailed, StatusCode: 401},
			want: "Authentication failed. Please check your API key.",
		},
		{
			err:  &APIError{Type: ErrorTypePermissionDenied, StatusCode: 403},
			want: "Permission denied. Your account does not have access to this model or feature.",
		},
		{
			err:  &APIError{Type: ErrorTypeModelUnavailable, StatusCode: 401, Message: "Model is disabled"},
			want: "Model unavailable: Model is disabled. Try selecting a different model.",
		},
		{
			err:  &APIError{Type: ErrorTypeServerError, StatusCode: 500},
			want: "Provider server error. Please try again later.",
		},
		{
			err:  &APIError{Type: ErrorTypeBadRequest, StatusCode: 400, Message: "Invalid model"},
			want: "Bad request: Invalid model",
		},
		{
			err:  &APIError{Type: ErrorTypeUnknown, StatusCode: 418, Message: "I'm a teapot"},
			want: "I'm a teapot",
		},
	}

	for _, tt := range tests {
		got := tt.err.UserMessage()
		if got != tt.want {
			t.Errorf("UserMessage() = %q, want %q", got, tt.want)
		}
	}
}

func TestAPIError_UserMessage_ReturnsProviderMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "rate limit with provider message",
			err: &APIError{
				Type:       ErrorTypeRateLimit,
				StatusCode: 429,
				Message:    "Rate limit reached. Please wait 10 seconds before retrying.",
			},
			want: "Rate limit reached. Please wait 10 seconds before retrying.",
		},
		{
			name: "rate limit without message",
			err: &APIError{
				Type:       ErrorTypeRateLimit,
				StatusCode: 429,
			},
			want: "Rate limit reached. Please wait before sending more requests.",
		},
		{
			name: "quota exceeded with provider message",
			err: &APIError{
				Type:       ErrorTypeQuotaExceeded,
				StatusCode: 429,
				Message:    "Weekly limit reached. Limit will reset on 2026-05-18.",
			},
			want: "Weekly limit reached. Limit will reset on 2026-05-18.",
		},
		{
			name: "credit exhausted with provider message",
			err: &APIError{
				Type:       ErrorTypeCreditExhausted,
				StatusCode: 402,
				Message:    "Insufficient credits. Add funds at opencode.ai/billing.",
			},
			want: "Insufficient credits. Add funds at opencode.ai/billing.",
		},
		{
			name: "auth failed with provider message",
			err: &APIError{
				Type:       ErrorTypeAuthFailed,
				StatusCode: 401,
				Message:    "Invalid API key. Please check your credentials.",
			},
			want: "Invalid API key. Please check your credentials.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.UserMessage()
			if got != tt.want {
				t.Errorf("UserMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "nested error object",
			body: `{"error": {"message": "Something went wrong"}}`,
			want: "Something went wrong",
		},
		{
			name: "flat error string",
			body: `{"error": "Something went wrong"}`,
			want: "Something went wrong",
		},
		{
			name: "flat message string",
			body: `{"message": "Something went wrong"}`,
			want: "Something went wrong",
		},
		{
			name: "detail field",
			body: `{"detail": "Something went wrong"}`,
			want: "Something went wrong",
		},
		{
			name: "no error field",
			body: `{"data": "some data"}`,
			want: "",
		},
		{
			name: "empty body",
			body: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorMessage(tt.body)
			if got != tt.want {
				t.Errorf("extractErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyAPIError_ModelUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantType   APIErrorType
	}{
		{
			name:       "401 with model disabled message (opencode-go format)",
			statusCode: 401,
			body:       `{"type":"error","error":{"type":"ModelError","message":"Model is disabled"}}`,
			wantType:   ErrorTypeModelUnavailable,
		},
		{
			name:       "401 with model not found message",
			statusCode: 401,
			body:       `{"error": {"message": "Model not found"}}`,
			wantType:   ErrorTypeModelUnavailable,
		},
		{
			name:       "403 with model not available message",
			statusCode: 403,
			body:       `{"error": {"message": "Model not available"}}`,
			wantType:   ErrorTypeModelUnavailable,
		},
		{
			name:       "401 with invalid API key (should be auth_failed)",
			statusCode: 401,
			body:       `{"error": {"message": "Invalid API key"}}`,
			wantType:   ErrorTypeAuthFailed,
		},
		{
			name:       "401 with missing API key (should be auth_failed)",
			statusCode: 401,
			body:       `{"error": {"message": "Missing API key"}}`,
			wantType:   ErrorTypeAuthFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyAPIError(tt.statusCode, []byte(tt.body))
			if err.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", err.Type, tt.wantType)
			}
		})
	}
}

func TestAPIError_ModelUnavailable_UserMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "with message",
			err:  &APIError{Type: ErrorTypeModelUnavailable, StatusCode: 401, Message: "Model is disabled"},
			want: "Model unavailable: Model is disabled. Try selecting a different model.",
		},
		{
			name: "without message",
			err:  &APIError{Type: ErrorTypeModelUnavailable, StatusCode: 401},
			want: "This model is currently unavailable. Try selecting a different model.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.UserMessage()
			if got != tt.want {
				t.Errorf("UserMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyAPIError_ContextOverflow(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "400 with context overflow",
			statusCode: 400,
			body:       `{"error": {"message": "This model's maximum context length is 128000 tokens. You have sent 150000 tokens."}}`,
		},
		{
			name:       "401 with context overflow",
			statusCode: 401,
			body:       `{"error": {"message": "maximum context length is 8192 tokens"}}`,
		},
		{
			name:       "default with context overflow",
			statusCode: 200,
			body:       `{"error": {"message": "maximum context length is 128000 tokens"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyAPIError(tt.statusCode, []byte(tt.body))
			if err.Type != ErrorTypeBadRequest {
				t.Errorf("Type = %v, want %v", err.Type, ErrorTypeBadRequest)
			}
		})
	}
}
