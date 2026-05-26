package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

// testModel returns a minimal model for testing.
func testModel(baseURL string) types.Model {
	return types.Model{
		ID:       "test-model",
		Name:     "Test",
		Provider: "test",
		API:      "openai-responses",
		BaseURL:  baseURL,
	}
}

func TestOpenAIProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test-key" {
			t.Errorf("expected Bearer token, got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}

		// Parse body
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["model"] != "test-model" {
			t.Errorf("expected model test-model, got %v", body["model"])
		}
		if body["stream"] != true {
			t.Errorf("expected stream=true, got %v", body["stream"])
		}

		// Send SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"Hello\"}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \" World\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 10, \"output_tokens\": 2, \"total_tokens\": 12}}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	// Check event sequence
	foundText := false
	foundDone := false
	for _, e := range events {
		if e.Type == types.EventTextDelta {
			foundText = true
		}
		if e.Type == types.EventDone {
			foundDone = true
		}
	}
	if !foundText {
		t.Error("expected text delta event")
	}
	if !foundDone {
		t.Error("expected done event")
	}
}

func TestOpenAIProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"Complete response\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 5, \"output_tokens\": 2, \"total_tokens\": 7}}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message, got nil")
	}

	// Check accumulated text
	var text string
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			text += block.Text
		}
	}
	if text != "Complete response" {
		t.Fatalf("expected 'Complete response', got %q", text)
	}
}

func TestOpenAIProvider_EmptyAPIKey(t *testing.T) {
	provider := NewOpenAIProvider("")
	ch := provider.Stream(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
}

func TestOpenAIProvider_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error": "internal error"}`)
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
}

func TestOpenAIProvider_RateLimitError(t *testing.T) {
	// This test verifies the provider correctly reports a 429 error
	// when the HTTP client returns a rate-limited response.
	// The actual retry logic is tested in http_test.go.
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     map[string][]string{"Retry-After": {"1"}},
				Body:       []byte("rate limited"),
			}, fmt.Errorf("rate limited after 3 retries")
		},
	}

	provider := NewOpenAIProviderWithClient("sk-test-key", client)
	model := testModel("http://localhost")

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event for rate limit, got %s", events[0].Type)
	}
}

func TestOpenAIProvider_Headers(t *testing.T) {
	var capturedHeaders http.Header
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	_ = provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})

	mu.Lock()
	defer mu.Unlock()
	if auth := capturedHeaders.Get("Authorization"); auth != "Bearer sk-test-key" {
		t.Fatalf("expected Bearer sk-test-key, got %s", auth)
	}
}

// Test that verifies the OpenAI provider sends the correct request body
func TestOpenAIProvider_RequestFormat(t *testing.T) {
	var receivedBody map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	messages := []types.AgentMessage{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: types.BlockText, Text: "Hello, world!"},
			},
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "You are a helpful assistant.",
		MaxTokens:    1000,
		Temperature:  0.7,
	}

	ch := provider.Stream(context.Background(), model, messages, nil, opts)
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if receivedBody["model"] != "test-model" {
		t.Errorf("expected model test-model, got %v", receivedBody["model"])
	}
	if receivedBody["stream"] != true {
		t.Errorf("expected stream=true, got %v", receivedBody["stream"])
	}
	if receivedBody["max_output_tokens"] != float64(1000) {
		t.Errorf("expected max_output_tokens=1000, got %v", receivedBody["max_output_tokens"])
	}
}

// testHTTPClient is an HTTPClient that makes real HTTP requests using net/http.
// It uses the URL from the Request directly, allowing httptest servers to work.
type testHTTPClient struct{}

