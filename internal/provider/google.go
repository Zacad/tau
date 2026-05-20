package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adam/tau/internal/types"
)

// GoogleProvider implements the Provider interface for the Google Generative AI API.
type GoogleProvider struct {
	baseProvider
	authMode string // "key-param" (default) or "bearer"
}

// GoogleConfig holds configuration for Google provider.
type GoogleConfig struct {
	// AuthMode is the authentication mode: "key-param" (default, uses ?key= in URL)
	// or "bearer" (uses Authorization header, for gateway providers like Zen).
	AuthMode string
}

// NewGoogleProvider creates a new Google provider with the given API key.
func NewGoogleProvider(apiKey string) *GoogleProvider {
	return &GoogleProvider{
		baseProvider: baseProvider{
			name:       "google",
			httpClient: &DefaultHTTPClient{},
			apiKey:     apiKey,
		},
		authMode: "key-param",
	}
}

// NewGoogleProviderWithClient creates a new Google provider with a custom HTTP client.
func NewGoogleProviderWithClient(apiKey string, client HTTPClient) *GoogleProvider {
	return &GoogleProvider{
		baseProvider: baseProvider{
			name:       "google",
			httpClient: client,
			apiKey:     apiKey,
		},
		authMode: "key-param",
	}
}

// NewGoogleProviderWithConfig creates a new Google provider with custom configuration.
func NewGoogleProviderWithConfig(apiKey string, config GoogleConfig, client HTTPClient) *GoogleProvider {
	authMode := config.AuthMode
	if authMode == "" {
		authMode = "key-param"
	}
	if client == nil {
		client = &DefaultHTTPClient{}
	}
	return &GoogleProvider{
		baseProvider: baseProvider{
			name:       "google",
			httpClient: client,
			apiKey:     apiKey,
		},
		authMode: authMode,
	}
}

// Stream sends messages to Google and returns a channel of streaming events.
func (p *GoogleProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	if err := p.apiKeyOrErr(); err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	var reqURL string
	var headers map[string]string

	if p.authMode == "bearer" {
		baseURL := model.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta"
		}
		reqURL = fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", strings.TrimRight(baseURL, "/"), model.ID)
		headers = map[string]string{
			"Content-Type":   "application/json",
			"x-goog-api-key": p.apiKey,
		}
	} else {
		baseURL := model.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta"
		}
		reqURL = fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", strings.TrimRight(baseURL, "/"), model.ID, p.apiKey)
		headers = map[string]string{"Content-Type": "application/json"}
	}

	body, err := p.buildRequest(model, messages, tools, opts)
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	httpResp, err := p.httpClient.Do(&Request{
		Method:  "POST",
		URL:     reqURL,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	if httpResp.StatusCode >= 400 {
		apiErr := types.ClassifyAPIError(httpResp.StatusCode, httpResp.Body)
		return streamToChannel(ctx, []types.StreamEvent{{
			Type:  types.EventError,
			Error: apiErr.UserMessage(),
		}})
	}

	return p.parseStreamResponse(ctx, httpResp.Body, model.ID)
}

// Complete sends messages to Google and returns the full response.
func (p *GoogleProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	if err := p.apiKeyOrErr(); err != nil {
		return nil, err
	}

	events := p.Stream(ctx, model, messages, tools, opts)
	return p.collectFromStream(events)
}

func (p *GoogleProvider) buildRequest(model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) ([]byte, error) {
	req := googleRequest{}

	// Build system instruction
	if opts.SystemPrompt != "" {
		req.SystemInstruction = &googleSystemInstruction{
			Parts: []googlePart{{Text: opts.SystemPrompt}},
		}
	}

	// Build contents from messages
	var googleMsgs []googleContentMsg
	for _, msg := range messages {
		gMsg := messageToGoogle(msg)
		if gMsg != nil {
			googleMsgs = append(googleMsgs, *gMsg)
		}
	}
	req.Contents = googleMsgs

	// Build tools
	if len(tools) > 0 {
		for _, t := range tools {
			decl := googleFunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
			}
		if t.Parameters != nil {
			raw, err := json.Marshal(t.Parameters)
			if err != nil {
				return nil, fmt.Errorf("marshaling tool parameters: %w", err)
			}
			var schemaMap map[string]any
			if err := json.Unmarshal(raw, &schemaMap); err != nil {
				return nil, fmt.Errorf("unmarshaling tool parameters: %w", err)
			}
			decl.Parameters = sanitizeGoogleSchema(schemaMap)
		}
			req.Tools = append(req.Tools, googleTool{
				FunctionDeclarations: []googleFunctionDeclaration{decl},
			})
		}
	}

	// Generation config
	req.GenerationConfig = &googleGenerationConfig{
		Temperature:     opts.Temperature,
		MaxOutputTokens: opts.MaxTokens,
	}

	// Add thinking config if supported
	if opts.ThinkingLevel != "" && opts.ThinkingLevel != types.ThinkingOff {
		// Use model's thinking level map for provider-specific values
		mapped := model.MapThinkingLevel(opts.ThinkingLevel)
		
		// Check if it's a numeric budget (Gemini 2.x) or a level (Gemini 3/Gemma)
		if isGoogleThinkingLevel(mapped) {
			req.GenerationConfig.ThinkingConfig = &googleThinkingConfig{
				ThinkingLevel:  mapped,
				IncludeThoughts: true,
			}
		} else {
			// Token budget (Gemini 2.x)
			budget := parseGoogleThinkingBudget(mapped)
			if budget > 0 {
				req.GenerationConfig.ThinkingConfig = &googleThinkingConfig{
					ThinkingBudget:  budget,
					IncludeThoughts: true,
				}
			}
		}
	}

	return json.Marshal(req)
}

