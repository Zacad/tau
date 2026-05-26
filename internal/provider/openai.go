package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adam/tau/internal/types"
)

const codexBaseURL = "https://chatgpt.com/backend-api/codex"

// OpenAIProvider implements the Provider interface for the OpenAI Responses API.
type OpenAIProvider struct {
	baseProvider
	oauthManager *OAuthManager
	codexBaseURL string
}

// NewOpenAIProvider creates a new OpenAI provider with the given API key.
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		baseProvider: baseProvider{
			name:       "openai",
			httpClient: &DefaultHTTPClient{},
			apiKey:     apiKey,
		},
	}
}

// NewOpenAIProviderWithClient creates a new OpenAI provider with a custom HTTP client.
func NewOpenAIProviderWithClient(apiKey string, client HTTPClient) *OpenAIProvider {
	return &OpenAIProvider{
		baseProvider: baseProvider{
			name:       "openai",
			httpClient: client,
			apiKey:     apiKey,
		},
	}
}

// NewOpenAIOAuthProvider creates a new OpenAI provider with OAuth credentials.
func NewOpenAIOAuthProvider(creds OAuthCredentials) *OpenAIProvider {
	return &OpenAIProvider{
		baseProvider: baseProvider{
			name:       "openai-oauth",
			httpClient: &DefaultHTTPClient{},
		},
		oauthManager: NewOAuthManager(creds, nil),
	}
}

// NewOpenAIOAuthProviderWithPersist creates a new OpenAI provider with OAuth credentials
// and a persistence callback for credential updates (e.g., token refresh).
func NewOpenAIOAuthProviderWithPersist(creds OAuthCredentials, persist PersistFunc) *OpenAIProvider {
	return &OpenAIProvider{
		baseProvider: baseProvider{
			name:       "openai-oauth",
			httpClient: &DefaultHTTPClient{},
		},
		oauthManager: NewOAuthManager(creds, persist),
	}
}

func (p *OpenAIProvider) isOAuth() bool {
	return p.oauthManager != nil
}

func (p *OpenAIProvider) getAccessToken() (string, error) {
	if p.isOAuth() {
		return p.oauthManager.GetAccessToken()
	}
	return p.apiKey, nil
}

func (p *OpenAIProvider) buildHeaders(model types.Model, accessToken string) map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
	}

	if p.isOAuth() {
		h["originator"] = "tau"
		creds := p.oauthManager.Credentials()
		if creds.AccountID != "" {
			h["chatgpt-account-id"] = creds.AccountID
		}
	}

	if len(model.Headers) > 0 {
		for k, v := range model.Headers {
			h[k] = v
		}
	}
	return h
}