func (c *testHTTPClient) Do(req *Request) (*Response, error) {
	method := req.Method
	if method == "" {
		method = http.MethodPost
	}

	var bodyReader interface{ Read([]byte) (int, error) }
	if req.Body != nil {
		bodyReader = &byteReader{data: req.Body}
	}

	var httpBody interface{ Read([]byte) (int, error) }
	if bodyReader != nil {
		httpBody = bodyReader
	}

	httpReq, err := http.NewRequest(method, req.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpBody != nil {
		httpReq.Body = &readCloser{r: httpBody}
		httpReq.ContentLength = int64(len(req.Body))
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := readAllBytes(resp.Body)
	if err != nil {
		return nil, err
	}

	headerMap := make(map[string][]string)
	for k, v := range resp.Header {
		headerMap[k] = v
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Header:     headerMap,
		Body:       bodyBytes,
	}, nil
}

type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

type readCloser struct {
	r interface{ Read([]byte) (int, error) }
}

func (r *readCloser) Read(p []byte) (int, error) { return r.r.Read(p) }
func (r *readCloser) Close() error               { return nil }

func readAllBytes(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var result []byte
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return result, nil
}

func TestOpenAIProvider_ThinkingMapping(t *testing.T) {
	tests := []struct {
		level       types.ThinkingLevel
		wantEffort  string
		wantEnabled bool
	}{
		{types.ThinkingOff, "", false},
		{types.ThinkingLow, "low", true},
		{types.ThinkingMedium, "medium", true},
		{types.ThinkingHigh, "high", true},
		{types.ThinkingXHigh, "xhigh", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			var capturedBody []byte
			var mu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				capturedBody, _ = io.ReadAll(r.Body)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
			}))
			defer server.Close()

			model := types.Model{
				ID:        "o1",
				Provider:  "openai",
				API:       "openai-responses",
				BaseURL:   server.URL,
				Reasoning: true,
				ThinkingLevelMap: map[string]string{
					"off": "none", "low": "low", "medium": "medium",
					"high": "high", "xhigh": "xhigh",
				},
			}
			provider := NewOpenAIProviderWithClient("sk-test", &testHTTPClient{})

			ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{
				ThinkingLevel: tt.level,
			})
			// Consume the channel
			for range ch {
			}

			mu.Lock()
			body := capturedBody
			mu.Unlock()

			if tt.wantEnabled {
				if !strings.Contains(string(body), `"reasoning"`) {
					t.Fatalf("expected reasoning in body, got: %s", string(body))
				}
				if !strings.Contains(string(body), fmt.Sprintf(`"effort":"%s"`, tt.wantEffort)) {
					t.Errorf("expected effort=%q in body, got: %s", tt.wantEffort, string(body))
				}
			} else {
				if strings.Contains(string(body), `"reasoning"`) {
					t.Errorf("expected no reasoning in body for level=%q, got: %s", tt.level, string(body))
				}
			}
		})
	}
}

func TestOpenAIProvider_IsOAuth_APIKeyMode(t *testing.T) {
	p := NewOpenAIProvider("sk-test-key")
	if p.isOAuth() {
		t.Fatal("expected API key mode, not OAuth")
	}
}

func TestOpenAIProvider_IsOAuth_OAuthMode(t *testing.T) {
	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProvider(creds)
	if !p.isOAuth() {
		t.Fatal("expected OAuth mode")
	}
}

func TestOpenAIProvider_OAuth_URLRewriting(t *testing.T) {
	var capturedURL string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedURL = r.URL.Path
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken: makeTestJWT(t, map[string]any{
			"chatgpt_account_id": "acct-123",
		}),
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if capturedURL != "/responses" {
		t.Fatalf("expected /responses path, got %s", capturedURL)
	}
}

func TestOpenAIProvider_OAuth_Headers(t *testing.T) {
	var capturedHeaders http.Header
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if auth := capturedHeaders.Get("Authorization"); auth != "Bearer test-access-token" {
		t.Fatalf("expected Bearer test-access-token, got %s", auth)
	}
	if originator := capturedHeaders.Get("originator"); originator != "tau" {
		t.Fatalf("expected originator=tau, got %s", originator)
	}
	if accountID := capturedHeaders.Get("chatgpt-account-id"); accountID != "acct-123" {
		t.Fatalf("expected chatgpt-account-id=acct-123, got %s", accountID)
	}
}

