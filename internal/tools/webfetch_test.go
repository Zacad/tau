package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Name() != "webfetch" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "webfetch")
	}
}

func TestWebFetchTool_ExecutionMode(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.ExecutionMode() != types.ExecutionParallel {
		t.Errorf("ExecutionMode() = %v, want %v", tool.ExecutionMode(), types.ExecutionParallel)
	}
}

func TestWebFetchTool_Description(t *testing.T) {
	tool := NewWebFetchTool()
	desc := tool.Description()
	if !strings.Contains(desc, "webfetch") {
		t.Error("Description should mention tool name")
	}
	if !strings.Contains(desc, "URL") {
		t.Error("Description should mention URL")
	}
}

func TestWebFetchTool_Execute_HTMLToMarkdown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Test</title></head><body><h1>Hello World</h1><p>This is a test.</p></body></html>`)
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	params := &WebFetchParams{URL: ts.URL, Format: "markdown"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() error result: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Hello World") {
		t.Errorf("Result should contain converted markdown, got: %s", text)
	}
}

func TestWebFetchTool_Execute_TextFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "Plain text response")
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	params := &WebFetchParams{URL: ts.URL, Format: "text"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, "Plain text response") {
		t.Errorf("Result should contain text, got: %s", result.Content[0].Text)
	}
}

func TestWebFetchTool_Execute_StripStyleScript(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><style>body{color:red}</style><script>alert('xss')</script></head><body><p>Content</p></body></html>`)
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	params := &WebFetchParams{URL: ts.URL, Format: "markdown"}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	text := result.Content[0].Text
	if strings.Contains(text, "alert('xss')") {
		t.Error("Result should not contain script content")
	}
	if strings.Contains(text, "color:red") {
		t.Error("Result should not contain style content")
	}
	if !strings.Contains(text, "Content") {
		t.Error("Result should contain actual content")
	}
}

func TestWebFetchTool_Execute_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	params := &WebFetchParams{URL: ts.URL}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("404 should produce error result")
	}
	if !strings.Contains(result.Content[0].Text, "not found") {
		t.Errorf("Error should mention 'not found', got: %s", result.Content[0].Text)
	}
}

func TestWebFetchTool_Execute_403(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	params := &WebFetchParams{URL: ts.URL}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("403 should produce error result")
	}
	if !strings.Contains(result.Content[0].Text, "denied") && !strings.Contains(result.Content[0].Text, "Access") {
		t.Errorf("Error should mention access denied, got: %s", result.Content[0].Text)
	}
}

func TestWebFetchTool_Execute_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	params := &WebFetchParams{URL: ts.URL, Timeout: 1}
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("Timeout should produce error result")
	}
}

func TestWebFetchTool_Execute_SSRF_Localhost(t *testing.T) {
	tool := NewWebFetchTool()

	tests := []struct {
		url string
	}{
		{"http://localhost/path"},
		{"http://127.0.0.1/path"},
		{"http://10.0.0.1/path"},
		{"http://172.16.0.1/path"},
		{"http://192.168.1.1/path"},
		{"http://169.254.169.254/path"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), &WebFetchParams{URL: tt.url})
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if !result.IsError {
				t.Errorf("SSRF: %s should be blocked", tt.url)
			}
			if !strings.Contains(result.Content[0].Text, "private") && !strings.Contains(result.Content[0].Text, "reserved") {
				t.Errorf("Error should mention private/reserved IP, got: %s", result.Content[0].Text)
			}
		})
	}
}

func TestWebFetchTool_Execute_SSRF_PublicAllowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><p>Public content</p></body></html>")
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	result, err := tool.Execute(context.Background(), &WebFetchParams{URL: ts.URL})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Errorf("Public URL should be allowed, got error: %s", result.Content[0].Text)
	}
}

func TestWebFetchTool_Execute_InvalidURLScheme(t *testing.T) {
	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), &WebFetchParams{URL: "ftp://example.com/file"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("Non-http(s) URL should produce error")
	}
}

func TestWebFetchTool_Execute_BinaryContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4 fake pdf content"))
	}))
	defer ts.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), &WebFetchParams{URL: ts.URL})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		text := result.Content[0].Text
		if !strings.Contains(text, "Binary") && !strings.Contains(text, "application/pdf") {
			t.Errorf("Binary content should return metadata, got: %s", text)
		}
	}
}

func TestWebFetchTool_Execute_UserAgent(t *testing.T) {
	var userAgent string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	tool.Execute(context.Background(), &WebFetchParams{URL: ts.URL})

	if !strings.Contains(userAgent, "tau-agent") {
		t.Errorf("User-Agent should contain 'tau-agent', got: %s", userAgent)
	}
}

func TestWebFetchTool_Execute_LargeResponse(t *testing.T) {
	largeHTML := "<html><body>" + strings.Repeat("<p>Lorem ipsum dolor sit amet</p>", 10000) + "</body></html>"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, largeHTML)
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	result, err := tool.Execute(context.Background(), &WebFetchParams{URL: ts.URL})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Large response should be truncated, not errored: %s", result.Content[0].Text)
	}
}

func TestWebFetchTool_Execute_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer ts.Close()

	tool := NewWebFetchTool()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := tool.Execute(ctx, &WebFetchParams{URL: ts.URL})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("Cancelled context should produce error result")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"169.254.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"::1", true},
		{"0.0.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := isPrivateIP(tt.ip)
			if got != tt.want {
				t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestWebFetchParams_Defaults(t *testing.T) {
	p := &WebFetchParams{URL: "https://example.com"}
	if p.Format != "" {
		t.Errorf("Format should default empty, got %q", p.Format)
	}
	if p.Timeout != 0 {
		t.Errorf("Timeout should default 0, got %d", p.Timeout)
	}
}

func TestWebFetchTool_Execute_EmptyURL(t *testing.T) {
	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), &WebFetchParams{URL: ""})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.IsError {
		t.Error("Empty URL should produce error")
	}
}

func TestWebFetchTool_Execute_HTMLFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><p>Raw HTML</p></body></html>")
	}))
	defer ts.Close()

	tool := NewWebFetchToolForTest()
	result, _ := tool.Execute(context.Background(), &WebFetchParams{URL: ts.URL, Format: "html"})
	if result.IsError {
		t.Fatalf("HTML format should work: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "<p>Raw HTML</p>") {
		t.Errorf("HTML format should return raw HTML, got: %s", result.Content[0].Text)
	}
}

func TestWebFetchTool_Execute_TimeoutClamped(t *testing.T) {
	tool := NewWebFetchTool()
	params := &WebFetchParams{URL: "https://example.com", Timeout: 999}
	result, _ := tool.Execute(context.Background(), params)
	if result.IsError && strings.Contains(result.Content[0].Text, "timed out") {
		t.Error("Timeout should be clamped to 120s, not immediately fail")
	}
}
