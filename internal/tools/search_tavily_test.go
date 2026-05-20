package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTavilyBackend_Name(t *testing.T) {
	b := NewTavilyBackend("tvly-test", 10*time.Second)
	if b.Name() != "tavily" {
		t.Errorf("Name() = %q, want %q", b.Name(), "tavily")
	}
}

func TestTavilyBackend_Available_WithKey(t *testing.T) {
	b := NewTavilyBackend("tvly-test", 10*time.Second)
	if !b.Available() {
		t.Error("Available() should be true with API key")
	}
}

func TestTavilyBackend_Available_NoKey(t *testing.T) {
	b := NewTavilyBackend("", 10*time.Second)
	if b.Available() {
		t.Error("Available() should be false without API key")
	}
}

func TestTavilyBackend_Search_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		if req["api_key"] != "tvly-test" {
			t.Errorf("Expected api_key in request body")
		}

		resp := map[string]any{
			"results": []map[string]any{
				{
					"title":  "Go Programming",
					"url":    "https://go.dev",
					"content": "Go is an open source programming language",
					"score":  0.95,
				},
				{
					"title":  "Go Tour",
					"url":    "https://go.dev/tour",
					"content": "A Tour of Go",
					"score":  0.8,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-test", 10*time.Second)
	b.baseURL = ts.URL

	results, err := b.Search(context.Background(), "Go programming", SearchOptions{MaxResults: 5, IncludeContent: true})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Go Programming" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Go Programming")
	}
	if results[0].URL != "https://go.dev" {
		t.Errorf("URL = %q, want %q", results[0].URL, "https://go.dev")
	}
	if results[0].Score != 0.95 {
		t.Errorf("Score = %f, want %f", results[0].Score, 0.95)
	}
	if results[0].Content == "" {
		t.Error("Content should be included when IncludeContent=true")
	}
}

func TestTavilyBackend_Search_NoContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		if req["include_raw_content"] != false {
			t.Errorf("include_raw_content should be false when IncludeContent=false")
		}

		resp := map[string]any{
			"results": []map[string]any{
				{"title": "Test", "url": "https://example.com", "content": "snippet"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-test", 10*time.Second)
	b.baseURL = ts.URL

	results, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5, IncludeContent: false})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
}

func TestTavilyBackend_Search_401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"detail": "Invalid API key"})
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-bad-key", 10*time.Second)
	b.baseURL = ts.URL

	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for 401")
	}
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if se.Code != 401 {
		t.Errorf("Code = %d, want 401", se.Code)
	}
}

func TestTavilyBackend_Search_429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"detail": "Rate limited"})
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-test", 10*time.Second)
	b.baseURL = ts.URL

	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for 429")
	}
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if se.Code != 429 {
		t.Errorf("Code = %d, want 429", se.Code)
	}
}

func TestTavilyBackend_Search_5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-test", 10*time.Second)
	b.baseURL = ts.URL

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
}

func TestTavilyBackend_Search_EmptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"results": []map[string]any{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-test", 10*time.Second)
	b.baseURL = ts.URL

	results, err := b.Search(context.Background(), "obscure", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestTavilyBackend_Search_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	b := NewTavilyBackend("tvly-test", 10*time.Second)
	b.baseURL = ts.URL

	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for malformed JSON")
	}
}
