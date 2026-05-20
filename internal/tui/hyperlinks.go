package tui

import (
	"regexp"
	"strings"
)

// OSC 8 hyperlink escape sequences.
// Format: \x1b]8;;URL\x1b\\clickable_text\x1b]8;;\x1b\\
const (
	osc8Prefix = "\x1b]8;;"
	osc8Term   = "\x1b\\"
)

// urlRegex matches http/https URLs and www. prefixed URLs.
// Excludes: whitespace, angle brackets, quotes, backticks, ESC char, and closing parens
// (closing parens are excluded to avoid capturing the delimiter in markdown links like [text](url)).
var urlRegex = regexp.MustCompile(`https?://[^\s<>\[\]"'` + "`" + `\x1b)]{2,}|www\.[^\s<>\[\]"'` + "`" + `\x1b)]{2,}`)

// WrapURLs wraps URLs in text with OSC 8 terminal hyperlink escape sequences.
// It skips URLs inside code blocks by analyzing the original markdown to find
// fenced code block regions, then mapping those to the rendered output.
// ANSI escape sequences in the input are preserved — OSC 8 is added around
// the visible URL text without breaking existing styling.
func WrapURLs(text string) string {
	return WrapURLsWithMarkdown(text, "")
}

// WrapURLsWithMarkdown wraps URLs in rendered text, using the original markdown
// to identify code block regions that should be skipped.
func WrapURLsWithMarkdown(rendered string, originalMarkdown string) string {
	if rendered == "" {
		return ""
	}

	// Extract code block content from the original markdown
	codeContent := extractCodeBlockContent(originalMarkdown)
	if len(codeContent) == 0 {
		// No code blocks, wrap all URLs
		return wrapURLsInLine(rendered)
	}

	// Find which lines in the rendered output contain code block content
	renderedCodeLines := findRenderedCodeLines(rendered, codeContent)

	lines := strings.Split(rendered, "\n")
	var result []string

	for i, line := range lines {
		if renderedCodeLines[i] {
			result = append(result, line)
			continue
		}
		// Wrap URLs in this line, preserving ANSI sequences
		wrapped := wrapURLsInLine(line)
		result = append(result, wrapped)
	}

	return strings.Join(result, "\n")
}

// extractCodeBlockContent extracts the content lines from fenced code blocks.
func extractCodeBlockContent(markdown string) []string {
	if markdown == "" {
		return nil
	}

	var content []string
	inCodeBlock := false
	fencePattern := regexp.MustCompile("^```")

	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fencePattern.MatchString(trimmed) {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			content = append(content, line)
		}
	}

	return content
}

// findRenderedCodeLines finds which lines in the rendered output contain
// code block content from the original markdown.
func findRenderedCodeLines(rendered string, codeContent []string) map[int]bool {
	codeLines := make(map[int]bool)
	if len(codeContent) == 0 {
		return codeLines
	}

	renderedLines := strings.Split(rendered, "\n")
	for i, line := range renderedLines {
		clean := stripANSIForCodeDetect(line)
		for _, content := range codeContent {
			if strings.Contains(clean, content) && content != "" {
				codeLines[i] = true
				break
			}
		}
	}

	return codeLines
}

// stripANSIForCodeDetect removes ANSI escape sequences for code block detection.
func stripANSIForCodeDetect(s string) string {
	// Remove CSI sequences (ESC [ ... letter)
	result := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`).ReplaceAllString(s, "")
	// Remove OSC sequences (ESC ] ... BEL or ESC \)
	result = regexp.MustCompile(`\x1b\][^\x07\x1b]*(\x07|\x1b\\)`).ReplaceAllString(result, "")
	return result
}

// wrapURLsInLine wraps all URLs in a single line with OSC 8 hyperlinks,
// preserving any ANSI escape sequences that may be present.
// Skips duplicate URLs inside parentheses (glamour renders bare URLs as "url (url)").
func wrapURLsInLine(line string) string {
	// Find all URL matches with their positions
	matches := urlRegex.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	// Collect URLs that are duplicates inside parentheses
	skipPositions := make(map[int]bool)
	for i := 0; i < len(matches); i++ {
		start := matches[i][0]
		end := matches[i][1]
		url := line[start:end]

		if !isURLInParens(line, start) {
			continue
		}

		// Check if the same URL appears immediately before the paren
		for j := 0; j < i; j++ {
			prevStart := matches[j][0]
			prevEnd := matches[j][1]
			prevURL := line[prevStart:prevEnd]
			if prevURL == url {
				// Check if paren comes right after the previous URL (with optional ANSI)
				if isParensAfterURL(line, prevEnd, start) {
					skipPositions[i] = true
					break
				}
			}
		}
	}

	// Process matches in reverse order to preserve positions
	for i := len(matches) - 1; i >= 0; i-- {
		if skipPositions[i] {
			continue
		}
		start := matches[i][0]
		end := matches[i][1]
		url := line[start:end]

		// Normalize www. URLs to include https://
		oscURL := url
		if strings.HasPrefix(url, "www.") {
			oscURL = "https://" + url
		}

		// Insert OSC 8 hyperlink: prefix before URL, terminator after
		hyperlink := osc8Prefix + oscURL + osc8Term + url + osc8Prefix + osc8Term
		line = line[:start] + hyperlink + line[end:]
	}

	return line
}

// isURLInParens checks if a URL at position start is inside parentheses,
// accounting for ANSI escape sequences between the paren and the URL.
func isURLInParens(line string, start int) bool {
	i := start - 1
	for i >= 0 {
		c := line[i]
		if c == '(' {
			return true
		}
		if c == ')' || c == ' ' || c == '\t' || c == '\n' {
			return false
		}
		// Skip ANSI CSI escape sequences (ESC [ ... letter)
		if c == '\x1b' && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) {
				b := line[j]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					// Skip entire sequence: set i to position before ESC
					i = i - 1
					break
				}
				j++
			}
			if j >= len(line) {
				return false // unterminated sequence
			}
			continue
		}
		i--
	}
	return false
}

// isParensAfterURL checks if there's only a paren (with optional ANSI) between endPos and urlStart.
func isParensAfterURL(line string, endPos int, urlStart int) bool {
	i := endPos
	for i < urlStart {
		c := line[i]
		if c == '(' {
			return true
		}
		if c == '\x1b' && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) {
				b := line[j]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					i = j
					break
				}
				j++
			}
			i++
			continue
		}
		if c != ' ' && c != '\t' {
			return false
		}
		i++
	}
	return false
}

// ExtractURLs extracts all URLs from text (including OSC 8 wrapped URLs).
// Returns the list of URLs found.
func ExtractURLs(text string) []string {
	// Extract URLs from OSC 8 sequences
	// OSC 8 format: \x1b]8;;URL\x1b\\
	osc8Regex := regexp.MustCompile(`\x1b]8;;([^\x1b]+)\x1b\\`)
	var urls []string
	seen := make(map[string]bool)
	for _, match := range osc8Regex.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && match[1] != "" && !seen[match[1]] {
			seen[match[1]] = true
			urls = append(urls, match[1])
		}
	}

	// Also find bare URLs not yet wrapped
	for _, match := range urlRegex.FindAllString(text, -1) {
		if !seen[match] {
			seen[match] = true
			urls = append(urls, match)
		}
	}

	return urls
}
