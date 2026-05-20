package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

type mockBackend struct {
	name      string
	available bool
	results   []SearchResult
	err       error
	called    atomic.Int32
}

func (m *mockBackend) Name() string         { return m.name }
func (m *mockBackend) Available() bool       { return m.available }
func (m *mockBackend) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	m.called.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool([]SearchBackend{&mockBackend{name: "test", available: true}}, time.Now())
	if tool.Name() != "websearch" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "websearch")
	}
}

func TestWebSearchTool_ExecutionMode(t *testing.T) {
	tool := NewWebSearchTool([]SearchBackend{&mockBackend{name: "test", available: true}}, time.Now())
	if tool.ExecutionMode() != types.ExecutionParallel {
		t.Errorf("ExecutionMode() = %v, want %v", tool.ExecutionMode(), types.ExecutionParallel)
	}
}

func TestWebSearchTool_DescriptionContainsDate(t *testing.T) {
	date := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	tool := NewWebSearchTool([]SearchBackend{&mockBackend{name: "test", available: true}}, date)
	desc := tool.Description()
	if !contains(desc, "2026") {
		t.Errorf("Description() should contain current date, got: %s", desc)
	}
}

func TestWebSearchTool_DescriptionEnforcesNaming(t *testing.T) {
	tool := NewWebSearchTool([]SearchBackend{&mockBackend{name: "test", available: true}}, time.Now())
	desc := tool.Description()
	if !contains(desc, "websearch") {
		t.Error("Description should mention exact tool name 'websearch'")
	}
	if !contains(desc, "google_search") {
		t.Error("Description should discourage alias 'google_search'")
	}
}

func TestWebSearchTool_DescriptionIntegrityRules(t *testing.T) {
	tool := NewWebSearchTool([]SearchBackend{&mockBackend{name: "test", available: true}}, time.Now())
	desc := tool.Description()
	if !contains(desc, "URL") || !contains(desc, "fabricat") {
		t.Error("Description should include integrity rules about citing URLs and not fabricating")
	}
}

func TestNewWebSearchTool_NilWhenNoBackends(t *testing.T) {
	tool := NewWebSearchTool(nil, time.Now())
	if tool != nil {
		t.Error("NewWebSearchTool with no backends should return nil")
	}

	tool = NewWebSearchTool([]SearchBackend{}, time.Now())
	if tool != nil {
		t.Error("NewWebSearchTool with empty backends should return nil")
	}
}

func TestWebSearchTool_Execute_PrimarySucceeds(t *testing.T) {
	b1 := &mockBackend{
		name:      "primary",
		available: true,
		results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com", Snippet: "test"},
		},
	}
	b2 := &mockBackend{name: "fallback", available: true}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())
	params := &WebSearchParams{Query: "test", MaxResults: 8}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error result: %s", result.Content[0].Text)
	}
	if b1.called.Load() != 1 {
		t.Error("Primary backend should be called")
	}
	if b2.called.Load() != 0 {
		t.Error("Fallback backend should NOT be called when primary succeeds")
	}
}

func TestWebSearchTool_Execute_FallbackOnFailure(t *testing.T) {
	b1 := &mockBackend{name: "primary", available: true, err: fmt.Errorf("network error")}
	b2 := &mockBackend{
		name:      "fallback",
		available: true,
		results: []SearchResult{
			{Title: "Fallback Result", URL: "https://fallback.com", Snippet: "ok"},
		},
	}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())
	params := &WebSearchParams{Query: "test"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error result: %s", result.Content[0].Text)
	}
	if b2.called.Load() != 1 {
		t.Error("Fallback backend should be called when primary fails")
	}
}

func TestWebSearchTool_Execute_AllBackendsFail(t *testing.T) {
	b1 := &mockBackend{name: "primary", available: true, err: fmt.Errorf("network error")}
	b2 := &mockBackend{name: "secondary", available: true, err: fmt.Errorf("auth error")}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())
	params := &WebSearchParams{Query: "test"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("Result should be an error when all backends fail")
	}
}