func (p *GoogleProvider) parseStreamResponse(ctx context.Context, body []byte, modelID string) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)

		msg := &types.AgentMessage{
			Role:  types.RoleAssistant,
			Model: modelID,
			API:   "google-generative-ai",
		}

		// Google SSE sends one JSON object per line (not standard SSE format)
		lines := strings.Split(string(body), "\n")
		for _, line := range lines {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Strip "data: " prefix if present
			line = strings.TrimPrefix(line, "data: ")

			var resp googleStreamResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				continue
			}

			if len(resp.Candidates) == 0 {
				continue
			}

		candidate := resp.Candidates[0]

		// Process content (Gemini sends text and finishReason in the same event)
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					msg.Content = append(msg.Content, types.ContentBlock{
						Type: types.BlockText,
						Text: part.Text,
					})
					sendEvent(ctx, ch, types.StreamEvent{
						Type:    types.EventTextDelta,
						Delta:   part.Text,
						Message: msg,
					})
				}
				if part.FunctionCall != nil {
					args := make(map[string]any)
					if part.FunctionCall.Args != nil {
						args = part.FunctionCall.Args
					}
					msg.Content = append(msg.Content, types.ContentBlock{
						Type: types.BlockToolCall,
						ToolCall: &types.ToolCallBlock{
							ID:        part.FunctionCall.Name,
							Name:      part.FunctionCall.Name,
							Arguments: args,
						},
					})
					sendEvent(ctx, ch, types.StreamEvent{
						Type:  types.EventToolCallStart,
						Delta: part.FunctionCall.Name,
					})
				}
			}
		}

		// Check for finish reason
		if candidate.FinishReason != "" {
			usage := types.Usage{}
			if resp.UsageMetadata != nil {
				usage.Input = resp.UsageMetadata.PromptTokenCount
				usage.Output = resp.UsageMetadata.CandidatesTokenCount
				usage.TotalTokens = resp.UsageMetadata.TotalTokenCount
			}
			sendEvent(ctx, ch, types.StreamEvent{
				Type:    types.EventDone,
				Message: msg,
				Usage:   &usage,
			})
			continue
		}
		}
	}()

	return ch
}

func (p *GoogleProvider) collectFromStream(ch <-chan types.StreamEvent) (*types.AgentMessage, error) {
	var lastMsg *types.AgentMessage

	for event := range ch {
		switch event.Type {
		case types.EventError:
			return nil, fmt.Errorf("Google stream error: %s", event.Error)
		case types.EventDone:
			if event.Message != nil {
				lastMsg = event.Message
			}
		case types.EventTextDelta:
			if event.Message != nil {
				lastMsg = event.Message
			}
		}
	}

	if lastMsg == nil {
		return nil, fmt.Errorf("no response from Google")
	}

	return lastMsg, nil
}

// Google request/response types

type googleRequest struct {
	Contents           []googleContentMsg          `json:"contents,omitempty"`
	SystemInstruction  *googleSystemInstruction    `json:"systemInstruction,omitempty"`
	Tools              []googleTool                `json:"tools,omitempty"`
	GenerationConfig   *googleGenerationConfig     `json:"generationConfig,omitempty"`
}

type googleContentMsg struct {
	Role  string        `json:"role"`
	Parts []googlePart  `json:"parts"`
}

type googlePart struct {
	Text         string                  `json:"text,omitempty"`
	FunctionCall *googleFunctionCallPart `json:"functionCall,omitempty"`
}

type googleFunctionCallPart struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type googleSystemInstruction struct {
	Parts []googlePart `json:"parts"`
}

