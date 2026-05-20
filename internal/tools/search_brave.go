package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"encoding/json"

	"log/slog"
)

const defaultBraveBaseURL = "https://api.search.brave.com"

type BraveBackend struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewBraveBackend(apiKey string, timeout time.Duration) *BraveBackend {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &BraveBackend{
		apiKey:     apiKey,
		baseURL:    defaultBraveBaseURL,
		httpClient: newBackendHTTPClient(timeout),
	}
}

func (b *BraveBackend) Name() string { return "brave" }

func (b *BraveBackend) Available() bool { return b.apiKey != "" }

type braveResponse struct {
	Web struct {
		Results []braveResult `json:"results"`
	} `json:"web"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (b *BraveBackend) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxSearchResults
	}

	reqURL := fmt.Sprintf("%s/res/v1/web/search?q=%s&count=%d", b.baseURL, url.QueryEscape(query), maxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("brave request: %w", err)
	}
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &searchError{Code: resp.StatusCode, Message: "invalid API key"}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &searchError{Code: resp.StatusCode, Message: "rate limited"}
	}
	if resp.StatusCode >= 500 {
		return nil, &searchError{Code: resp.StatusCode, Message: "server error"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &searchError{Code: resp.StatusCode, Message: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("brave read response: %w", err)
	}

	var bResp braveResponse
	if err := json.Unmarshal(body, &bResp); err != nil {
		return nil, fmt.Errorf("brave parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(bResp.Web.Results))
	for _, r := range bResp.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	slog.Debug("brave: search completed", "query", query, "results", len(results))
	return results, nil
}