func TestOpenAIProvider_OAuth_BodyInstructions(t *testing.T) {
	var receivedBody map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		json.NewDecoder(r.Body).Decode(&receivedBody)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	opts := types.StreamOptions{
		SystemPrompt: "You are a helpful assistant.",
	}
	ch := p.Stream(context.Background(), model, nil, nil, opts)
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if receivedBody["instructions"] != "You are a helpful assistant." {
		t.Fatalf("expected instructions, got %v", receivedBody["instructions"])
	}

	input, ok := receivedBody["input"].([]any)
	if ok && len(input) > 0 {
		t.Fatalf("expected no input for system prompt in OAuth mode, got %v", input)
	}
}

func TestOpenAIProvider_OAuth_StoreFalse(t *testing.T) {
	var receivedBody map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		json.NewDecoder(r.Body).Decode(&receivedBody)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	store, ok := receivedBody["store"]
	if !ok {
		t.Fatal("expected store field in body")
	}
	if store != false {
		t.Fatalf("expected store=false, got %v", store)
	}
}

func TestOpenAIProvider_OAuth_NoStoreInAPIKeyMode(t *testing.T) {
	var receivedBody map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		json.NewDecoder(r.Body).Decode(&receivedBody)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	p := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if _, ok := receivedBody["store"]; ok {
		t.Fatal("expected no store field in API key mode")
	}
}

func TestOpenAIProvider_OAuth_ErrorClassification_RateLimit(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       []byte(`{"error": {"type": "rate_limit", "message": "rate limit exceeded"}}`),
			}, nil
		},
	}

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClient(creds, client)

	model := testModel("http://localhost")
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
	if !strings.Contains(events[0].Error, "Rate limit") {
		t.Fatalf("expected rate limit message, got %s", events[0].Error)
	}
}

func TestOpenAIProvider_OAuth_ErrorClassification_QuotaExceeded(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: http.StatusForbidden,
				Body:       []byte(`{"error": {"type": "insufficient_quota"}}`),
			}, nil
		},
	}

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClient(creds, client)

	model := testModel("http://localhost")
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
	if !strings.Contains(events[0].Error, "usage limit") {
		t.Fatalf("expected quota exceeded message, got %s", events[0].Error)
	}
}

func TestOpenAIProvider_OAuth_ErrorClassification_InvalidToken(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: http.StatusUnauthorized,
				Body:       []byte(`{"error": {"type": "invalid_token"}}`),
			}, nil
		},
	}

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClient(creds, client)

	model := testModel("http://localhost")
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
	if !strings.Contains(events[0].Error, "token expired") {
		t.Fatalf("expected invalid token message, got %s", events[0].Error)
	}
}

