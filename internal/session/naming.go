package session

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// AutoName generates a session name from the first user message.
// It strips special characters, collapses whitespace, truncates to 50 chars,
// and converts to lowercase with hyphens as separators.
// If the input is empty or produces no valid characters, returns a timestamp fallback.
func AutoName(firstUserMessage string, ts time.Time) string {
	name := normalizeName(firstUserMessage)
	if name == "" {
		return ts.Format("2006-01-02-150405")
	}
	if len(name) > 50 {
		name = name[:50]
		// Trim trailing hyphen if truncation cut mid-word
		name = strings.TrimRight(name, "-")
	}
	return name
}

// normalizeName converts a message to a slug: lowercase, hyphens for spaces, no special chars.
func normalizeName(s string) string {
	// Pass 1: keep only alphanumeric and spaces, lowercase
	var cleaned strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cleaned.WriteRune(unicode.ToLower(r))
		} else if unicode.IsSpace(r) {
			cleaned.WriteByte(' ')
		}
		// all other chars (punctuation, symbols) are dropped
	}

	// Pass 2: collapse spaces to single hyphens, trim
	result := strings.TrimSpace(cleaned.String())
	result = strings.Join(strings.Fields(result), "-")
	return result
}

// EncodeCWD replaces "/" with "-" for directory-safe filenames.
func EncodeCWD(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// GenerateFilename creates a session filename: <timestamp>_<8-char-hex>.jsonl
// The timestamp is in UTC with format YYYYMMDDTHHMMSS.
func GenerateFilename(ts time.Time, id string) string {
	return fmt.Sprintf("%s_%s.jsonl", ts.UTC().Format("20060102T150405"), id)
}
