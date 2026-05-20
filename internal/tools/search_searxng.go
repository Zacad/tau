package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log/slog"
)

type SearXNGBackend struct {
	baseURL    string
	httpClient *http.Client
}

func NewSearXNGBackend(baseURL string, timeout time.Duration) *SearXNGBackend {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SearXNGBackend{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: newBackendHTTPClient(timeout),
	}
}

func (b *SearXNGBackend) Name() string { return "searxng" }

func (b *SearXNGBackend) Available() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(b.baseURL + "/healthz")
	if err != nil {
		resp, err = client.Get(b.baseURL + "/")
		if err != nil {
			return false
		}
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

func (b *SearXNGBackend) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	reqURL := fmt.Sprintf("%s/search?q=%s&format=json&categories=general", b.baseURL, url.QueryEscape(query))
	if opts.MaxResults > 0 {
		reqURL += fmt.Sprintf("&pageno=1")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("searxng request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("searxng read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &searchError{Code: resp.StatusCode, Message: fmt.Sprintf("authentication error: %s", truncateBody(body))}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &searchError{Code: resp.StatusCode, Message: fmt.Sprintf("rate limited: %s", truncateBody(body))}
	}
	if resp.StatusCode >= 500 {
		return nil, &searchError{Code: resp.StatusCode, Message: fmt.Sprintf("server error: %s", truncateBody(body))}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &searchError{Code: resp.StatusCode, Message: fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, truncateBody(body))}
	}

	var sResp searxngResponse
	if err := json.Unmarshal(body, &sResp); err != nil {
		return nil, fmt.Errorf("searxng parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(sResp.Results))
	limit := opts.MaxResults
	if limit <= 0 {
		limit = defaultMaxSearchResults
	}

	for i, r := range sResp.Results {
		if i >= limit {
			break
		}
		sr := SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		}
		results = append(results, sr)
	}

	slog.Debug("searxng: search completed", "query", query, "results", len(results))
	return results, nil
}

func truncateBody(body []byte) string {
	const maxLen = 512
	s := strings.TrimSpace(string(body))
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}