func (p *OpenAIProvider) buildRequestURL(baseURL string) string {
	if p.isOAuth() {
		codexURL := p.codexBaseURL
		if codexURL == "" {
			codexURL = codexBaseURL
		}
		return codexURL + "/responses"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return baseURL + "/responses"
}

// Stream sends messages to OpenAI and returns a channel of streaming events.
func (p *OpenAIProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	accessToken, err := p.getAccessToken()
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	reqURL := p.buildRequestURL(model.BaseURL)

	body, err := p.buildStreamRequest(model, messages, tools, opts)
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	httpResp, err := p.httpClient.Do(&Request{
		Method:  "POST",
		URL:     reqURL,
		Headers: p.buildHeaders(model, accessToken),
		Body:    body,
	})
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	if httpResp.StatusCode >= 400 {
		apiErr := p.classifyError(httpResp.StatusCode, httpResp.Body)
		return streamToChannel(ctx, []types.StreamEvent{{
			Type:  types.EventError,
			Error: apiErr.UserMessage(),
		}})
	}

	return p.parseStreamResponse(ctx, httpResp.Body, model.ID)
}

// Complete sends messages to OpenAI and returns the full response.
func (p *OpenAIProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	accessToken, err := p.getAccessToken()
	if err != nil {
		return nil, err
	}

	reqURL := p.buildRequestURL(model.BaseURL)

	body, err := p.buildStreamRequest(model, messages, tools, opts)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.httpClient.Do(&Request{
		Method:  "POST",
		URL:     reqURL,
		Headers: p.buildHeaders(model, accessToken),
		Body:    body,
	})
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode >= 400 {
		return nil, p.classifyError(httpResp.StatusCode, httpResp.Body)
	}

	events := p.parseStreamResponse(ctx, httpResp.Body, model.ID)
	return p.collectFromStream(events)
}

func (p *OpenAIProvider) buildStreamRequest(model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) ([]byte, error) {
	req := openAIRequest{
		Model:           model.ID,
		Stream:          true,
		MaxOutputTokens: opts.MaxTokens,
		Temperature:     opts.Temperature,
	}

	if p.isOAuth() {
		storeFalse := false
		req.Store = &storeFalse
		req.Instructions = opts.SystemPrompt
	} else if opts.SystemPrompt != "" {
		req.Input = append(req.Input, openAIInputMessage{
			Role: "system",
			Content: []openAIInputTextContent{
				{Type: "input_text", Text: opts.SystemPrompt},
			},
		})
	}

	for _, msg := range messages {
		inputs := messageToOpenAI(msg)
		req.Input = append(req.Input, inputs...)
	}

	if model.Reasoning && opts.ThinkingLevel != "" && opts.ThinkingLevel != types.ThinkingOff {
		effort := model.MapThinkingLevel(opts.ThinkingLevel)
		req.Reasoning = &openAIReasoningConfig{
			Effort:  effort,
			Summary: "auto",
		}
		req.Include = append(req.Include, "reasoning.encrypted_content")
	}

	if len(tools) > 0 {
		for _, t := range tools {
			req.Tools = append(req.Tools, openAITool{
				Type:        "function",
				Name:        t.Name,
				Description: t.Description,
				Parameters:  toolDefToSchema(t),
			})
		}
	}

	return json.Marshal(req)
}

func (p *OpenAIProvider) classifyError(statusCode int, body []byte) *types.APIError {
	if !p.isOAuth() {
		return types.ClassifyAPIError(statusCode, body)
	}

	bodyStr := string(body)
	if strings.Contains(bodyStr, "rate_limit") || strings.Contains(bodyStr, "rate limit") {
		return &types.APIError{
			Type:       types.ErrorTypeRateLimit,
			StatusCode: statusCode,
			Message:    "Rate limit reached for your ChatGPT subscription. Please try again later.",
			Raw:        bodyStr,
		}
	}
	if strings.Contains(bodyStr, "insufficient_quota") || strings.Contains(bodyStr, "quota_exceeded") {
		return &types.APIError{
			Type:       types.ErrorTypeQuotaExceeded,
			StatusCode: statusCode,
			Message:    "Your ChatGPT subscription usage limit has been reached. Check your plan details for reset times.",
			Raw:        bodyStr,
		}
	}
	if strings.Contains(bodyStr, "invalid_token") || strings.Contains(bodyStr, "token_expired") {
		return &types.APIError{
			Type:       types.ErrorTypeAuthFailed,
			StatusCode: statusCode,
			Message:    "Authentication token expired. Please reconnect your ChatGPT account.",
			Raw:        bodyStr,
		}
	}

	return types.ClassifyAPIError(statusCode, body)
}

func (p *OpenAIProvider) parseStreamResponse(ctx context.Context, body []byte, modelID string) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)

		msg := &types.AgentMessage{
			Role:  types.RoleAssistant,
			Model: modelID,
			API:   "openai-responses",
		}

		var currentToolCall *types.ToolCallBlock
		var toolCallArgs strings.Builder

		reader := NewSSELineReader(body)
		for fields, ok := reader.ReadNext(); ok; fields, ok = reader.ReadNext() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// OpenAI uses 'event' field for the event type
			eventType := fields["event"]
			if eventType == "" {
				eventType = fields["type"]
			}
			data := fields["data"]

			switch eventType {
			case "response.output_text.delta":
				var delta openAIDelta
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					if delta.Delta != "" {
						msg.Content = append(msg.Content, types.ContentBlock{
							Type: types.BlockText,
							Text: delta.Delta,
						})
						sendEvent(ctx, ch, types.StreamEvent{
							Type:    types.EventTextDelta,
							Delta:   delta.Delta,
							Message: msg,
						})
					}
				}

			case "response.reasoning_summary_text.delta":
				var delta openAIDelta
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					if delta.Delta != "" {
						msg.Content = append(msg.Content, types.ContentBlock{
							Type: types.BlockThinking,
							Text: delta.Delta,
						})
						sendEvent(ctx, ch, types.StreamEvent{
							Type:    types.EventThinkingDelta,
							Delta:   delta.Delta,
							Message: msg,
						})
					}
				}

			case "response.output_item.added":
				var outputItem openAIOutputItem
				if err := json.Unmarshal([]byte(data), &outputItem); err == nil {
					item := outputItem.Item
					if item.Type == "function_call" {
						callID := item.CallID
						if callID == "" {
							callID = item.ID
						}
						currentToolCall = &types.ToolCallBlock{
							ID:        callID + "|" + item.ID,
							Name:      item.Name,
							Arguments: make(map[string]any),
						}
						toolCallArgs.Reset()
						sendEvent(ctx, ch, types.StreamEvent{
							Type:  types.EventToolCallStart,
							Delta: item.Name,
						})
					}
				}

			case "response.function_call_arguments.delta":
				var delta openAIDelta
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					if currentToolCall != nil && delta.Delta != "" {
						toolCallArgs.WriteString(delta.Delta)
					}
				}

			case "response.function_call_arguments.done":
				if currentToolCall != nil {
					argsStr := toolCallArgs.String()
					if argsStr != "" {
						var args map[string]any
						if err := json.Unmarshal([]byte(argsStr), &args); err == nil {
							currentToolCall.Arguments = args
						}
					}
					msg.Content = append(msg.Content, types.ContentBlock{
						Type:     types.BlockToolCall,
						ToolCall: currentToolCall,
					})
					sendEvent(ctx, ch, types.StreamEvent{
						Type:  types.EventToolCallEnd,
						Delta: currentToolCall.Name,
					})
					currentToolCall = nil
				}

			case "response.output_text.annotation.added":
				// Ignore annotations for now

			case "response.completed", "response.done":
				// Final usage
				var usageResp openAIUsageResponse
				if err := json.Unmarshal([]byte(data), &usageResp); err == nil {
					if usageResp.Usage != nil {
						usage := convertOpenAIUsage(usageResp.Usage)
						sendEvent(ctx, ch, types.StreamEvent{
							Type:    types.EventDone,
							Message: msg,
							Usage:   &usage,
						})
					} else {
						sendEvent(ctx, ch, types.StreamEvent{
							Type:    types.EventDone,
							Message: msg,
						})
					}
				} else {
					sendEvent(ctx, ch, types.StreamEvent{
						Type:    types.EventDone,
						Message: msg,
					})
				}

			case "error":
				sendEvent(ctx, ch, types.StreamEvent{
					Type:  types.EventError,
					Error: "OpenAI stream error: " + data,
				})
			}
		}
	}()

	return ch
}