func TestOpenAIProvider_OAuth_TokenRefresh(t *testing.T) {
	refreshed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "refreshed-access-token",
				"refresh_token": "refreshed-refresh-token",
				"expires_in":    3600,
			})
			refreshed = true
			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer refreshed-access-token" {
			t.Errorf("expected refreshed token, got %s", auth)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	oldURL := OAuthTokenURL
	defer func() { OAuthTokenURL = oldURL }()
	OAuthTokenURL = server.URL + "/oauth/token"

	creds := OAuthCredentials{
		AccessToken:  "expired-token",
		RefreshToken: "old-refresh-token",
		Expires:      time.Now().Add(-1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClient(creds, &testHTTPClient{})

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	if !refreshed {
		t.Fatal("expected token to be refreshed")
	}
}

func TestOpenAIProvider_OAuth_NoAccountIDHeader(t *testing.T) {
	var capturedHeaders http.Header
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if capturedHeaders.Get("chatgpt-account-id") != "" {
		t.Fatalf("expected no chatgpt-account-id header when AccountID is empty")
	}
}

func TestOpenAIProvider_OAuth_OriginatorAlwaysSet(t *testing.T) {
	var capturedHeaders http.Header
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	ch := p.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	mu.Lock()
	defer mu.Unlock()

	if capturedHeaders.Get("originator") != "tau" {
		t.Fatalf("expected originator=tau, got %s", capturedHeaders.Get("originator"))
	}
}

func TestOpenAIProvider_OAuth_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"OAuth Complete\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 5, \"output_tokens\": 2, \"total_tokens\": 7}}\n\n")
	}))
	defer server.Close()

	creds := OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "refresh",
		Expires:      time.Now().Add(1 * time.Hour).Unix(),
		AccountID:    "acct-123",
	}
	p := NewOpenAIOAuthProviderWithClientAndCodexURL(creds, &testHTTPClient{}, server.URL)

	model := testModel(server.URL)
	msg, err := p.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message, got nil")
	}

	var text string
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			text += block.Text
		}
	}
	if text != "OAuth Complete" {
		t.Fatalf("expected 'OAuth Complete', got %q", text)
	}
}

// NewOpenAIOAuthProviderWithClient creates a new OpenAI provider with OAuth credentials and a custom HTTP client.
func NewOpenAIOAuthProviderWithClient(creds OAuthCredentials, client HTTPClient) *OpenAIProvider {
	return &OpenAIProvider{
		baseProvider: baseProvider{
			name:       "openai-oauth",
			httpClient: client,
		},
		oauthManager: NewOAuthManager(creds, nil),
	}
}

// NewOpenAIOAuthProviderWithClientAndCodexURL creates a new OpenAI provider with OAuth credentials,
// a custom HTTP client, and a configurable Codex base URL for testing.
func NewOpenAIOAuthProviderWithClientAndCodexURL(creds OAuthCredentials, client HTTPClient, codexURL string) *OpenAIProvider {
	return &OpenAIProvider{
		baseProvider: baseProvider{
			name:       "openai-oauth",
			httpClient: client,
		},
		oauthManager: NewOAuthManager(creds, nil),
		codexBaseURL: codexURL,
	}
}

func TestCodexModels_Count(t *testing.T) {
	models := CodexModels()
	if len(models) != 6 {
		t.Fatalf("expected 6 Codex models, got %d", len(models))
	}
}

func TestCodexModels_ExpectedIDs(t *testing.T) {
	wantIDs := []string{
		"gpt-5.5", "gpt-5.4", "gpt-5.4-mini",
		"gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2",
	}
	models := CodexModels()
	for i, want := range wantIDs {
		if models[i].ID != want {
			t.Errorf("model[%d].ID = %q, want %q", i, models[i].ID, want)
		}
	}
}

func TestCodexModels_ZeroCost(t *testing.T) {
	models := CodexModels()
	for _, m := range models {
		if m.Cost.Input != 0 {
			t.Errorf("%s: cost input = %f, want 0", m.ID, m.Cost.Input)
		}
		if m.Cost.Output != 0 {
			t.Errorf("%s: cost output = %f, want 0", m.ID, m.Cost.Output)
		}
		if m.Cost.CacheRead != 0 {
			t.Errorf("%s: cost cache_read = %f, want 0", m.ID, m.Cost.CacheRead)
		}
		if m.Cost.CacheWrite != 0 {
			t.Errorf("%s: cost cache_write = %f, want 0", m.ID, m.Cost.CacheWrite)
		}
	}
}

