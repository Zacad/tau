package tui

import (
	"strings"
	"testing"
)

func TestWrapURLs_Empty(t *testing.T) {
	got := WrapURLs("")
	if got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

func TestWrapURLs_NoURLs(t *testing.T) {
	input := "This is plain text with no links."
	got := WrapURLs(input)
	if got != input {
		t.Errorf("expected unchanged text, got %q", got)
	}
}

func TestWrapURLs_SingleHTTPS(t *testing.T) {
	input := "Visit https://example.com for more."
	got := WrapURLs(input)

	if !strings.Contains(got, osc8Prefix+"https://example.com"+osc8Term) {
		t.Errorf("missing OSC 8 wrapper for URL, got:\n%q", got)
	}
	if !strings.Contains(got, "https://example.com"+osc8Prefix+osc8Term) {
		t.Errorf("missing URL text before closing OSC 8, got:\n%q", got)
	}
}

func TestWrapURLs_SingleHTTP(t *testing.T) {
	input := "Go to http://example.com/path"
	got := WrapURLs(input)

	if !strings.Contains(got, osc8Prefix+"http://example.com/path"+osc8Term) {
		t.Errorf("missing OSC 8 wrapper for HTTP URL, got:\n%q", got)
	}
}

func TestWrapURLs_WWWPrefix(t *testing.T) {
	input := "Check www.example.com/page"
	got := WrapURLs(input)

	// www. URLs should be normalized to https:// in the OSC 8 target
	if !strings.Contains(got, osc8Prefix+"https://www.example.com/page"+osc8Term) {
		t.Errorf("missing normalized OSC 8 wrapper for www URL, got:\n%q", got)
	}
	// But visible text should remain as-is
	if !strings.Contains(got, "www.example.com/page") {
		t.Errorf("visible text should contain original www URL, got:\n%q", got)
	}
}

func TestWrapURLs_MultipleURLs(t *testing.T) {
	input := "See https://a.com and https://b.org for details."
	got := WrapURLs(input)

	count := strings.Count(got, osc8Prefix)
	// Each URL gets 2 OSC 8 sequences (open + close)
	if count != 4 {
		t.Errorf("expected 4 OSC 8 sequences for 2 URLs, got %d in:\n%q", count, got)
	}
}

func TestWrapURLs_URLInCodeBlock(t *testing.T) {
	// Use WrapURLsWithMarkdown with original markdown to detect code blocks
	markdown := "text\n```\nhttps://example.com\n```\nmore"
	// Simulated rendered output (glamour-style with borders)
	rendered := "text\n\n  \n│ https://example.com  │\n\nmore"
	got := WrapURLsWithMarkdown(rendered, markdown)

	// URL inside code block should NOT be wrapped
	if strings.Contains(got, osc8Prefix+"https://example.com") {
		t.Errorf("URL inside code block should not be wrapped, got:\n%q", got)
	}
}

func TestWrapURLs_MarkdownLink(t *testing.T) {
	// Glamour renders [text](url) as: text (url)
	input := "Click example (https://example.com)"
	got := WrapURLs(input)

	if !strings.Contains(got, osc8Prefix+"https://example.com"+osc8Term) {
		t.Errorf("missing OSC 8 wrapper for markdown link URL, got:\n%q", got)
	}
}

func TestWrapURLs_PreservesANSI(t *testing.T) {
	// ANSI bold sequence around URL
	input := "\x1b[1mhttps://example.com\x1b[0m"
	got := WrapURLs(input)

	// Should contain OSC 8 wrapper
	if !strings.Contains(got, osc8Prefix+"https://example.com"+osc8Term) {
		t.Errorf("missing OSC 8 wrapper in ANSI text, got:\n%q", got)
	}
	// Should preserve original ANSI codes
	if !strings.Contains(got, "\x1b[1m") || !strings.Contains(got, "\x1b[0m") {
		t.Errorf("ANSI codes not preserved, got:\n%q", got)
	}
}

func TestWrapURLs_URLWithQueryParams(t *testing.T) {
	input := "https://example.com/search?q=go+lang&lang=en"
	got := WrapURLs(input)

	if !strings.Contains(got, osc8Prefix+"https://example.com/search?q=go+lang&lang=en"+osc8Term) {
		t.Errorf("missing OSC 8 wrapper for URL with query params, got:\n%q", got)
	}
}

func TestWrapURLs_URLWithParentheses(t *testing.T) {
	// URLs with parentheses should be handled (common in Wikipedia URLs)
	input := "See https://en.wikipedia.org/wiki/Go_(programming_language)"
	got := WrapURLs(input)

	// The regex stops at ) so it should capture up to the closing paren
	if !strings.Contains(got, "https://en.wikipedia.org/wiki/Go_") {
		t.Errorf("URL with parens not handled correctly, got:\n%q", got)
	}
}

func TestWrapURLs_MalformedURL(t *testing.T) {
	// "https://" alone should not match (need at least 2 chars after)
	input := "Go to https:// "
	got := WrapURLs(input)

	if strings.Contains(got, osc8Prefix) {
		t.Errorf("malformed URL should not be wrapped, got:\n%q", got)
	}
}

func TestWrapURLs_MixedContent(t *testing.T) {
	// Without original markdown, code block detection is not possible
	// This tests basic URL wrapping in mixed content
	input := "# Heading\n\nSome text with https://example.com link.\n\nMore text with www.google.com"
	got := WrapURLs(input)

	// Should wrap the bare URL
	if !strings.Contains(got, osc8Prefix+"https://example.com"+osc8Term) {
		t.Errorf("missing wrapper for bare URL, got:\n%q", got)
	}
	// Should wrap the www URL
	if !strings.Contains(got, osc8Prefix+"https://www.google.com"+osc8Term) {
		t.Errorf("missing wrapper for www URL, got:\n%q", got)
	}
}

func TestWrapURLsWithMarkdown_CodeBlockSkipped(t *testing.T) {
	// Use WrapURLsWithMarkdown with original markdown to detect code blocks
	markdown := "# Heading\n\nSome text with https://example.com link.\n\n```\ncode block\nhttps://no.com\n```\n\nMore text with https://yes.com"
	rendered := RenderMarkdown(markdown, 80)

	// Should wrap the bare URL before code block
	if !strings.Contains(rendered, osc8Prefix+"https://example.com"+osc8Term) {
		t.Errorf("missing wrapper for bare URL before code block")
	}
	// Should wrap the URL after code block
	if !strings.Contains(rendered, osc8Prefix+"https://yes.com"+osc8Term) {
		t.Errorf("missing wrapper for URL after code block")
	}
	// Code block URL should NOT be wrapped
	if strings.Contains(rendered, osc8Prefix+"https://no.com") {
		t.Errorf("code block URL should not be wrapped")
	}
}

func TestWrapURLs_URLAtStartAndEnd(t *testing.T) {
	input := "https://start.com text https://end.com"
	got := WrapURLs(input)

	if !strings.HasPrefix(got, osc8Prefix+"https://start.com"+osc8Term) {
		t.Errorf("URL at start not wrapped correctly, got:\n%q", got)
	}
	if !strings.HasSuffix(got, "https://end.com"+osc8Prefix+osc8Term) {
		t.Errorf("URL at end not wrapped correctly, got:\n%q", got)
	}
}

func TestWrapURLs_SameURLTwice(t *testing.T) {
	input := "https://example.com and again https://example.com"
	got := WrapURLs(input)

	count := strings.Count(got, osc8Prefix+"https://example.com"+osc8Term)
	if count != 2 {
		t.Errorf("expected 2 OSC 8 wrappers for same URL twice, got %d in:\n%q", count, got)
	}
}

func TestWrapURLs_GlamourLinkFormat(t *testing.T) {
	// Glamour renders links as "text (url)" — skip the parenthesized URL
	input := "See https://example.com (https://example.com) for more"
	got := WrapURLs(input)

	// Only wrap the first URL (visible text), not the parenthesized one
	count := strings.Count(got, osc8Prefix+"https://example.com"+osc8Term)
	if count != 1 {
		t.Errorf("expected 1 OSC 8 wrapper for glamour link format, got %d in:\n%q", count, got)
	}
}

func TestWrapURLs_GlamourLinkFormatWithANSI(t *testing.T) {
	// Glamour may add ANSI codes inside the parens
	input := "See https://example.com (\x1b[36mhttps://example.com\x1b[0m) for more"
	got := WrapURLs(input)

	count := strings.Count(got, osc8Prefix+"https://example.com"+osc8Term)
	if count != 1 {
		t.Errorf("expected 1 OSC 8 wrapper for glamour link with ANSI, got %d in:\n%q", count, got)
	}
}

func TestExtractURLs_FromOSC8(t *testing.T) {
	input := "Visit " + osc8Prefix + "https://example.com" + osc8Term + "link" + osc8Prefix + osc8Term
	urls := ExtractURLs(input)

	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com" {
		t.Errorf("expected https://example.com, got %q", urls[0])
	}
}

