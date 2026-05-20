package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adam/tau/internal/types"
)

// OpenAIProvider implements the Provider interface for the OpenAI Responses API.
type OpenAIProvider struct {
	baseProvider
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

// Stream sends messages to OpenAI and returns a channel of streaming events.
func (p *OpenAIProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	if err := p.apiKeyOrErr(); err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	reqURL := baseURL + "/responses"

	body, err := p.buildStreamRequest(model, messages, tools, opts)
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	httpResp, err := p.httpClient.Do(&Request{
		Method:  "POST",
		URL:     reqURL,
		Headers: p.headers(model),
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

// Complete sends messages to OpenAI and returns the full response.
func (p *OpenAIProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	if err := p.apiKeyOrErr(); err != nil {
		return nil, err
	}

	events := p.Stream(ctx, model, messages, tools, opts)
	return p.collectFromStream(events)
}

func (p *OpenAIProvider) headers(model types.Model) map[string]string {
	h := map[string]string{
		"Authorization":  "Bearer " + p.apiKey,
		"Content-Type":   "application/json",
	}
	if len(model.Headers) > 0 {
		for k, v := range model.Headers {
			h[k] = v
		}
	}
	return h
}

func (p *OpenAIProvider) buildStreamRequest(model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) ([]byte, error) {
	req := openAIRequest{
		Model:    model.ID,
		Stream:   true,
		MaxOutputTokens: opts.MaxTokens,
		Temperature:     opts.Temperature,
	}

	// Only include session_usage for official OpenAI API (Zen doesn't support it)
	baseURL := model.BaseURL
	if baseURL == "" || strings.HasPrefix(baseURL, "https://api.openai.com") {
		req.Include = []string{"session_usage"}
	}

	// Add system message
	if opts.SystemPrompt != "" {
		req.Input = append(req.Input, openAIMessage{
			Role:    "system",
			Content: opts.SystemPrompt,
		})
	}

	// Add conversation messages
	for _, msg := range messages {
		oaiMsg := messageToOpenAI(msg)
		if oaiMsg != nil {
			req.Input = append(req.Input, *oaiMsg)
		}
	}

	// Add thinking if supported
	if model.Reasoning && opts.ThinkingLevel != "" && opts.ThinkingLevel != types.ThinkingOff {
		effort := model.MapThinkingLevel(opts.ThinkingLevel)
		req.Thinking = &openAIThinkingConfig{
			Type:   "enabled",
			Effort: effort,
		}
	}

	// Add tools
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
				var item openAIOutputItem
				if err := json.Unmarshal([]byte(data), &item); err == nil {
					if item.Type == "function_call" {
						currentToolCall = &types.ToolCallBlock{
							ID:        item.ID,
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
	Model             string         `json:"model"`
	Input             []openAIMessage `json:"input"`
	Stream            bool           `json:"stream"`
	Include           []string       `json:"include,omitempty"`
	MaxOutputTokens   int            `json:"max_output_tokens,omitempty"`
	Temperature       float64        `json:"temperature,omitempty"`
	Tools             []openAITool   `json:"tools,omitempty"`
	Thinking          *openAIThinkingConfig `json:"thinking,omitempty"`
}

type openAIThinkingConfig struct {
	Type   string `json:"type"`
	Effort string `json:"effort,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

type openAIOutputItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
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

func messageToOpenAI(msg types.AgentMessage) *openAIMessage {
	switch msg.Role {
	case types.RoleUser:
		text := extractText(msg)
		if text == "" {
			return nil
		}
		return &openAIMessage{Role: "user", Content: text}
	case types.RoleAssistant:
		text := extractText(msg)
		if text == "" {
			return nil
		}
		return &openAIMessage{Role: "assistant", Content: text}
	case types.RoleToolResult:
		text := extractText(msg)
		return &openAIMessage{Role: "user", Content: text}
	default:
		return nil
	}
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

// Ensure OpenAIProvider implements Provider
var _ Provider = (*OpenAIProvider)(nil)
