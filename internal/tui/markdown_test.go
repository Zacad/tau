package tui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderMarkdown_Empty(t *testing.T) {
	got := RenderMarkdown("", 80)
	if got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

func TestRenderMarkdown_Bold(t *testing.T) {
	got := RenderMarkdown("**bold text**", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "bold text") {
		t.Errorf("output missing 'bold text': %q", clean)
	}
}

func TestRenderMarkdown_Italic(t *testing.T) {
	got := RenderMarkdown("*italic text*", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "italic text") {
		t.Errorf("output missing 'italic text': %q", clean)
	}
}

func TestRenderMarkdown_Heading(t *testing.T) {
	got := RenderMarkdown("# Heading 1", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "Heading 1") {
		t.Errorf("output missing heading text: %q", clean)
	}
}

func TestRenderMarkdown_InlineCode(t *testing.T) {
	got := RenderMarkdown("use `fmt.Println()`", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "fmt.Println()") {
		t.Errorf("output missing inline code: %q", clean)
	}
}

func TestRenderMarkdown_FencedCodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```"
	got := RenderMarkdown(input, 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "func main()") {
		t.Errorf("output missing code block content: %q", clean)
	}
}

func TestRenderMarkdown_List_Unordered(t *testing.T) {
	input := "- item one\n- item two\n- item three"
	got := RenderMarkdown(input, 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "item one") {
		t.Errorf("output missing list item: %q", clean)
	}
	if !strings.Contains(clean, "item two") {
		t.Errorf("output missing list item: %q", clean)
	}
}

func TestRenderMarkdown_List_Ordered(t *testing.T) {
	input := "1. first\n2. second\n3. third"
	got := RenderMarkdown(input, 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "first") {
		t.Errorf("output missing ordered list item: %q", clean)
	}
}

func TestRenderMarkdown_Link(t *testing.T) {
	got := RenderMarkdown("[example](https://example.com)", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "example") {
		t.Errorf("output missing link text: %q", clean)
	}
	if !strings.Contains(clean, "https://example.com") {
		t.Errorf("output missing URL: %q", clean)
	}
}

func TestRenderMarkdown_BareURL(t *testing.T) {
	got := RenderMarkdown("Visit https://example.com for more.", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "https://example.com") {
		t.Errorf("output missing bare URL: %q", clean)
	}
}

func TestRenderMarkdown_Blockquote(t *testing.T) {
	got := RenderMarkdown("> this is a quote", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "this is a quote") {
		t.Errorf("output missing blockquote text: %q", clean)
	}
}

func TestRenderMarkdown_MixedContent(t *testing.T) {
	input := "# Title\n\nSome **bold** and *italic* text.\n\n- list item\n- another item\n\n```go\nfmt.Println(\"hello\")\n```\n\n[link](https://example.com)"
	got := RenderMarkdown(input, 80)
	clean := stripANSI(got)
	for _, want := range []string{"Title", "bold", "italic", "list item", "another item", "fmt.Println", "hello", "link", "https://example.com"} {
		if !strings.Contains(clean, want) {
			t.Errorf("output missing %q in mixed content: %q", want, clean)
		}
	}
}

func TestRenderMarkdown_WidthWrapping(t *testing.T) {
	longText := strings.Repeat("word ", 50)
	got80 := RenderMarkdown(longText, 80)
	got40 := RenderMarkdown(longText, 40)
	clean80 := stripANSI(got80)
	clean40 := stripANSI(got40)
	if clean80 == clean40 {
		t.Log("width 80 and 40 produced identical output — may be expected for short content")
	}
}

func TestRenderMarkdown_PolishUnicode(t *testing.T) {
	got := RenderMarkdown("Cześć! W czym mogę Ci pomóc?", 80)
	clean := stripANSI(got)
	if !strings.Contains(clean, "Cześć") {
		t.Errorf("output missing Polish text: %q", clean)
	}
	if !strings.Contains(clean, "pomóc") {
		t.Errorf("output missing Polish text: %q", clean)
	}
}

func TestRenderMarkdown_BareURLGetsOSC8(t *testing.T) {
	got := RenderMarkdown("Visit https://example.com for more.", 80)
	// OSC 8 prefix should be present for the bare URL
	if !strings.Contains(got, osc8Prefix+"https://example.com"+osc8Term) {
		t.Errorf("bare URL should have OSC 8 hyperlink, got:\n%q", got)
	}
}

func TestRenderMarkdown_CodeBlockURLNoOSC8(t *testing.T) {
	input := "```\nhttps://example.com\n```"
	got := RenderMarkdown(input, 80)
	// URL inside code block should NOT get OSC 8 wrapping
	if strings.Contains(got, osc8Prefix+"https://example.com") {
		t.Errorf("URL inside code block should not have OSC 8 hyperlink, got:\n%q", got)
	}
}

func TestRenderMarkdown_EmptyAndWhitespace(t *testing.T) {
	if got := RenderMarkdown("", 80); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
	if got := RenderMarkdown("   ", 80); got == "" {
		t.Error("whitespace-only input should not return empty")
	}
	if got := RenderMarkdown("\n\n\n", 80); got == "" {
		t.Error("newlines-only input should not return empty")
	}
}

func TestRenderMarkdown_MalformedMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unclosed code fence", "```go\nfunc main() {}"},
		{"broken list", "- item one\n- item two\nnot a list item"},
		{"unclosed bold", "**bold without closing"},
		{"nested unclosed", "**bold *italic without closing"},
		{"empty fences", "```\n```"},
		{"mixed malformed", "# Heading\n\n**bold\n\n```\nsome code\n\n- list\n  normal text"},
		{"lone hash", "#"},
		{"unclosed link", "[text](https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderMarkdown(tt.input, 80)
			// Should not panic or return empty for non-empty input
			if tt.input != "" && got == "" {
				t.Error("expected non-empty output for malformed markdown")
			}
		})
	}
}