func TestExtractURLs_MultipleOSC8(t *testing.T) {
	input := osc8Prefix + "https://a.com" + osc8Term + "a" + osc8Prefix + osc8Term + " " +
		osc8Prefix + "https://b.org" + osc8Term + "b" + osc8Prefix + osc8Term
	urls := ExtractURLs(input)

	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://a.com" {
		t.Errorf("expected first URL https://a.com, got %q", urls[0])
	}
	if urls[1] != "https://b.org" {
		t.Errorf("expected second URL https://b.org, got %q", urls[1])
	}
}

func TestExtractURLs_Empty(t *testing.T) {
	urls := ExtractURLs("")
	if len(urls) != 0 {
		t.Errorf("expected 0 URLs from empty string, got %d", len(urls))
	}
}

func TestExtractCodeBlockContent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
	}{
		{
			name:  "no code blocks",
			input: "plain text\nmore text",
			want:  nil,
		},
		{
			name:  "single code block",
			input: "text\n```\ncode line\n```\nmore",
			want:  []string{"code line"},
		},
		{
			name:  "code block with language",
			input: "```go\nfunc main() {}\n```",
			want:  []string{"func main() {}"},
		},
		{
			name:  "multiple code blocks",
			input: "```\ncode1\n```\ntext\n```\ncode2\n```",
			want:  []string{"code1", "code2"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "multi-line code block",
			input: "```\nline1\nline2\nline3\n```",
			want:  []string{"line1", "line2", "line3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCodeBlockContent(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("expected %d content lines, got %d: %v", len(tt.want), len(got), got)
				return
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("content[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}
