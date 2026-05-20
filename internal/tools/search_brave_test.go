package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBraveBackend_Name(t *testing.T) {
	b := NewBraveBackend("BSA-test", 10*time.Second)
	if b.Name() != "brave" {
		t.Errorf("Name() = %q, want %q", b.Name(), "brave")
	}
}

func TestBraveBackend_Available_WithKey(t *testing.T) {
	b := NewBraveBackend("BSA-test", 10*time.Second)
	if !b.Available() {
		t.Error("Available() should be true with API key")
	}
}

func TestBraveBackend_Available_NoKey(t *testing.T) {
	b := NewBraveBackend("", 10*time.Second)
	if b.Available() {
		t.Error("Available() should be false without API key")
	}
}

func TestBraveBackend_Search_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-Subscription-Token") != "BSA-test" {
			t.Errorf("Expected X-Subscription-Token header")
		}
		if r.URL.Query().Get("q") != "Go programming" {
			t.Errorf("Expected q parameter, got %s", r.URL.Query().Get("q"))
		}

		resp := map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{
						"title":       "Go Programming Language",
						"url":         "https://go.dev",
						"description": "Go is an open source programming language",
					},
					{
						"title":       "Go Tour",
						"url":         "https://go.dev/tour",
						"description": "A Tour of Go",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewBraveBackend("BSA-test", 10*time.Second)
	b.baseURL = ts.URL

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
	if results[0].URL != "https://go.dev" {
		t.Errorf("URL = %q, want %q", results[0].URL, "https://go.dev")
	}
	if results[0].Snippet != "Go is an open source programming language" {
		t.Errorf("Snippet = %q, unexpected", results[0].Snippet)
	}
}

func TestBraveBackend_Search_401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	b := NewBraveBackend("BSA-bad", 10*time.Second)
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

func TestBraveBackend_Search_429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	b := NewBraveBackend("BSA-test", 10*time.Second)
	b.baseURL = ts.URL

	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if se.Code != 429 {
		t.Errorf("Code = %d, want 429", se.Code)
	}
}

func TestBraveBackend_Search_5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	b := NewBraveBackend("BSA-test", 10*time.Second)
	b.baseURL = ts.URL

	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	se, ok := err.(*searchError)
	if !ok {
		t.Fatalf("Expected *searchError, got %T", err)
	}
	if se.Code != 502 {
		t.Errorf("Code = %d, want 502", se.Code)
	}
}

func TestBraveBackend_Search_EmptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"web": map[string]any{
				"results": []map[string]any{},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	b := NewBraveBackend("BSA-test", 10*time.Second)
	b.baseURL = ts.URL

	results, err := b.Search(context.Background(), "obscure", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestBraveBackend_Search_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	b := NewBraveBackend("BSA-test", 10*time.Second)
	b.baseURL = ts.URL

	_, err := b.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("Expected error for malformed JSON")
	}
}