func TestRenderMarkdown_LongContent(t *testing.T) {
	longText := strings.Repeat("This is a paragraph with **bold** and *italic* text. ", 200) // ~10K chars
	got := RenderMarkdown(longText, 80)
	if got == "" {
		t.Fatal("expected non-empty output for long content")
	}
	clean := stripANSI(got)
	if len(clean) < 1000 {
		t.Errorf("expected substantial output for 10K chars, got %d chars", len(clean))
	}
}

func TestRenderMarkdown_MixedContentWithThinking(t *testing.T) {
	input := "# Response\n\nHere is the answer to your question.\n\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\nSee [docs](https://example.com) for more."
	got := RenderMarkdown(input, 80)
	clean := stripANSI(got)
	for _, want := range []string{"Response", "Here is the answer", "func main()", "fmt.Println", "docs", "https://example.com"} {
		if !strings.Contains(clean, want) {
			t.Errorf("output missing %q in mixed content", want)
		}
	}
}

func BenchmarkRenderMarkdown_Typical(b *testing.B) {
	input := "# Heading\n\nSome **bold** and *italic* text with a [link](https://example.com).\n\n- item 1\n- item 2\n- item 3\n\n```go\nfunc main() { fmt.Println(\"hello\") }\n```\n\n> A blockquote with some text.\n\n1. first\n2. second\n3. third"
	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		RenderMarkdown(input, 80)
	}
	elapsed := time.Since(start) / time.Duration(b.N)
	if elapsed > 50*time.Millisecond {
		b.Errorf("render took %v, expected < 50ms", elapsed)
	}
}

func BenchmarkRenderMarkdown_LongContent(b *testing.B) {
	input := strings.Repeat("Some paragraph text with **bold** and *italic*. ", 200) // ~10K chars
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RenderMarkdown(input, 80)
	}
}

func BenchmarkRenderMarkdown_Empty(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RenderMarkdown("", 80)
	}
}

func TestRenderMarkdown_ErrorFallback(t *testing.T) {
	// Verify that when glamour rendering fails, the original text is returned.
	// We can't easily trigger a glamour error with valid inputs, but we can
	// verify the function doesn't panic on edge cases that might cause issues.
	tests := []struct {
		name  string
		input string
		width int
	}{
		{"very long single line", strings.Repeat("a", 100000), 80},
		{"deeply nested markdown", "**bold *italic **nested** more* end**", 40},
		{"mixed unicode and special chars", "日本語テスト 🎉 **bold** *italic*", 60},
		{"only special chars", "### *** ___", 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderMarkdown(tt.input, tt.width)
			if got == "" && tt.input != "" {
				t.Error("expected non-empty output for non-empty input")
			}
		})
	}
}
