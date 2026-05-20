package provider

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultHTTPClient_RetryOn500(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	client := &DefaultHTTPClient{}
	resp, err := client.Do(&Request{
		Method: "GET",
		URL:    server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestDefaultHTTPClient_RetryOn429(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	client := &DefaultHTTPClient{}
	resp, err := client.Do(&Request{
		Method: "GET",
		URL:    server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestDefaultHTTPClient_ExhaustedRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &DefaultHTTPClient{}
	_, err := client.Do(&Request{
		Method: "GET",
		URL:    server.URL,
	})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if atomic.LoadInt32(&attempts) != 3 { // 1 initial + 2 retries
		t.Fatalf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestDefaultHTTPClient_NetworkError(t *testing.T) {
	client := &DefaultHTTPClient{}
	_, err := client.Do(&Request{
		Method: "GET",
		URL:    "http://localhost:1", // Connection refused
	})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestDefaultHTTPClient_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	client := &DefaultHTTPClient{}
	resp, err := client.Do(&Request{
		Method: "GET",
		URL:    server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"ok": true}` {
		t.Fatalf("expected body, got %s", string(resp.Body))
	}
}

func TestDefaultHTTPClient_PostWithBody(t *testing.T) {
	var receivedBody []byte
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := make([]byte, 1024)
		n, _ := r.Body.Read(data)
		mu.Lock()
		receivedBody = data[:n]
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &DefaultHTTPClient{}
	resp, err := client.Do(&Request{
		Method: "POST",
		URL:    server.URL,
		Body:   []byte(`{"key": "value"}`),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if string(receivedBody) != `{"key": "value"}` {
		t.Fatalf("expected body %q, got %q", `{"key": "value"}`, string(receivedBody))
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestParseRetryAfterHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   http.Header
		expected time.Duration
	}{
		{
			name:     "seconds",
			header:   http.Header{"Retry-After": []string{"30"}},
			expected: 30 * time.Second,
		},
		{
			name:     "empty",
			header:   http.Header{},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfterHeader(tc.header)
			// For seconds, allow some tolerance
			if tc.expected > 0 {
				if got < tc.expected-time.Second || got > tc.expected+time.Second {
					t.Fatalf("expected ~%v, got %v", tc.expected, got)
				}
			} else {
				if got != 0 {
					t.Fatalf("expected 0, got %v", got)
				}
			}
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	d0 := backoffDelay(0)
	if d0 != initialBackoff {
		t.Fatalf("expected %v, got %v", initialBackoff, d0)
	}

	d1 := backoffDelay(1)
	if d1 != 2*time.Second {
		t.Fatalf("expected 2s, got %v", d1)
	}

	d2 := backoffDelay(2)
	if d2 != 4*time.Second {
		t.Fatalf("expected 4s, got %v", d2)
	}
}

func TestTruncateString(t *testing.T) {
	if truncateString("hello", 10) != "hello" {
		t.Fatal("short string should not be truncated")
	}
	if truncateString("hello world", 5) != "hello..." {
		t.Fatalf("expected hello..., got %s", truncateString("hello world", 5))
	}
}

func TestIsRetryableRateLimit_Retryable(t *testing.T) {
	retryable := []string{
		`{"error": "Rate limit exceeded"}`,
		`{"message": "Too many requests"}`,
		`{"error": "Rate limit, try again later"}`,
	}
	for _, body := range retryable {
		if !isRetryableRateLimit([]byte(body)) {
			t.Errorf("expected %q to be retryable", body)
		}
	}
}

func TestIsRetryableRateLimit_NonRetryable(t *testing.T) {
	nonRetryable := []string{
		`{"error": "Weekly limit reached"}`,
		`{"error": "Quota exceeded"}`,
		`{"error": "Monthly limit exceeded"}`,
		`{"error": "Daily limit exceeded"}`,
		`{"error": "Insufficient credits"}`,
		`{"error": "Upgrade your subscription"}`,
	}
	for _, body := range nonRetryable {
		if isRetryableRateLimit([]byte(body)) {
			t.Errorf("expected %q to be non-retryable", body)
		}
	}
}

func TestIsRetryableRateLimit_CaseInsensitive(t *testing.T) {
	if isRetryableRateLimit([]byte(`{"error": "QUOTA EXCEEDED"}`)) {
		t.Error("should detect 'QUOTA' case-insensitively")
	}
	if isRetryableRateLimit([]byte(`{"error": "Weekly Limit Reached"}`)) {
		t.Error("should detect 'Weekly Limit' case-insensitively")
	}
}
