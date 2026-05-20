package provider

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// maxRetries is the maximum number of retry attempts for failed requests.
	maxRetries = 2
	// initialBackoff is the initial delay between retries (exponential backoff).
	initialBackoff = 1 * time.Second
	// maxBackoff is the maximum delay between retries.
	maxBackoff = 60 * time.Second
)

// DefaultHTTPClient wraps net/http for production use.
type DefaultHTTPClient struct{}

// newHTTPRequest converts our Request type to a net/http.Request.
func newHTTPRequest(req *Request) (*http.Request, error) {
	method := req.Method
	if method == "" {
		method = http.MethodPost
	}

	var body io.Reader
	if req.Body != nil {
		body = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	if req.Headers != nil {
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}
	}

	return httpReq, nil
}

// executeHTTPRequest executes a net/http request with retry logic.
// It recreates the request for each retry to handle body replay.
func executeHTTPRequest(httpReq *http.Request) (*Response, error) {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	// Read the original body so we can replay it on retries
	var bodyBytes []byte
	if httpReq.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(httpReq.Body)
		httpReq.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry (after the first attempt)
			delay := backoffDelay(attempt - 1)
			time.Sleep(delay)
		}

		// Clone the request with a fresh body reader
		cloneReq := httpReq.Clone(httpReq.Context())
		cloneReq.Body = nil
		if bodyBytes != nil {
			cloneReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			cloneReq.ContentLength = int64(len(bodyBytes))
		}

		resp, err := client.Do(cloneReq)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed (attempt %d/%d): %w", attempt+1, maxRetries+1, err)
			continue
		}

		bodyRead, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("reading response body: %w", readErr)
			continue
		}

		// Build response headers map
		headerMap := make(map[string][]string)
		for k, v := range resp.Header {
			headerMap[k] = v
		}

		result := &Response{
			StatusCode: resp.StatusCode,
			Header:     headerMap,
			Body:       bodyRead,
		}

		// Handle rate limiting (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			// Check if this is a non-retryable limit (quota exceeded, weekly limit, etc.)
			if !isRetryableRateLimit(bodyRead) {
				return result, fmt.Errorf("rate limit exceeded: %s", truncateString(string(bodyRead), 500))
			}

			retryAfter := parseRetryAfterHeader(resp.Header)
			if retryAfter > 0 {
				if attempt < maxRetries {
					time.Sleep(retryAfter)
					continue
				}
			} else {
				// No Retry-After header, use exponential backoff
				delay := backoffDelay(attempt)
				if attempt < maxRetries {
					time.Sleep(delay)
					continue
				}
			}
			return result, fmt.Errorf("rate limited after %d retries", attempt)
		}

		// Handle server errors (5xx) with retry
		if resp.StatusCode >= 500 {
			if attempt < maxRetries {
				continue
			}
			return result, fmt.Errorf("server error %d: %s", resp.StatusCode, truncateString(string(bodyRead), 500))
		}

		return result, nil
	}

	return nil, lastErr
}

// backoffDelay calculates exponential backoff delay.
func backoffDelay(attempt int) time.Duration {
	delay := time.Duration(float64(initialBackoff) * math.Pow(2, float64(attempt)))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

// parseRetryAfterHeader parses the Retry-After header value.
func parseRetryAfterHeader(header http.Header) time.Duration {
	val := header.Get("Retry-After")
	if val == "" {
		return 0
	}

	// Try parsing as seconds (integer)
	if seconds, err := strconv.Atoi(val); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(val); err == nil {
		return time.Until(t)
	}

	return 0
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// isRetryableRateLimit checks if a 429 response body indicates a retryable rate limit.
// Returns false for non-retryable errors like quota exceeded, weekly limits, etc.
func isRetryableRateLimit(body []byte) bool {
	bodyStr := strings.ToLower(string(body))

	// Non-retryable limit indicators
	nonRetryable := []string{
		"quota",
		"weekly limit",
		"monthly limit",
		"daily limit",
		"billing",
		"subscription",
		"upgrade",
		"insufficient credits",
		"credit balance",
	}

	for _, indicator := range nonRetryable {
		if strings.Contains(bodyStr, indicator) {
			return false
		}
	}

	return true
}

// Do implements HTTPClient using net/http with retry logic.
func (c *DefaultHTTPClient) Do(req *Request) (*Response, error) {
	httpReq, err := newHTTPRequest(req)
	if err != nil {
		return nil, err
	}
	return executeHTTPRequest(httpReq)
}

// Ensure DefaultHTTPClient implements HTTPClient at compile time.
var _ HTTPClient = (*DefaultHTTPClient)(nil)

// MockHTTPClient is a simple HTTPClient that returns a pre-configured response.
// Useful for testing without httptest.
type MockHTTPClient struct {
	DoFunc func(req *Request) (*Response, error)
}

// Do calls the configured function.
func (m *MockHTTPClient) Do(req *Request) (*Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	return &Response{StatusCode: 200, Body: []byte("{}")}, nil
}

// SSELineReader reads Server-Sent Events from a reader.
// Each event is a block of "field: value" lines separated by blank lines.
type SSELineReader struct {
	data   []byte
	pos    int
	fields map[string]string
}

// NewSSELineReader creates a new SSE reader.
func NewSSELineReader(data []byte) *SSELineReader {
	return &SSELineReader{
		data:   data,
		pos:    0,
		fields: make(map[string]string),
	}
}

// ReadNext parses the next SSE event and returns its fields.
// Returns false when no more events are available.
func (r *SSELineReader) ReadNext() (map[string]string, bool) {
	if r.pos >= len(r.data) {
		return nil, false
	}

	fields := make(map[string]string)
	for r.pos < len(r.data) {
		// Find end of line
		end := bytes.IndexByte(r.data[r.pos:], '\n')
		if end == -1 {
			end = len(r.data) - r.pos
		}

		line := string(r.data[r.pos : r.pos+end])
		r.pos += end + 1 // +1 for the newline

		// Skip empty lines (event separator)
		line = strings.TrimSpace(line)
		if line == "" {
			if len(fields) > 0 {
				return fields, true
			}
			continue
		}

		// Parse "field: value"
		if colon := strings.Index(line, ":"); colon != -1 {
			key := strings.TrimSpace(line[:colon])
			value := strings.TrimSpace(line[colon+1:])
			if key == "data" {
				// Append multi-line data
				if existing, ok := fields["data"]; ok {
					fields["data"] = existing + "\n" + value
				} else {
					fields["data"] = value
				}
			} else {
				fields[key] = value
			}
		}
	}

	if len(fields) > 0 {
		return fields, true
	}
	return nil, false
}