func TestCodexModels_Gpt55ContextWindow(t *testing.T) {
	models := CodexModels()
	var gpt55 *types.Model
	for i := range models {
		if models[i].ID == "gpt-5.5" {
			gpt55 = &models[i]
			break
		}
	}
	if gpt55 == nil {
		t.Fatal("gpt-5.5 not found in Codex models")
	}
	if gpt55.ContextWindow != 400000 {
		t.Errorf("gpt-5.5 context window = %d, want 400000", gpt55.ContextWindow)
	}
	if gpt55.MaxTokens != 128000 {
		t.Errorf("gpt-5.5 max tokens = %d, want 128000", gpt55.MaxTokens)
	}
}

func TestCodexModels_AllHaveReasoning(t *testing.T) {
	models := CodexModels()
	for _, m := range models {
		if !m.Reasoning {
			t.Errorf("%s: Reasoning = false, want true", m.ID)
		}
	}
}

func TestCodexModels_ThinkingLevelMap(t *testing.T) {
	models := CodexModels()
	wantOff := "none"
	wantXHigh := "xhigh"
	wantMinimal := "minimal"

	for _, m := range models {
		if m.ThinkingLevelMap == nil {
			t.Fatalf("%s: ThinkingLevelMap is nil", m.ID)
		}
		if m.ThinkingLevelMap["off"] != wantOff {
			t.Errorf("%s: off = %q, want %q", m.ID, m.ThinkingLevelMap["off"], wantOff)
		}
		if m.ThinkingLevelMap["xhigh"] != wantXHigh {
			t.Errorf("%s: xhigh = %q, want %q", m.ID, m.ThinkingLevelMap["xhigh"], wantXHigh)
		}
		if m.ThinkingLevelMap["minimal"] != wantMinimal {
			t.Errorf("%s: minimal = %q, want %q", m.ID, m.ThinkingLevelMap["minimal"], wantMinimal)
		}
	}
}

func TestCodexModels_InputTypes(t *testing.T) {
	models := CodexModels()
	for _, m := range models {
		if len(m.InputTypes) != 1 || m.InputTypes[0] != "text" {
			t.Errorf("%s: InputTypes = %v, want [text]", m.ID, m.InputTypes)
		}
	}
}

func TestCodexModels_API(t *testing.T) {
	models := CodexModels()
	for _, m := range models {
		if m.API != "openai-completions" {
			t.Errorf("%s: API = %q, want openai-completions", m.ID, m.API)
		}
	}
}

func TestOpenAIProvider_Stream_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `event: response.output_item.added
data: {"item": {"id": "fc_abc123", "type": "function_call", "name": "search", "call_id": "call_xyz", "arguments": ""}}

`)
		fmt.Fprint(w, `event: response.function_call_arguments.delta
data: {"delta": "{\"query\": \"scooters\"}"}

`)
		fmt.Fprint(w, `event: response.function_call_arguments.done
data: {"arguments": "{\"query\": \"scooters\"}"}

`)
		fmt.Fprint(w, `event: response.completed
data: {"usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}

`)
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	var foundToolCallStart, foundToolCallEnd, foundDone bool
	for _, e := range events {
		switch e.Type {
		case types.EventToolCallStart:
			foundToolCallStart = true
			if e.Delta != "search" {
				t.Errorf("tool call start delta = %q, want 'search'", e.Delta)
			}
		case types.EventToolCallEnd:
			foundToolCallEnd = true
		case types.EventDone:
			foundDone = true
			if e.Message == nil {
				t.Fatal("done event has no message")
			}
			toolCalls := 0
			for _, block := range e.Message.Content {
				if block.Type == types.BlockToolCall {
					toolCalls++
					if block.ToolCall.ID != "call_xyz|fc_abc123" {
						t.Errorf("tool call ID = %q, want 'call_xyz|fc_abc123'", block.ToolCall.ID)
					}
					if block.ToolCall.Name != "search" {
						t.Errorf("tool call name = %q, want 'search'", block.ToolCall.Name)
					}
					if q, ok := block.ToolCall.Arguments["query"]; !ok || q != "scooters" {
						t.Errorf("tool call arguments = %v, want query=scooters", block.ToolCall.Arguments)
					}
				}
			}
			if toolCalls != 1 {
				t.Errorf("expected 1 tool call block, got %d", toolCalls)
			}
		}
	}
	if !foundToolCallStart {
		t.Error("expected EventToolCallStart")
	}
	if !foundToolCallEnd {
		t.Error("expected EventToolCallEnd")
	}
	if !foundDone {
		t.Error("expected EventDone")
	}
}

