package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"log/slog"
)

const defaultTavilyBaseURL = "https://api.tavily.com"

type TavilyBackend struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewTavilyBackend(apiKey string, timeout time.Duration) *TavilyBackend {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &TavilyBackend{
		apiKey:     apiKey,
		baseURL:    defaultTavilyBaseURL,
		httpClient: newBackendHTTPClient(timeout),
	}
}

func (b *TavilyBackend) Name() string { return "tavily" }

func (b *TavilyBackend) Available() bool { return b.apiKey != "" }

type tavilyRequest struct {
	APIKey           string `json:"api_key"`
	Query            string `json:"query"`
	MaxResults       int    `json:"max_results"`
	IncludeRawContent bool   `json:"include_raw_content"`
	SearchDepth      string `json:"search_depth"`
}

type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	RawContent string `json:"raw_content"`
	Score   float64 `json:"score"`
}

func (b *TavilyBackend) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxSearchResults
	}

	reqBody := tavilyRequest{
		APIKey:           b.apiKey,
		Query:            query,
		MaxResults:       maxResults,
		IncludeRawContent: opts.IncludeContent,
		SearchDepth:      "basic",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tavily request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily request failed: %w", err)
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily read response: %w", err)
	}

	var tResp tavilyResponse
	if err := json.Unmarshal(respBody, &tResp); err != nil {
		return nil, fmt.Errorf("tavily parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(tResp.Results))
	for _, r := range tResp.Results {
		sr := SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Score:   r.Score,
		}
		if opts.IncludeContent && r.RawContent != "" {
			sr.Content = r.RawContent
		} else if opts.IncludeContent && r.Content != "" {
			sr.Content = r.Content
		}
		results = append(results, sr)
	}

	slog.Debug("tavily: search completed", "query", query, "results", len(results))
	return results, nil
}