func TestWebSearchTool_Execute_EmptyResultsNotError(t *testing.T) {
	b := &mockBackend{name: "test", available: true, results: []SearchResult{}}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	params := &WebSearchParams{Query: "obscure query"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Error("Empty results should not be an error")
	}
}

func TestWebSearchTool_Execute_ContextCancellation(t *testing.T) {
	b := &mockBackend{
		name:      "slow",
		available: true,
		err:       context.Canceled,
	}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	params := &WebSearchParams{Query: "test"}
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("Cancelled context should produce error result")
	}
}

func TestWebSearchTool_Execute_DegradedBackend(t *testing.T) {
	b1 := &mockBackend{name: "bad_auth", available: true, err: &searchError{Code: 401, Message: "unauthorized"}}
	b2 := &mockBackend{
		name:      "good",
		available: true,
		results:   []SearchResult{{Title: "OK", URL: "https://ok.com", Snippet: "ok"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())

	result1, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})
	if result1.IsError {
		t.Fatalf("First call should succeed via fallback")
	}
	if b1.called.Load() != 1 {
		t.Error("Primary should be tried first time")
	}

	b1.called.Store(0)
	result2, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test2"})
	if result2.IsError {
		t.Fatalf("Second call should succeed")
	}
	if b1.called.Load() != 0 {
		t.Error("Degraded backend should be skipped on subsequent calls")
	}
}

func TestWebSearchTool_Execute_RetryableError(t *testing.T) {
	b := &mockBackend{
		name:      "flaky",
		available: true,
		err:       &searchError{Code: 429, Message: "rate limited"},
	}
	b2 := &mockBackend{
		name:      "stable",
		available: true,
		results:   []SearchResult{{Title: "OK", URL: "https://ok.com"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b, b2}, time.Now())
	result, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})

	if result.IsError {
		t.Error("429 should trigger fallback, not error")
	}
}

