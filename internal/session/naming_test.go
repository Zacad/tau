package session

import (
	"strings"
	"testing"
	"time"
)

func TestAutoName_SimpleMessage(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("Hello world", ts)
	if name != "hello-world" {
		t.Errorf("expected 'hello-world', got '%s'", name)
	}
}

func TestAutoName_SpecialCharacters(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("Let's build a REST API in Go!!", ts)
	if name != "lets-build-a-rest-api-in-go" {
		t.Errorf("expected 'lets-build-a-rest-api-in-go', got '%s'", name)
	}
}

func TestAutoName_LongMessage_Truncated(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	msg := strings.Repeat("a", 60)
	name := AutoName(msg, ts)
	if len(name) > 50 {
		t.Errorf("name too long: %d chars, expected <= 50", len(name))
	}
	// Should be all 'a's (no hyphens since no separators)
	if name != strings.Repeat("a", 50) {
		t.Errorf("expected 50 'a's, got '%s'", name)
	}
}

func TestAutoName_LongMessage_TruncatedAtHyphen(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	// Create a message that would be truncated mid-word with trailing hyphen
	msg := "this-is-a-very-long-message-that-needs-truncation-now"
	name := AutoName(msg, ts)
	if len(name) > 50 {
		t.Errorf("name too long: %d chars, expected <= 50", len(name))
	}
	// Should not end with hyphen
	if strings.HasSuffix(name, "-") {
		t.Errorf("name should not end with hyphen: '%s'", name)
	}
}

func TestAutoName_EmptyMessage_Fallback(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("", ts)
	expected := "2026-05-03-120000"
	if name != expected {
		t.Errorf("expected '%s', got '%s'", expected, name)
	}
}

func TestAutoName_OnlySpecialChars_Fallback(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("!@#$%^&*()", ts)
	expected := "2026-05-03-120000"
	if name != expected {
		t.Errorf("expected fallback timestamp '%s', got '%s'", expected, name)
	}
}

func TestAutoName_NumbersOnly(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("12345", ts)
	if name != "12345" {
		t.Errorf("expected '12345', got '%s'", name)
	}
}

func TestAutoName_MultipleSpaces_Collapsed(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("hello    world    test", ts)
	if name != "hello-world-test" {
		t.Errorf("expected 'hello-world-test', got '%s'", name)
	}
}

func TestAutoName_LeadingTrailingSpaces_Trimmed(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("  hello world  ", ts)
	if name != "hello-world" {
		t.Errorf("expected 'hello-world', got '%s'", name)
	}
}

func TestAutoName_Unicode(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("héllo wörld café", ts)
	if name != "héllo-wörld-café" {
		t.Errorf("expected 'héllo-wörld-café', got '%s'", name)
	}
}

func TestAutoName_MixedCase(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	name := AutoName("Hello WORLD Test", ts)
	if name != "hello-world-test" {
		t.Errorf("expected 'hello-world-test', got '%s'", name)
	}
}

func TestGenerateFilename(t *testing.T) {
	ts := time.Date(2026, 5, 3, 12, 30, 45, 0, time.UTC)
	name := GenerateFilename(ts, "a3f7b2c1")
	expected := "20260503T123045_a3f7b2c1.jsonl"
	if name != expected {
		t.Errorf("expected '%s', got '%s'", expected, name)
	}
}

func TestGenerateFilename_UTC(t *testing.T) {
	// Ensure timestamp is always UTC regardless of input timezone
	loc, _ := time.LoadLocation("America/New_York")
	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, loc) // noon in NY = 16:00 UTC
	name := GenerateFilename(ts, "abcd1234")
	if !strings.HasPrefix(name, "20260503T160000_") {
		t.Errorf("expected UTC timestamp in filename, got '%s'", name)
	}
}

func TestEncodeCWD(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/adam/Projects/tau", "-home-adam-Projects-tau"},
		{"/", "-"},
		{"", ""},
		{"/a/b/c", "-a-b-c"},
	}

	for _, tt := range tests {
		result := EncodeCWD(tt.input)
		if result != tt.expected {
			t.Errorf("EncodeCWD(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}