type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"functionDeclarations"`
}

type googleFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

type googleGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *googleThinkingConfig `json:"thinkingConfig,omitempty"`
}

type googleThinkingConfig struct {
	ThinkingBudget int    `json:"thinkingBudget,omitempty"`
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
	IncludeThoughts bool  `json:"includeThoughts,omitempty"`
}

type googleStreamResponse struct {
	Candidates    []googleCandidate   `json:"candidates"`
	UsageMetadata *googleUsageMetadata `json:"usageMetadata,omitempty"`
}

type googleCandidate struct {
	Content      *googleContent `json:"content"`
	FinishReason string         `json:"finishReason,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role"`
	Parts []googlePart `json:"parts"`
}

type googleUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func messageToGoogle(msg types.AgentMessage) *googleContentMsg {
	var role string
	switch msg.Role {
	case types.RoleUser:
		role = "user"
	case types.RoleAssistant:
		role = "model"
	case types.RoleToolResult:
		role = "user" // Google doesn't have a separate tool role
	default:
		return nil
	}

	var parts []googlePart
	for _, block := range msg.Content {
		switch block.Type {
		case types.BlockText:
			parts = append(parts, googlePart{Text: block.Text})
		case types.BlockToolCall:
			if block.ToolCall != nil {
				parts = append(parts, googlePart{
					FunctionCall: &googleFunctionCallPart{
						Name: block.ToolCall.Name,
						Args: block.ToolCall.Arguments,
					},
				})
			}
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &googleContentMsg{Role: role, Parts: parts}
}

// googleSchemaMetaFields are JSON Schema meta fields that Google's API rejects.
var googleSchemaMetaFields = map[string]bool{
	"$schema": true,
	"$id":     true,
	"$ref":    true,
	"$defs":   true,
}

// sanitizeGoogleSchema removes JSON Schema meta fields that Google's API rejects
// and converts integer enums to string enums (Google requirement).
// It also inlines $ref definitions from $defs (Google doesn't support $ref).
// Reference: OpenCode packages/opencode/src/provider/transform.ts — sanitizeGemini()
func sanitizeGoogleSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	// First pass: collect all values, skip meta fields
	result := make(map[string]any)
	for key, value := range schema {
		if googleSchemaMetaFields[key] {
			continue
		}

		if m, ok := value.(map[string]any); ok {
			result[key] = sanitizeGoogleSchema(m)
		} else if arr, ok := value.([]any); ok {
			sanitized := make([]any, len(arr))
			for i, item := range arr {
				if m, ok := item.(map[string]any); ok {
					sanitized[i] = sanitizeGoogleSchema(m)
				} else {
					sanitized[i] = item
				}
			}
			result[key] = sanitized
		} else {
			result[key] = value
		}
	}

	// Inline $ref from $defs if present (Google doesn't support $ref)
	if ref, ok := schema["$ref"].(string); ok {
		if defs, ok := schema["$defs"].(map[string]any); ok {
			refName := strings.TrimPrefix(ref, "#/$defs/")
			if def, ok := defs[refName]; ok {
				if defMap, ok := def.(map[string]any); ok {
					// Merge the definition into the result
					for k, v := range defMap {
						if _, exists := result[k]; !exists {
							if m, ok := v.(map[string]any); ok {
								result[k] = sanitizeGoogleSchema(m)
							} else if arr, ok := v.([]any); ok {
								sanitized := make([]any, len(arr))
								for i, item := range arr {
									if m, ok := item.(map[string]any); ok {
										sanitized[i] = sanitizeGoogleSchema(m)
									} else {
										sanitized[i] = item
									}
								}
								result[k] = sanitized
							} else {
								result[k] = v
							}
						}
					}
				}
			}
		}
	}

	// Second pass: convert integer/number enums to string enums
	if arr, ok := result["enum"].([]any); ok {
		strEnums := make([]any, len(arr))
		for i, v := range arr {
			strEnums[i] = fmt.Sprintf("%v", v)
		}
		result["enum"] = strEnums
		if result["type"] == "integer" || result["type"] == "number" {
			result["type"] = "string"
		}
	}

	// Filter required array to only include fields that exist in properties
	if result["type"] == "object" {
		if props, ok := result["properties"].(map[string]any); ok {
			if req, ok := result["required"].([]any); ok {
				filtered := make([]any, 0, len(req))
				for _, r := range req {
					if s, ok := r.(string); ok {
						if _, exists := props[s]; exists {
							filtered = append(filtered, s)
						}
					}
				}
				if len(filtered) > 0 {
					result["required"] = filtered
				} else {
					delete(result, "required")
				}
			}
		}
	} else if result["type"] != nil && result["type"] != "object" {
		delete(result, "properties")
		delete(result, "required")
	}

	return result
}

// Ensure GoogleProvider implements Provider
var _ Provider = (*GoogleProvider)(nil)

// mapGoogleThinkingLevel maps a standardized thinking level to Google's API format.
func mapGoogleThinkingLevel(level types.ThinkingLevel) (string, bool) {
	switch level {
	case types.ThinkingMinimal:
		return "MINIMAL", true
	case types.ThinkingLow:
		return "LOW", true
	case types.ThinkingMedium:
		return "MEDIUM", true
	case types.ThinkingHigh:
		return "HIGH", true
	default:
		return "", false
	}
}

// isGoogleThinkingLevel checks if a value is a Google thinking level (vs a numeric budget).
func isGoogleThinkingLevel(v string) bool {
	switch v {
	case "MINIMAL", "LOW", "MEDIUM", "HIGH":
		return true
	default:
		return false
	}
}

// parseGoogleThinkingBudget parses a numeric thinking budget string.
func parseGoogleThinkingBudget(v string) int {
	var budget int
	fmt.Sscanf(v, "%d", &budget)
	return budget
}
