package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/adam/tau/internal/types"
)

// AnthropicProvider implements the Provider interface for the Anthropic Messages API.
type AnthropicProvider struct {
	baseProvider
	apiVersion string
}

// NewAnthropicProvider creates a new Anthropic provider with the given API key.
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		baseProvider: baseProvider{
			name:       "anthropic",
			httpClient: &DefaultHTTPClient{},
			apiKey:     apiKey,
		},
		apiVersion: "2023-06-01",
	}
}

// NewAnthropicProviderWithClient creates a new Anthropic provider with a custom HTTP client.
func NewAnthropicProviderWithClient(apiKey string, client HTTPClient) *AnthropicProvider {
	return &AnthropicProvider{
		baseProvider: baseProvider{
			name:       "anthropic",
			httpClient: client,
			apiKey:     apiKey,
		},
		apiVersion: "2023-06-01",
	}
}

// Stream sends messages to Anthropic and returns a channel of streaming events.
func (p *AnthropicProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	if err := p.apiKeyOrErr(); err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	reqURL := baseURL + "/messages"

	body, err := p.buildRequest(model, messages, tools, opts)
	if err != nil {
		return streamToChannel(ctx, []types.StreamEvent{{Type: types.EventError, Error: err.Error()}})
	}

	headers := p.headers(model)
	// Extended thinking requires the beta header. Anthropic rejects thinking
	// requests without it (400 Bad Request).
	if model.Reasoning && opts.ThinkingLevel != "" && opts.ThinkingLevel != types.ThinkingOff {
		headers["anthropic-beta"] = "extended-thinking-2025-05-01"
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

// Complete sends messages to Anthropic and returns the full response.
func (p *AnthropicProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	if err := p.apiKeyOrErr(); err != nil {
		return nil, err
	}

	events := p.Stream(ctx, model, messages, tools, opts)
	return p.collectFromStream(events)
}

func (p *AnthropicProvider) headers(model types.Model) map[string]string {
	h := map[string]string{
		"x-api-key":         p.apiKey,
		"anthropic-version": p.apiVersion,
		"Content-Type":      "application/json",
	}
	if len(model.Headers) > 0 {
		for k, v := range model.Headers {
			h[k] = v
		}
	}
	return h
}

func (p *AnthropicProvider) buildRequest(model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) ([]byte, error) {
	req := anthropicRequest{
		Model:       model.ID,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		Stream:      true,
	}

	// Extract system prompt from options
	if opts.SystemPrompt != "" {
		req.System = opts.SystemPrompt
	}

	// Build messages from conversation history
	for _, msg := range messages {
		aMsg := messageToAnthropic(msg)
		if aMsg != nil {
			req.Messages = append(req.Messages, *aMsg)
		}
	}

	// Add thinking if supported
	if model.Reasoning && opts.ThinkingLevel != "" && opts.ThinkingLevel != types.ThinkingOff {
		// Check if model uses adaptive thinking (effort-based) vs budget-based
		effort := model.MapThinkingLevel(opts.ThinkingLevel)
		if isAdaptiveThinkingEffort(effort) {
			// Adaptive thinking: model decides when/how much to think
			req.Thinking = &anthropicThinking{
				Type: "adaptive",
			}
			req.OutputConfig = &anthropicOutputConfig{
				Effort: effort,
			}
		} else {
			// Budget-based thinking: fixed token budget
			req.Thinking = &anthropicThinking{
				Type:         "enabled",
				BudgetTokens: thinkingBudget(opts.ThinkingLevel),
			}
		}
	}

	// Add tools
	if len(tools) > 0 {
		for _, t := range tools {
			req.Tools = append(req.Tools, anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: toolDefToSchema(t),
			})
		}
	}

	if req.MaxTokens == 0 {
		req.MaxTokens = model.MaxTokens
	}

	return json.Marshal(req)
}

func (p *AnthropicProvider) parseStreamResponse(ctx context.Context, body []byte, modelID string) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)

		msg := &types.AgentMessage{
			Role:  types.RoleAssistant,
			Model: modelID,
			API:   "anthropic-messages",
		}

		var textAcc textAccum
		var thinkAcc thinkingAccum

		reader := NewSSELineReader(body)
		for fields, ok := reader.ReadNext(); ok; fields, ok = reader.ReadNext() {
			select {
			case <-ctx.Done():
				textAcc.finish(ctx, ch, msg)
				thinkAcc.finish(ctx, ch, msg)
				sendEvent(ctx, ch, types.StreamEvent{
					Type:    types.EventDone,
					Message: msg,
				})
				return
			default:
			}

			eventType := fields["event"]
			if eventType == "" {
				eventType = fields["type"]
			}
			data := fields["data"]
			if data == "" {
				continue
			}

			switch eventType {
			case "message_start":
				sendEvent(ctx, ch, types.StreamEvent{
					Type:    types.EventStart,
					Message: msg,
				})

			case "content_block_start":
				var blockStart anthropicBlockStart
				if err := json.Unmarshal([]byte(data), &blockStart); err == nil {
					if blockStart.ContentBlock.Type == "thinking" {
						thinkAcc.start(ctx, ch, msg, "")
					} else if blockStart.ContentBlock.Type == "tool_use" {
						thinkAcc.finish(ctx, ch, msg)
						textAcc.finish(ctx, ch, msg)
						sendEvent(ctx, ch, types.StreamEvent{
							Type:  types.EventToolCallStart,
							Delta: blockStart.ContentBlock.Name,
						})
					}
				}

			case "content_block_delta":
				var delta anthropicContentDelta
				if err := json.Unmarshal([]byte(data), &delta); err == nil {
					switch delta.Delta.Type {
					case "text_delta":
						thinkAcc.finish(ctx, ch, msg)
						textAcc.write(ctx, ch, msg, delta.Delta.Text)
					case "thinking_delta":
						thinkAcc.write(ctx, ch, msg, delta.Delta.Thinking)
					case "signature_delta":
						thinkAcc.signature = delta.Delta.Signature
					case "input_json_delta":
						// Tool call arguments accumulating
					}
				}

			case "content_block_stop":
				thinkAcc.finish(ctx, ch, msg)
				textAcc.finish(ctx, ch, msg)

			case "message_delta":
				var msgDelta anthropicMessageDelta
				if err := json.Unmarshal([]byte(data), &msgDelta); err == nil {
					usage := types.Usage{}
					hasUsage := false
					if msgDelta.Usage != nil {
						hasUsage = true
						usage.Output = msgDelta.Usage.OutputTokens
						usage.TotalTokens = msgDelta.Usage.OutputTokens
						if msgDelta.Usage.CacheCreationInputTokens > 0 {
							usage.CacheWrite = msgDelta.Usage.CacheCreationInputTokens
						}
						if msgDelta.Usage.CacheReadInputTokens > 0 {
							usage.CacheRead = msgDelta.Usage.CacheReadInputTokens
						}
					}
					sendEvent(ctx, ch, types.StreamEvent{
						Type:    types.EventDone,
						Message: msg,
						Usage:   func() *types.Usage { if hasUsage { return &usage }; return nil }(),
					})
				}

			case "message_stop":
				// Handled by message_delta for usage

			case "error":
				sendEvent(ctx, ch, types.StreamEvent{
					Type:  types.EventError,
					Error: "Anthropic stream error: " + data,
				})
			}
		}

		// SSE ended without message_delta — flush accumulators and send done.
		textAcc.finish(ctx, ch, msg)
		thinkAcc.finish(ctx, ch, msg)
		sendEvent(ctx, ch, types.StreamEvent{
			Type:    types.EventDone,
			Message: msg,
		})
	}()

	return ch
}