func TestWebSearchTool_Execute_MaxResultsDefault(t *testing.T) {
	b := &mockBackend{
		name:      "test",
		available: true,
		results:   []SearchResult{{Title: "R1", URL: "https://a.com"}, {Title: "R2", URL: "https://b.com"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	params := &WebSearchParams{Query: "test"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content[0].Text)
	}
}

func TestWebSearchParams_Defaults(t *testing.T) {
	p := &WebSearchParams{Query: "test"}
	if p.MaxResults != 0 {
		t.Errorf("MaxResults default should be 0 (tool applies default), got %d", p.MaxResults)
	}
	if p.IncludeContent {
		t.Error("IncludeContent should default to false")
	}
}

func TestSearchError_Error(t *testing.T) {
	e := &searchError{Code: 401, Message: "unauthorized"}
	if e.Error() != "search error 401: unauthorized" {
		t.Errorf("Error() = %q, want %q", e.Error(), "search error 401: unauthorized")
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{401, false},
		{403, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{200, false},
		{404, false},
	}
	for _, tt := range tests {
		got := isRetryableHTTPCode(tt.code)
		if got != tt.want {
			t.Errorf("isRetryableHTTPCode(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestWebSearchTool_Execute_ResultFormat(t *testing.T) {
	b := &mockBackend{
		name:      "test",
		available: true,
		results: []SearchResult{
			{Title: "Go 1.24", URL: "https://go.dev/doc/go1.24", Snippet: "Release notes", Content: "Full content here", Score: 0.95},
		},
	}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	params := &WebSearchParams{Query: "Go 1.24 release", MaxResults: 5, IncludeContent: true}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if !contains(text, "Go 1.24") {
		t.Error("Result should contain title")
	}
	if !contains(text, "https://go.dev/doc/go1.24") {
		t.Error("Result should contain URL")
	}
	if !contains(text, "Full content here") {
		t.Error("Result should contain content when IncludeContent=true")
	}
}

func TestWebSearchTool_Execute_ResultFormat_NoContent(t *testing.T) {
	b := &mockBackend{
		name:      "test",
		available: true,
		results: []SearchResult{
			{Title: "Go 1.24", URL: "https://go.dev/doc/go1.24", Snippet: "Release notes", Content: "Should not appear", Score: 0.95},
		},
	}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	params := &WebSearchParams{Query: "Go 1.24 release", MaxResults: 5, IncludeContent: false}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	text := result.Content[0].Text
	if contains(text, "Should not appear") {
		t.Error("Result should NOT contain content when IncludeContent=false")
	}
}

func TestWebSearchTool_Fallback_PrimaryFails_SecondarySucceeds(t *testing.T) {
	b1 := &mockBackend{name: "searxng", available: true, err: fmt.Errorf("connection refused")}
	b2 := &mockBackend{
		name:      "tavily",
		available: true,
		results:   []SearchResult{{Title: "From Tavily", URL: "https://tavily.com"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())
	result, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})
	if result.IsError {
		t.Error("Should succeed via fallback")
	}
	if !contains(result.Content[0].Text, "From Tavily") {
		t.Error("Should return fallback backend results")
	}
}

func TestWebSearchTool_Fallback_PrimaryFails_SecondaryFails_TertiarySucceeds(t *testing.T) {
	b1 := &mockBackend{name: "searxng", available: true, err: fmt.Errorf("connection refused")}
	b2 := &mockBackend{name: "tavily", available: true, err: &searchError{Code: 401, Message: "bad key"}}
	b3 := &mockBackend{
		name:      "brave",
		available: true,
		results:   []SearchResult{{Title: "From Brave", URL: "https://brave.com"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b1, b2, b3}, time.Now())
	result, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})
	if result.IsError {
		t.Error("Should succeed via third backend")
	}
	if !contains(result.Content[0].Text, "From Brave") {
		t.Error("Should return tertiary backend results")
	}
}

func TestWebSearchTool_Fallback_DegradedSkippedOnRetry(t *testing.T) {
	b1 := &mockBackend{name: "tavily", available: true, err: &searchError{Code: 401, Message: "bad key"}}
	b2 := &mockBackend{
		name:      "brave",
		available: true,
		results:   []SearchResult{{Title: "OK", URL: "https://ok.com"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())

	result1, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test1"})
	if result1.IsError {
		t.Error("First call should succeed via fallback")
	}
	if b1.called.Load() != 1 {
		t.Error("Primary should be tried first time")
	}

	b1.called.Store(0)
	result2, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test2"})
	if result2.IsError {
		t.Error("Second call should succeed")
	}
	if b1.called.Load() != 0 {
		t.Error("Degraded backend should be skipped on retry")
	}
	if b2.called.Load() != 2 {
		t.Error("Brave should handle both calls")
	}
}

func TestWebSearchTool_Registration_NoBackends(t *testing.T) {
	tool := NewWebSearchTool(nil, time.Now())
	if tool != nil {
		t.Error("NewWebSearchTool(nil) should return nil")
	}

	tool = NewWebSearchTool([]SearchBackend{}, time.Now())
	if tool != nil {
		t.Error("NewWebSearchTool(empty) should return nil")
	}
}

func TestWebSearchTool_Registration_WithBackends(t *testing.T) {
	b := &mockBackend{name: "test", available: true}
	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	if tool == nil {
		t.Error("NewWebSearchTool with backends should not return nil")
	}
}

func TestWebSearchTool_Execute_RateLimitedAllBackends(t *testing.T) {
	b1 := &mockBackend{name: "tavily", available: true, err: &searchError{Code: 429, Message: "rate limited"}}
	b2 := &mockBackend{name: "brave", available: true, err: &searchError{Code: 429, Message: "rate limited"}}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())
	result, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})

	if !result.IsError {
		t.Error("All rate-limited should produce error")
	}
	if !contains(result.Content[0].Text, "rate") {
		t.Errorf("Error should mention rate limiting, got: %s", result.Content[0].Text)
	}
}

func TestWebSearchTool_BackendAvailabilityCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"query": "test",
			"results": []map[string]any{
				{"title": "Test", "url": "https://example.com", "content": "test content"},
			},
		})
	}))
	defer ts.Close()

	backend := NewSearXNGBackend(ts.URL, 2*time.Second)
	if !backend.Available() {
		t.Error("Backend should be available when server responds 200")
	}
}