func TestOpenAIProvider_Stream_ReasoningAndToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `event: response.reasoning_summary_text.delta
data: {"delta": "Let me search for scooters"}

`)
		fmt.Fprint(w, `event: response.output_item.added
data: {"item": {"id": "fc_def456", "type": "function_call", "name": "search", "call_id": "call_123", "arguments": ""}}

`)
		fmt.Fprint(w, `event: response.function_call_arguments.delta
data: {"delta": "{\"q\":\"electric scooter\"}"}

`)
		fmt.Fprint(w, `event: response.function_call_arguments.done
data: {"arguments": "{\"q\":\"electric scooter\"}"}

`)
		fmt.Fprint(w, `event: response.completed
data: {"usage": {"input_tokens": 20, "output_tokens": 10, "total_tokens": 30}}

`)
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	var foundThinking, foundToolCall, foundDone bool
	for _, e := range events {
		switch e.Type {
		case types.EventThinkingDelta:
			foundThinking = true
		case types.EventToolCallEnd:
			foundToolCall = true
		case types.EventDone:
			foundDone = true
			if e.Message == nil {
				t.Fatal("done event has no message")
			}
			hasThinking := false
			hasToolCall := false
			for _, block := range e.Message.Content {
				if block.Type == types.BlockThinking {
					hasThinking = true
				}
				if block.Type == types.BlockToolCall {
					hasToolCall = true
				}
			}
			if !hasThinking {
				t.Error("message should contain thinking block")
			}
			if !hasToolCall {
				t.Error("message should contain tool call block")
			}
		}
	}
	if !foundThinking {
		t.Error("expected EventThinkingDelta")
	}
	if !foundToolCall {
		t.Error("expected EventToolCallEnd")
	}
	if !foundDone {
		t.Error("expected EventDone")
	}
}

func TestMessageToOpenAI_UserMessage(t *testing.T) {
	msg := types.AgentMessage{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "Hello"},
		},
	}
	items := messageToOpenAI(msg)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	m, ok := items[0].(openAIInputMessage)
	if !ok {
		t.Fatalf("expected openAIInputMessage, got %T", items[0])
	}
	if m.Role != "user" {
		t.Errorf("role = %q, want 'user'", m.Role)
	}
	if len(m.Content) != 1 || m.Content[0].Text != "Hello" {
		t.Errorf("content = %v, want [{input_text Hello}]", m.Content)
	}
}

func TestMessageToOpenAI_AssistantWithToolCall(t *testing.T) {
	msg := types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "Let me search"},
			{
				Type: types.BlockToolCall,
				ToolCall: &types.ToolCallBlock{
					ID:        "call_abc|fc_xyz",
					Name:      "search",
					Arguments: map[string]any{"q": "scooter"},
				},
			},
		},
	}
	items := messageToOpenAI(msg)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	textMsg, ok := items[0].(openAIInputMessage)
	if !ok {
		t.Fatalf("expected openAIInputMessage, got %T", items[0])
	}
	if textMsg.Role != "assistant" {
		t.Errorf("role = %q, want 'assistant'", textMsg.Role)
	}
	if len(textMsg.Content) != 1 || textMsg.Content[0].Text != "Let me search" {
		t.Errorf("text content = %v", textMsg.Content)
	}
	fcItem, ok := items[1].(openAIFunctionCallItem)
	if !ok {
		t.Fatalf("expected openAIFunctionCallItem, got %T", items[1])
	}
	if fcItem.CallID != "call_abc" {
		t.Errorf("call_id = %q, want 'call_abc'", fcItem.CallID)
	}
	if fcItem.ID != "" {
		t.Errorf("id = %q, want empty; replayed function_call IDs require paired reasoning items", fcItem.ID)
	}
	if fcItem.Name != "search" {
		t.Errorf("name = %q, want 'search'", fcItem.Name)
	}
}

