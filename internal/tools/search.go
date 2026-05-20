package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/adam/tau/internal/types"
)

const defaultMaxSearchResults = 8

type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Content string  `json:"content,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

type SearchOptions struct {
	MaxResults     int
	IncludeContent bool
}

type SearchBackend interface {
	Name() string
	Available() bool
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
}

type searchError struct {
	Code    int
	Message string
}

func (e *searchError) Error() string {
	return fmt.Sprintf("search error %d: %s", e.Code, e.Message)
}

type PartialResultError struct {
	Results []SearchResult
	Err     error
}

func (e *PartialResultError) Error() string {
	return fmt.Sprintf("partial results (%d): %v", len(e.Results), e.Err)
}

func (e *PartialResultError) Unwrap() error { return e.Err }

type WebSearchParams struct {
	Query          string   `json:"query,omitempty" jsonschema:"description=Search query"`
	Q              string   `json:"q,omitempty" jsonschema:"description=Search query (alias for query)"`
	Queries        []string `json:"queries,omitempty" jsonschema:"description=Search queries (alternative to query)"`
	MaxResults     int      `json:"maxResults,omitempty" jsonschema:"description=Maximum results (default 8)"`
	IncludeContent bool     `json:"includeContent,omitempty" jsonschema:"description=Include page content in results (uses more tokens)"`
}

type WebSearchTool struct {
	backends  []SearchBackend
	date      time.Time
	degraded  map[string]bool
	degradedMu sync.Mutex
	maxChars  int
}

func NewWebSearchTool(backends []SearchBackend, date time.Time) *WebSearchTool {
	if len(backends) == 0 {
		return nil
	}
	return &WebSearchTool{
		backends: backends,
		date:     date,
		degraded: make(map[string]bool),
		maxChars: DefaultMaxOutputChars,
	}
}

func (t *WebSearchTool) Name() string { return "websearch" }

func (t *WebSearchTool) Description() string {
	return fmt.Sprintf(
		"Search the web for information. Use this tool when you need current, up-to-date information "+
			"that may not be in your training data — such as recent library versions, documentation, "+
			"error solutions, or current best practices. The current date is %s. "+
			"Always cite URLs from search results. Never fabricate or modify search results. "+
			"Use the websearch tool to search the web. "+
			"Do not use non-existent aliases like google_search, search_web, or web_lookup.",
		t.date.Format("January 2, 2006"),
	)
}

func (t *WebSearchTool) Parameters() any { return &WebSearchParams{} }

func (t *WebSearchTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

func (t *WebSearchTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*WebSearchParams)

	// Resolve query from any of the accepted fields
	query := strings.TrimSpace(p.Query)
	if query == "" {
		query = strings.TrimSpace(p.Q)
	}
	if query == "" && len(p.Queries) > 0 {
		query = strings.TrimSpace(p.Queries[0])
	}

	if query == "" {
		slog.Warn("websearch: empty query received", "params", fmt.Sprintf("%+v", p))
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{Type: "text", Text: "Web search failed: query parameter is required. Please provide a search query."}},
		}, nil
	}

	maxResults := p.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxSearchResults
	}

	opts := SearchOptions{
		MaxResults:     maxResults,
		IncludeContent: p.IncludeContent,
	}

	var allErrors []string

	for _, backend := range t.backends {
		if t.isDegraded(backend.Name()) {
			slog.Debug("websearch: skipping degraded backend", "backend", backend.Name())
			continue
		}

		results, err := backend.Search(ctx, query, opts)
		if err != nil {
			if ctx.Err() != nil {
				return &types.ToolResult{
					IsError: true,
					Content: []types.ContentBlock{{Type: "text", Text: "Web search cancelled."}},
				}, nil
			}

			if pre, ok := err.(*PartialResultError); ok && len(pre.Results) > 0 {
				slog.Warn("websearch: backend returned partial results", "backend", backend.Name(), "results", len(pre.Results), "error", pre.Err)
				result := t.formatResults(pre.Results, p.IncludeContent)
				result.Content[0].Text += "\n\nWarning: search was incomplete. Some results may be missing."
				return result, nil
			}

			allErrors = append(allErrors, fmt.Sprintf("%s: %v", backend.Name(), err))

			if se, ok := err.(*searchError); ok {
				if se.Code == 401 || se.Code == 403 {
					t.markDegraded(backend.Name())
					slog.Warn("websearch: backend auth error, marking degraded", "backend", backend.Name(), "code", se.Code)
				}
			}

			slog.Debug("websearch: backend failed, trying next", "backend", backend.Name(), "error", err)
			continue
		}

		return t.formatResults(results, p.IncludeContent), nil
	}

	if len(allErrors) == 0 {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{Type: "text", Text: "Web search failed: no backends available."}},
		}, nil
	}

	return &types.ToolResult{
		IsError: true,
		Content: []types.ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf("Web search failed: all backends unavailable. [%s]", strings.Join(allErrors, "; ")),
		}},
	}, nil
}

func (t *WebSearchTool) formatResults(results []SearchResult, includeContent bool) *types.ToolResult {
	var sb strings.Builder

	if len(results) == 0 {
		sb.WriteString("No results found for query.")
	} else {
		for i, r := range results {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(fmt.Sprintf("## %d. %s\n", i+1, r.Title))
			sb.WriteString(fmt.Sprintf("URL: %s\n", r.URL))
			if r.Snippet != "" {
				sb.WriteString(fmt.Sprintf("Snippet: %s\n", r.Snippet))
			}
			if includeContent && r.Content != "" {
				sb.WriteString(fmt.Sprintf("Content: %s\n", r.Content))
			}
			if r.Score > 0 {
				sb.WriteString(fmt.Sprintf("Relevance: %.2f\n", r.Score))
			}
		}
	}

	text := sb.String()
	truncated, err := Truncate(text, t.maxChars)
	if err != nil {
		truncated = &TruncateResult{Output: text}
	}

	return &types.ToolResult{
		Content: []types.ContentBlock{{Type: "text", Text: truncated.Output}},
		Details: map[string]any{
			"resultCount": len(results),
			"truncated":   truncated.Truncated,
		},
	}
}

func (t *WebSearchTool) isDegraded(name string) bool {
	t.degradedMu.Lock()
	defer t.degradedMu.Unlock()
	return t.degraded[name]
}

func (t *WebSearchTool) markDegraded(name string) {
	t.degradedMu.Lock()
	defer t.degradedMu.Unlock()
	t.degraded[name] = true
}

func isRetryableHTTPCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case 401, 403:
		return false
	default:
		return code >= 500
	}
}

func newBackendHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}