func TestWebSearchTool_BackendUnavailable(t *testing.T) {
	backend := NewSearXNGBackend("http://localhost:1", 100*time.Millisecond)
	if backend.Available() {
		t.Error("Backend should NOT be available when server is unreachable")
	}
}

func TestWebSearchTool_Execute_PartialResults(t *testing.T) {
	partialResults := []SearchResult{
		{Title: "Result 1", URL: "https://example.com/1", Snippet: "first"},
		{Title: "Result 2", URL: "https://example.com/2", Snippet: "second"},
	}
	b := &mockBackend{
		name:      "flaky",
		available: true,
		err:       &PartialResultError{Results: partialResults, Err: fmt.Errorf("connection reset mid-stream")},
	}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	result, err := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Partial results should not be an error, got: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !contains(text, "Result 1") {
		t.Error("Should contain first partial result")
	}
	if !contains(text, "Result 2") {
		t.Error("Should contain second partial result")
	}
	if !contains(text, "Warning") {
		t.Error("Should contain warning about incomplete search")
	}
	if !contains(text, "incomplete") {
		t.Error("Warning should mention incomplete search")
	}
}

func TestWebSearchTool_Execute_PartialResultsEmpty(t *testing.T) {
	b := &mockBackend{
		name:      "flaky",
		available: true,
		err:       &PartialResultError{Results: []SearchResult{}, Err: fmt.Errorf("connection reset")},
	}

	tool := NewWebSearchTool([]SearchBackend{b}, time.Now())
	result, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})
	if !result.IsError {
		t.Error("Empty partial results should fall through to error handling (no results to return)")
	}
}

func TestWebSearchTool_Execute_PartialResultsWithFallback(t *testing.T) {
	partialResults := []SearchResult{
		{Title: "Partial", URL: "https://example.com", Snippet: "partial"},
	}
	b1 := &mockBackend{
		name:      "flaky",
		available: true,
		err:       &PartialResultError{Results: partialResults, Err: fmt.Errorf("mid-stream error")},
	}
	b2 := &mockBackend{
		name:      "stable",
		available: true,
		results:   []SearchResult{{Title: "Stable", URL: "https://stable.com", Snippet: "full result"}},
	}

	tool := NewWebSearchTool([]SearchBackend{b1, b2}, time.Now())
	result, _ := tool.Execute(context.Background(), &WebSearchParams{Query: "test"})
	if result.IsError {
		t.Fatalf("Should succeed with partial results, not fall through to fallback")
	}
	text := result.Content[0].Text
	if !contains(text, "Partial") {
		t.Error("Should return partial results, not fall through to secondary backend")
	}
	if contains(text, "Stable") {
		t.Error("Should NOT return fallback results when partial results are available")
	}
}

func TestPartialResultError_Error(t *testing.T) {
	e := &PartialResultError{
		Results: []SearchResult{{Title: "A"}, {Title: "B"}},
		Err:     fmt.Errorf("connection reset"),
	}
	if !contains(e.Error(), "partial results (2)") {
		t.Errorf("Error() should mention count, got: %s", e.Error())
	}
	if !contains(e.Error(), "connection reset") {
		t.Errorf("Error() should mention underlying error, got: %s", e.Error())
	}
}

func TestPartialResultError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	e := &PartialResultError{Results: nil, Err: inner}
	if e.Unwrap() != inner {
		t.Error("Unwrap() should return the underlying error")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
