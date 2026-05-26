package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const toolSummaryMaxRunes = 160

var sensitiveSummaryKeys = map[string]bool{
	"api_key":       true,
	"apikey":        true,
	"authorization": true,
	"auth":          true,
	"cookie":        true,
	"password":      true,
	"secret":        true,
	"token":         true,
}

var omittedLargeSummaryKeys = map[string]bool{
	"content":  true,
	"old_text": true,
	"new_text": true,
}

// SummarizeToolArgsJSON converts raw JSON tool arguments into a sanitized,
// deterministic, display-oriented summary. Malformed JSON returns a safe
// fallback that never echoes the raw argument string.
func SummarizeToolArgsJSON(toolName string, argsJSON []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(argsJSON, &raw); err != nil {
		return "args: [invalid JSON]"
	}
	return SummarizeToolArgs(toolName, raw)
}

// SummarizeToolArgs converts structured tool arguments into compact metadata
// suitable for UI display. It redacts common secret keys, omits large content
// fields, sorts fallback keys, and truncates by rune count.
func SummarizeToolArgs(toolName string, raw map[string]any) string {
	safe := sanitizeToolArgs(raw)
	var summary string
	switch toolName {
	case "websearch":
		summary = summarizeWebsearch(safe)
	case "read":
		summary = summarizePathTool(safe, "path", "offset", "limit")
	case "write", "edit", "ls":
		summary = summarizePathTool(safe, "path")
	case "bash":
		summary = stringArg(safe, "command")
	case "grep":
		summary = joinPresent(safe, []summaryField{{"pattern", 60}, {"path", 0}})
	case "find":
		summary = joinPresent(safe, []summaryField{{"pattern", 60}, {"path", 0}})
	case "webfetch":
		summary = stringArg(safe, "url")
	case "subagent":
		summary = summarizeSubagent(safe)
	}
	if summary == "" {
		summary = summarizeKeyValues(safe)
	}
	return truncateRunes(summary, toolSummaryMaxRunes)
}

type summaryField struct {
	key      string
	maxRunes int
}

func sanitizeToolArgs(raw map[string]any) map[string]any {
	safe := make(map[string]any, len(raw))
	for k, v := range raw {
		lk := strings.ToLower(k)
		if sensitiveSummaryKeys[lk] || containsSensitiveToken(lk) {
			safe[k] = "[redacted]"
			continue
		}
		if omittedLargeSummaryKeys[lk] {
			safe[k] = "[omitted]"
			continue
		}
		safe[k] = v
	}
	return safe
}

func containsSensitiveToken(key string) bool {
	for token := range sensitiveSummaryKeys {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func summarizeWebsearch(raw map[string]any) string {
	if q := stringArg(raw, "query"); q != "" {
		return "query: " + truncateRunes(q, 80)
	}
	if queries, ok := raw["queries"].([]any); ok && len(queries) > 0 {
		parts := make([]string, 0, len(queries))
		for _, q := range queries {
			if s, ok := q.(string); ok && s != "" {
				parts = append(parts, truncateRunes(s, 50))
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func summarizePathTool(raw map[string]any, pathKey string, extraKeys ...string) string {
	fields := []summaryField{{pathKey, 0}}
	for _, key := range extraKeys {
		fields = append(fields, summaryField{key, 0})
	}
	return joinPresent(raw, fields)
}

func summarizeSubagent(raw map[string]any) string {
	fields := []summaryField{{"agent_name", 40}, {"type", 40}, {"task", 90}, {"timeout", 20}}
	return joinPresent(raw, fields)
}

func joinPresent(raw map[string]any, fields []summaryField) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		v, ok := raw[field.key]
		if !ok {
			continue
		}
		val := fmt.Sprint(v)
		if val == "" {
			continue
		}
		if field.maxRunes > 0 {
			val = truncateRunes(val, field.maxRunes)
		}
		parts = append(parts, field.key+": "+val)
	}
	return strings.Join(parts, "  ")
}

func summarizeKeyValues(raw map[string]any) string {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+truncateRunes(fmt.Sprint(raw[k]), 80))
	}
	return strings.Join(parts, "  ")
}

func stringArg(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}