func (p *OpenAIProvider) collectFromStream(ch <-chan types.StreamEvent) (*types.AgentMessage, error) {
	var lastMsg *types.AgentMessage
	var lastUsage *types.Usage

	for event := range ch {
		switch event.Type {
		case types.EventError:
			return nil, fmt.Errorf("OpenAI stream error: %s", event.Error)
		case types.EventDone:
			if event.Message != nil {
				lastMsg = event.Message
			}
			if event.Usage != nil {
				lastUsage = event.Usage
			}
		case types.EventTextDelta:
			if event.Message != nil {
				lastMsg = event.Message
			}
		case types.EventThinkingDelta:
			if event.Message != nil {
				lastMsg = event.Message
			}
		case types.EventToolCallStart, types.EventToolCallEnd:
			if event.Message != nil {
				lastMsg = event.Message
			}
		}
	}

	if lastMsg == nil {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Accumulate text from content blocks
	var text strings.Builder
	var thinking strings.Builder
	for _, block := range lastMsg.Content {
		switch block.Type {
		case types.BlockText:
			text.WriteString(block.Text)
		case types.BlockThinking:
			thinking.WriteString(block.Text)
		}
	}

	// Reset content to accumulated blocks
	var accumulated []types.ContentBlock
	if thinking.Len() > 0 {
		accumulated = append(accumulated, types.ContentBlock{Type: types.BlockThinking, Text: thinking.String()})
	}
	if text.Len() > 0 {
		accumulated = append(accumulated, types.ContentBlock{Type: types.BlockText, Text: text.String()})
	}
	// Preserve tool call blocks
	for _, block := range lastMsg.Content {
		if block.Type == types.BlockToolCall {
			accumulated = append(accumulated, block)
		}
	}
	lastMsg.Content = accumulated

	if lastUsage != nil {
		// Usage already set on lastMsg via the event
	}

	return lastMsg, nil
}

// OpenAI request/response types

type openAIRequest struct {
	Model           string                 `json:"model"`
	Input           []any                  `json:"input,omitempty"`
	Instructions    string                 `json:"instructions,omitempty"`
	Stream          bool                   `json:"stream"`
	Store           *bool                  `json:"store,omitempty"`
	Include         []string               `json:"include,omitempty"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
	Temperature     float64                `json:"temperature,omitempty"`
	Tools           []openAITool           `json:"tools,omitempty"`
	Reasoning       *openAIReasoningConfig `json:"reasoning,omitempty"`
}

type openAIReasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type openAIInputText struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIInputMessage struct {
	Role    string                   `json:"role"`
	Content []openAIInputTextContent `json:"content"`
}

type openAIInputTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIFunctionCallItem struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIFunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type openAITool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

type openAIDelta struct {
	Delta string `json:"delta"`
}

type openAIOutputItemData struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openAIOutputItem struct {
	Item openAIOutputItemData `json:"item"`
}

type openAIUsageResponse struct {
	Usage *openAIUsage `json:"usage,omitempty"`
}

type openAIUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func convertOpenAIUsage(u *openAIUsage) types.Usage {
	return types.Usage{
		Input:       u.InputTokens,
		Output:      u.OutputTokens,
		TotalTokens: u.TotalTokens,
	}
}

func messageToOpenAI(msg types.AgentMessage) []any {
	var items []any
	switch msg.Role {
	case types.RoleUser:
		text := extractText(msg)
		if text == "" {
			return nil
		}
		items = append(items, openAIInputMessage{
			Role: "user",
			Content: []openAIInputTextContent{
				{Type: "input_text", Text: text},
			},
		})
	case types.RoleAssistant:
		var hasText bool
		var text strings.Builder
		for _, block := range msg.Content {
			if block.Type == types.BlockText && block.Text != "" {
				hasText = true
				text.WriteString(block.Text)
			}
		}
		if hasText {
			items = append(items, openAIInputMessage{
				Role: "assistant",
				Content: []openAIInputTextContent{
					{Type: "output_text", Text: text.String()},
				},
			})
		}
		for _, block := range msg.Content {
			if block.Type == types.BlockToolCall && block.ToolCall != nil {
				tc := block.ToolCall
				callID := tc.ID
				if parts := strings.SplitN(tc.ID, "|", 2); len(parts) == 2 {
					callID = parts[0]
				}
				argsJSON, _ := json.Marshal(tc.Arguments)
				fcItem := openAIFunctionCallItem{
					Type:      "function_call",
					CallID:    callID,
					Name:      tc.Name,
					Arguments: string(argsJSON),
				}
				items = append(items, fcItem)
			}
		}
	case types.RoleToolResult:
		text := extractText(msg)
		callID := msg.ToolCallID
		if parts := strings.SplitN(msg.ToolCallID, "|", 2); len(parts) == 2 {
			callID = parts[0]
		}
		items = append(items, openAIFunctionCallOutput{
			Type:   "function_call_output",
			CallID: callID,
			Output: text,
		})
	default:
		return nil
	}
	return items
}

func extractText(msg types.AgentMessage) string {
	var text string
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			text += block.Text
		}
	}
	return text
}

// CodexModels returns the full set of models available through ChatGPT Plus/Pro
// subscription (OAuth). All models have $0 costs, reasoning enabled, and
// effort-based thinking level maps.
func CodexModels() []types.Model {
	effortThinking := map[string]string{
		"off": "none", "minimal": "minimal", "low": "low",
		"medium": "medium", "high": "high", "xhigh": "xhigh",
	}

	return []types.Model{
		{
			ID:               "gpt-5.5",
			Name:             "gpt-5.5",
			API:              "openai-completions",
			Reasoning:        true,
			InputTypes:       []string{"text"},
			Cost:             types.CostInfo{Input: 0, Output: 0, CacheRead: 0, CacheWrite: 0},
			ContextWindow:    400000,
			MaxTokens:        128000,
			ThinkingLevelMap: effortThinking,
		},
		{
			ID:               "gpt-5.4",
			Name:             "gpt-5.4",
			API:              "openai-completions",
			Reasoning:        true,
			InputTypes:       []string{"text"},
			Cost:             types.CostInfo{Input: 0, Output: 0, CacheRead: 0, CacheWrite: 0},
			ContextWindow:    272000,
			MaxTokens:        128000,
			ThinkingLevelMap: effortThinking,
		},
		{
			ID:               "gpt-5.4-mini",
			Name:             "gpt-5.4-mini",
			API:              "openai-completions",
			Reasoning:        true,
			InputTypes:       []string{"text"},
			Cost:             types.CostInfo{Input: 0, Output: 0, CacheRead: 0, CacheWrite: 0},
			ContextWindow:    128000,
			MaxTokens:        128000,
			ThinkingLevelMap: effortThinking,
		},
		{
			ID:               "gpt-5.3-codex",
			Name:             "gpt-5.3-codex",
			API:              "openai-completions",
			Reasoning:        true,
			InputTypes:       []string{"text"},
			Cost:             types.CostInfo{Input: 0, Output: 0, CacheRead: 0, CacheWrite: 0},
			ContextWindow:    272000,
			MaxTokens:        128000,
			ThinkingLevelMap: effortThinking,
		},
		{
			ID:               "gpt-5.3-codex-spark",
			Name:             "gpt-5.3-codex-spark",
			API:              "openai-completions",
			Reasoning:        true,
			InputTypes:       []string{"text"},
			Cost:             types.CostInfo{Input: 0, Output: 0, CacheRead: 0, CacheWrite: 0},
			ContextWindow:    128000,
			MaxTokens:        128000,
			ThinkingLevelMap: effortThinking,
		},
		{
			ID:               "gpt-5.2",
			Name:             "gpt-5.2",
			API:              "openai-completions",
			Reasoning:        true,
			InputTypes:       []string{"text"},
			Cost:             types.CostInfo{Input: 0, Output: 0, CacheRead: 0, CacheWrite: 0},
			ContextWindow:    128000,
			MaxTokens:        128000,
			ThinkingLevelMap: effortThinking,
		},
	}
}

// Ensure OpenAIProvider implements Provider
var _ Provider = (*OpenAIProvider)(nil)
