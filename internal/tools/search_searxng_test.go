package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearXNGBackend_Name(t *testing.T) {
	b := NewSearXNGBackend("http://localhost:8964", 10*time.Second)
	if b.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", b.Name(), "searxng")
	}
}

func TestSearXNGBackend_Search_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("q") != "Go programming" {
			t.Errorf("Expected q parameter, got %s", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("Expected format=json, got %s", r.URL.Query().Get("format"))
		}

		resp := searxngResponse{
			Results: []searxngResult{
				{Title: "Go Programming Language", URL: "https://go.dev", Content: "Go is an open source programming language"},
				{Title: "Go Tour", URL: "https://go.dev/tour", Content: "A Tour of Go"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	results, err := b.Search(context.Background(), "Go programming", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Go Programming Language" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Go Programming Language")
	}
	if results[0].Snippet != "Go is an open source programming language" {
		t.Errorf("Snippet = %q, unexpected", results[0].Snippet)
	}
}

func TestSearXNGBackend_Search_MaxResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := searxngResponse{
			Results: []searxngResult{
				{Title: "R1", URL: "https://a.com", Content: "A"},
				{Title: "R2", URL: "https://b.com", Content: "B"},
				{Title: "R3", URL: "https://c.com", Content: "C"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	results, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("Expected at most 2 results, got %d", len(results))
	}
}

func TestSearXNGBackend_Search_5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for 500")
	}
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if se.Code != 500 {
		t.Errorf("Code = %d, want 500", se.Code)
	}
	if !contains(se.Message, "server error") {
		t.Errorf("Message should contain 'server error', got: %s", se.Message)
	}
}

func TestSearXNGBackend_Search_EmptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := searxngResponse{Results: []searxngResult{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	results, err := b.Search(context.Background(), "obscure", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestSearXNGBackend_Search_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for malformed JSON")
	}
}

func TestSearXNGBackend_Search_ConnectionRefused(t *testing.T) {
	b := NewSearXNGBackend("http://localhost:1", 100*time.Millisecond)
	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for connection refused")
	}
}

func TestSearXNGBackend_Search_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Search(ctx, "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for timeout")
	}
}

func TestSearXNGBackend_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	b := NewSearXNGBackend("http://localhost:8964", 10*time.Second)
	if !b.Available() {
		t.Skip("SearXNG not running at localhost:8964")
	}

	results, err := b.Search(context.Background(), "Go programming language", SearchOptions{MaxResults: 3})
	if err != nil {
		t.Fatalf("Integration search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least one result from SearXNG")
	}
	t.Logf("Got %d results from SearXNG", len(results))
	for i, r := range results {
		t.Logf("  %d: %s (%s)", i+1, r.Title, r.URL)
	}
}

func TestSearXNGBackend_Search_400WithBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "JSON format not enabled. Check your settings.yml."}`))
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for 400")
	}
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if se.Code != 400 {
		t.Errorf("Code = %d, want 400", se.Code)
	}
	if !contains(se.Message, "JSON format not enabled") {
		t.Errorf("Error message should include response body, got: %s", se.Message)
	}
}

func TestSearXNGBackend_Search_4xxWithBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "API key required"}`))
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for 403")
	}
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if !contains(se.Message, "API key required") {
		t.Errorf("Error message should include response body, got: %s", se.Message)
	}
}

func TestSearXNGBackend_Search_5xxWithBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 10*time.Second)
	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for 500")
	}
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if !contains(se.Message, "Internal server error") {
		t.Errorf("Error message should include response body, got: %s", se.Message)
	}
}

func TestSearXNGBackend_Available_400ReturnsFalse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 2*time.Second)
	if b.Available() {
		t.Error("Available() should return false for 400 status")
	}
}

func TestSearXNGBackend_Available_200ReturnsTrue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	b := NewSearXNGBackend(ts.URL, 2*time.Second)
	if !b.Available() {
		t.Error("Available() should return true for 200 status")
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{"empty", []byte{}, "(empty body)"},
		{"whitespace only", []byte("   \n\t  "), "(empty body)"},
		{"short body", []byte("error message"), "error message"},
		{"trims spaces", []byte("  hello  "), "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBody(tt.body)
			if got != tt.want {
				t.Errorf("truncateBody() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("long body truncated", func(t *testing.T) {
		longBody := make([]byte, 600)
		for i := range longBody {
			longBody[i] = 'a'
		}
		got := truncateBody(longBody)
		if len(got) != 512+3 {
			t.Errorf("truncateBody() length = %d, want %d", len(got), 512+3)
		}
		if !contains(got, "...") {
			t.Error("truncated body should end with ...")
		}
	})
}