func (p *AnthropicProvider) collectFromStream(ch <-chan types.StreamEvent) (*types.AgentMessage, error) {
	var lastMsg *types.AgentMessage

	for event := range ch {
		switch event.Type {
		case types.EventError:
			return nil, fmt.Errorf("Anthropic stream error: %s", event.Error)
		case types.EventDone:
			if event.Message != nil {
				lastMsg = event.Message
			}
		case types.EventTextStart, types.EventTextDelta, types.EventTextEnd:
			if event.Message != nil {
				lastMsg = event.Message
			}
		case types.EventThinkingDelta:
			if event.Message != nil {
				lastMsg = event.Message
			}
		}
	}

	if lastMsg == nil {
		return nil, fmt.Errorf("no response from Anthropic")
	}

	return lastMsg, nil
}

// Anthropic request/response types

type anthropicRequest struct {
	Model          string                    `json:"model"`
	MaxTokens      int                       `json:"max_tokens"`
	Temperature    float64                   `json:"temperature,omitempty"`
	Stream         bool                      `json:"stream"`
	System         string                    `json:"system,omitempty"`
	Messages       []anthropicMessage        `json:"messages"`
	Tools          []anthropicTool           `json:"tools,omitempty"`
	Thinking       *anthropicThinking        `json:"thinking,omitempty"`
	OutputConfig   *anthropicOutputConfig    `json:"output_config,omitempty"`
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Input     *any   `json:"input,omitempty"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type anthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type anthropicBlockStart struct {
	ContentBlock struct {
		Type string `json:"type"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

type anthropicContentDelta struct {
	Index int `json:"index,omitempty"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Thinking string `json:"thinking,omitempty"`
		Signature string `json:"signature,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthropicMessageDelta struct {
	Usage *struct {
		OutputTokens              int `json:"output_tokens"`
		CacheCreationInputTokens  int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens      int `json:"cache_read_input_tokens"`
	} `json:"usage,omitempty"`
}

func messageToAnthropic(msg types.AgentMessage) *anthropicMessage {
	switch msg.Role {
	case types.RoleUser:
		var blocks []anthropicContentBlock
		for _, block := range msg.Content {
			switch block.Type {
			case types.BlockText:
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: block.Text})
			case types.BlockToolCall:
				if block.ToolCall != nil {
					input := any(block.ToolCall.Arguments)
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						Text:  "",
						Input: &input,
					})
				}
			}
		}
		if len(blocks) == 0 {
			return nil
		}
		return &anthropicMessage{Role: "user", Content: blocks}
	case types.RoleAssistant:
		var blocks []anthropicContentBlock
		for _, block := range msg.Content {
			switch block.Type {
			case types.BlockText:
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: block.Text})
			case types.BlockThinking:
				// Anthropic round-trip: split thinking text by NUL separator to extract
				// thinking content and signature. Format: "<thinking>\x00<signature>"
				thinkingText := block.Text
				var signature string
				for j := 0; j < len(thinkingText); j++ {
					if thinkingText[j] == '\x00' {
						signature = thinkingText[j+1:]
						thinkingText = thinkingText[:j]
						break
					}
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:      "thinking",
					Thinking:  thinkingText,
					Signature: signature,
				})
			}
		}
		if len(blocks) == 0 {
			return nil
		}
		return &anthropicMessage{Role: "assistant", Content: blocks}
	case types.RoleToolResult:
		var blocks []anthropicContentBlock
		for _, block := range msg.Content {
			if block.Type == types.BlockText {
				blocks = append(blocks, anthropicContentBlock{Type: "tool_result", Text: block.Text})
			}
		}
		if len(blocks) == 0 {
			return nil
		}
		return &anthropicMessage{Role: "user", Content: blocks}
	default:
		return nil
	}
}

func thinkingBudget(level types.ThinkingLevel) int {
	switch level {
	case types.ThinkingMinimal:
		return 1024
	case types.ThinkingLow:
		return 2048
	case types.ThinkingMedium:
		return 4096
	case types.ThinkingHigh:
		return 8192
	case types.ThinkingXHigh:
		return 16384
	default:
		return 4096
	}
}

// isAdaptiveThinkingEffort checks if a mapped thinking level is an Anthropic
// adaptive thinking effort value (vs a numeric budget token value).
func isAdaptiveThinkingEffort(mapped string) bool {
	switch mapped {
	case "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

// Ensure AnthropicProvider implements Provider
var _ Provider = (*AnthropicProvider)(nil)
