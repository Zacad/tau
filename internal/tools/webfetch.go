package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown"
	"github.com/adam/tau/internal/types"
)

const (
	webfetchDefaultTimeout = 30
	webfetchMaxTimeout     = 120
	webfetchMaxResponseSize = 5 * 1024 * 1024
	webfetchMaxHTMLSize    = 2 * 1024 * 1024
	webfetchUserAgent      = "tau-agent"
)

var binaryContentTypes = []string{
	"application/pdf",
	"application/zip",
	"application/x-tar",
	"application/gzip",
	"application/x-gzip",
	"image/",
	"video/",
	"audio/",
	"application/octet-stream",
}

type WebFetchParams struct {
	URL     string `json:"url" jsonschema:"required,description=URL to fetch (must be http or https)"`
	Format  string `json:"format,omitempty" jsonschema:"description=Output format: markdown (default), text, or html"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (max 120, default 30)"`
}

type WebFetchTool struct {
	maxChars     int
	skipSSRF     bool
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{maxChars: DefaultMaxOutputChars}
}

func NewWebFetchToolForTest() *WebFetchTool {
	return &WebFetchTool{maxChars: DefaultMaxOutputChars, skipSSRF: true}
}

func (t *WebFetchTool) Name() string { return "webfetch" }

func (t *WebFetchTool) Description() string {
	return "Fetch the content of a web page and convert it to markdown. Use this when you have a specific URL and need to read its content. " +
		"The URL must be a valid http or https URL. This tool is read-only and does not modify any files. " +
		"Prefer websearch when you need to discover information; use webfetch when you already know the URL you want to read. " +
		"Results may be truncated for very large pages."
}

func (t *WebFetchTool) Parameters() any { return &WebFetchParams{} }

func (t *WebFetchTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

func (t *WebFetchTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*WebFetchParams)

	if p.URL == "" {
		return errorResult("URL is required"), nil
	}

	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		return errorResult("URL must start with http:// or https://"), nil
	}

	if !t.skipSSRF {
		if err := validateURL(p.URL); err != nil {
			return errorResult(err.Error()), nil
		}
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = webfetchDefaultTimeout
	}
	if timeout > webfetchMaxTimeout {
		timeout = webfetchMaxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return errorResult(fmt.Sprintf("Invalid URL: %v", err)), nil
	}
	req.Header.Set("User-Agent", webfetchUserAgent)

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if err := validateURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect to private IP blocked: %v", err)
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return errorResult("Request cancelled."), nil
		}
		if ctx.Err() == context.DeadlineExceeded {
			return errorResult(fmt.Sprintf("Request timed out after %ds.", timeout)), nil
		}
		return errorResult(formatHTTPErr(err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errorResult(fmt.Sprintf("Page not found: %s", p.URL)), nil
	}
	if resp.StatusCode == http.StatusForbidden {
		return errorResult(fmt.Sprintf("Access denied by %s. The site may block automated requests.", extractHost(p.URL))), nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return errorResult(fmt.Sprintf("Rate limited by %s. Try again later.", extractHost(p.URL))), nil
	}
	if resp.StatusCode >= 500 {
		return errorResult(fmt.Sprintf("Server error at %s (HTTP %d)", extractHost(p.URL), resp.StatusCode)), nil
	}
	if resp.StatusCode >= 400 {
		return errorResult(fmt.Sprintf("HTTP error %d from %s", resp.StatusCode, extractHost(p.URL))), nil
	}

	contentType := resp.Header.Get("Content-Type")
	if isBinaryContent(contentType) {
		return &types.ToolResult{
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Binary content (%s, %d bytes). URL: %s", contentType, resp.ContentLength, p.URL),
			}},
		}, nil
	}

	lr := io.LimitReader(resp.Body, webfetchMaxResponseSize)
	body, err := io.ReadAll(lr)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to read response: %v", err)), nil
	}

	format := p.Format
	if format == "" {
		format = "markdown"
	}

	switch format {
	case "html":
		return t.formatOutput(string(body), p.URL), nil
	case "text":
		text := stripHTMLTags(string(body))
		return t.formatOutput(text, p.URL), nil
	default:
		htmlStr := string(body)
		if len(htmlStr) > webfetchMaxHTMLSize {
			htmlStr = htmlStr[:webfetchMaxHTMLSize]
		}
		htmlStr = stripStyleAndScript(htmlStr)
		markdown, err := htmlToMarkdown(htmlStr)
		if err != nil {
			text := stripHTMLTags(string(body))
			return t.formatOutput(text, p.URL), nil
		}
		return t.formatOutput(markdown, p.URL), nil
	}
}

func (t *WebFetchTool) formatOutput(content, url string) *types.ToolResult {
	truncated, err := Truncate(content, t.maxChars)
	if err != nil {
		truncated = &TruncateResult{Output: content}
	}
	return &types.ToolResult{
		Content: []types.ContentBlock{{Type: "text", Text: truncated.Output}},
		Details: map[string]any{
			"url":       url,
			"truncated": truncated.Truncated,
		},
	}
}

func validateURL(rawURL string) error {
	host := extractHost(rawURL)
	if host == "" {
		return fmt.Errorf("URL has no host")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("Failed to resolve hostname: %s", host)
	}

	for _, ip := range ips {
		if isPrivateIP(ip.String()) {
			return fmt.Errorf("URL blocked: resolves to private/reserved IP address (%s)", ip)
		}
	}
	return nil
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalUnicast() {
		return true
	}
	if ip.IsLinkLocalMulticast() {
		return true
	}

	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"169.254.0.0/16"},
		{"fc00::/7"},
	}

	for _, r := range privateRanges {
		_, cidr, err := net.ParseCIDR(r.network)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

func isBinaryContent(contentType string) bool {
	ct := strings.ToLower(contentType)
	for _, bin := range binaryContentTypes {
		if strings.HasPrefix(ct, bin) {
			return true
		}
	}
	return false
}

func htmlToMarkdown(html string) (string, error) {
	converter := md.NewConverter("", true, nil)
	return converter.ConvertString(html)
}

var styleScriptRe = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>|<script[^>]*>.*?</script>`)

func stripStyleAndScript(html string) string {
	return styleScriptRe.ReplaceAllString(html, "")
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTMLTags(html string) string {
	return htmlTagRe.ReplaceAllString(html, "")
}

func extractHost(rawURL string) string {
	parts := strings.SplitN(rawURL, "//", 2)
	if len(parts) < 2 {
		return ""
	}
	hostPart := parts[1]
	if idx := strings.Index(hostPart, "/"); idx >= 0 {
		hostPart = hostPart[:idx]
	}
	if idx := strings.Index(hostPart, ":"); idx >= 0 {
		hostPart = hostPart[:idx]
	}
	return hostPart
}

func formatHTTPErr(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") {
		return fmt.Sprintf("Connection refused: %s", extractHostFromErr(msg))
	}
	if strings.Contains(msg, "no such host") || strings.Contains(msg, "lookup") {
		return fmt.Sprintf("Failed to resolve hostname: %s", extractHostFromErr(msg))
	}
	return fmt.Sprintf("Request failed: %s", msg)
}

func extractHostFromErr(msg string) string {
	parts := strings.SplitN(msg, " ", -1)
	for _, p := range parts {
		if strings.Contains(p, ".") || strings.Contains(p, ":") {
			return strings.Trim(p, "\"':")
		}
	}
	return ""
}

func errorResult(msg string) *types.ToolResult {
	return &types.ToolResult{
		IsError: true,
		Content: []types.ContentBlock{{Type: "text", Text: msg}},
	}
}