func TestOpenAIProvider_RequestOmitsFunctionCallItemIDOnReplay(t *testing.T) {
	provider := NewOpenAIProvider("sk-test-key")
	model := testModel("http://localhost")

	body, err := provider.buildStreamRequest(model, []types.AgentMessage{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type: types.BlockThinking,
					Text: "reasoning summary",
				},
				{
					Type: types.BlockToolCall,
					ToolCall: &types.ToolCallBlock{
						ID:        "call_abc|fc_xyz",
						Name:      "search",
						Arguments: map[string]any{"q": "scooter"},
					},
				},
			},
		},
		{
			Role:       types.RoleToolResult,
			ToolCallID: "call_abc|fc_xyz",
			Content: []types.ContentBlock{
				{Type: types.BlockText, Text: "Found 3 scooters"},
			},
		},
	}, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("buildStreamRequest returned error: %v", err)
	}

	var req openAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	var foundCall, foundOutput bool
	for _, item := range req.Input {
		b, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("failed to marshal input item: %v", err)
		}

		var fields map[string]any
		if err := json.Unmarshal(b, &fields); err != nil {
			t.Fatalf("failed to unmarshal input item: %v", err)
		}

		switch fields["type"] {
		case "function_call":
			foundCall = true
			if got := fields["call_id"]; got != "call_abc" {
				t.Errorf("function_call call_id = %v, want call_abc", got)
			}
			if _, ok := fields["id"]; ok {
				t.Errorf("function_call replay should omit id, got item: %s", string(b))
			}
		case "function_call_output":
			foundOutput = true
			if got := fields["call_id"]; got != "call_abc" {
				t.Errorf("function_call_output call_id = %v, want call_abc", got)
			}
		}
	}

	if !foundCall {
		t.Fatal("expected function_call input item")
	}
	if !foundOutput {
		t.Fatal("expected function_call_output input item")
	}
}

func TestMessageToOpenAI_ToolResult(t *testing.T) {
	msg := types.AgentMessage{
		Role:       types.RoleToolResult,
		ToolCallID: "call_abc|fc_xyz",
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "Found 3 scooters"},
		},
	}
	items := messageToOpenAI(msg)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	fcOut, ok := items[0].(openAIFunctionCallOutput)
	if !ok {
		t.Fatalf("expected openAIFunctionCallOutput, got %T", items[0])
	}
	if fcOut.CallID != "call_abc" {
		t.Errorf("call_id = %q, want 'call_abc'", fcOut.CallID)
	}
	if fcOut.Output != "Found 3 scooters" {
		t.Errorf("output = %q, want 'Found 3 scooters'", fcOut.Output)
	}
}

func TestMessageToOpenAI_AssistantTextOnly(t *testing.T) {
	msg := types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "Hello world"},
		},
	}
	items := messageToOpenAI(msg)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	textMsg, ok := items[0].(openAIInputMessage)
	if !ok {
		t.Fatalf("expected openAIInputMessage, got %T", items[0])
	}
	if textMsg.Role != "assistant" {
		t.Errorf("role = %q, want 'assistant'", textMsg.Role)
	}
	if len(textMsg.Content) != 1 || textMsg.Content[0].Text != "Hello world" {
		t.Errorf("content = %v", textMsg.Content)
	}
}

func TestMessageToOpenAI_AssistantEmpty(t *testing.T) {
	msg := types.AgentMessage{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{},
	}
	items := messageToOpenAI(msg)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty assistant message, got %d", len(items))
	}
}
