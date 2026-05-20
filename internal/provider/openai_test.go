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
				ID:       "o1",
				Provider: "openai",
				API:      "openai-responses",
				BaseURL:  server.URL,
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
				if !strings.Contains(string(body), `"thinking"`) {
					t.Fatalf("expected thinking in body, got: %s", string(body))
				}
				if !strings.Contains(string(body), fmt.Sprintf(`"effort":"%s"`, tt.wantEffort)) {
					t.Errorf("expected effort=%q in body, got: %s", tt.wantEffort, string(body))
				}
			} else {
				if strings.Contains(string(body), `"thinking"`) {
					t.Errorf("expected no thinking in body for level=%q, got: %s", tt.level, string(body))
				}
			}
		})
	}
}
