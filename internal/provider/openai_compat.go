package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/adam/tau/internal/types"
)

// OpenAICompatProvider implements the Provider interface for OpenAI-compatible APIs.
// This covers: OpenRouter, OpenCode Zen, OpenCode Go, Ollama, llama.cpp, LM Studio.
// These providers all implement the OpenAI Chat Completions API format.
type OpenAICompatProvider struct {
	baseProvider
	config OpenAICompatConfig
}

// OpenAICompatConfig holds configuration for OpenAI-compatible providers.
type OpenAICompatConfig struct {
	// BaseURL is the API base URL (e.g., "https://openrouter.ai/api/v1").
	BaseURL string
	// APIPath is the API path (default: "/chat/completions").
	APIPath string
	// ExtraHeaders are additional headers to send (e.g., routing headers for OpenRouter).
	ExtraHeaders map[string]string
	// ProviderName is the name for this provider instance.
	ProviderName string
	// StreamField is the SSE field name for the event type (default: "type" for OpenAI,
	// some providers may use different names).
	StreamField string
	// ThinkingLevel controls reasoning depth (OpenRouter: maps to reasoning.effort).
	ThinkingLevel types.ThinkingLevel
	// ProviderRouting is provider routing preferences (OpenRouter: passed as "provider" object).
	ProviderRouting map[string]any
}

// NewOpenAICompatProvider creates a new OpenAI-compatible provider.
func NewOpenAICompatProvider(apiKey string, config OpenAICompatConfig) *OpenAICompatProvider {
	if config.APIPath == "" {
		config.APIPath = "/chat/completions"
	}
	if config.ProviderName == "" {
		config.ProviderName = "openai-compat"
	}
	if config.StreamField == "" {
		config.StreamField = "type"
	}
	return &OpenAICompatProvider{
		baseProvider: baseProvider{
			name:                 config.ProviderName,
			httpClient:           &DefaultHTTPClient{},
			apiKey:               apiKey,
			skipAPIKeyValidation: true, // Ollama, LM Studio etc. don't require keys
		},
		config: config,
	}
}

// NewOpenAICompatProviderWithClient creates a new OpenAI-compatible provider with a custom HTTP client.
func NewOpenAICompatProviderWithClient(apiKey string, config OpenAICompatConfig, client HTTPClient) *OpenAICompatProvider {
	if config.APIPath == "" {
		config.APIPath = "/chat/completions"
	}
	if config.ProviderName == "" {
		config.ProviderName = "openai-compat"
	}
	if config.StreamField == "" {
		config.StreamField = "type"
	}
	return &OpenAICompatProvider{
		baseProvider: baseProvider{
			name:                 config.ProviderName,
			httpClient:           client,
			apiKey:               apiKey,
			skipAPIKeyValidation: true,
		},
		config: config,
	}
}

// Stream sends messages to the provider and returns a channel of streaming events.
func (p *OpenAICompatProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)

		if err := p.apiKeyOrErr(); err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}

		baseURL := p.config.BaseURL
		if baseURL == "" {
			baseURL = model.BaseURL
		}
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		reqURL := strings.TrimRight(baseURL, "/") + p.config.APIPath

		body, err := p.buildRequest(model, messages, tools, opts)
		if err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
		if err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}

		headers := p.headers()
		for k, v := range headers {
			httpReq.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(httpReq)
		if err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			apiErr := types.ClassifyAPIError(resp.StatusCode, bodyBytes)
			ch <- types.StreamEvent{
				Type:  types.EventError,
				Error: p.name + ": " + apiErr.UserMessage(),
			}
			return
		}

		p.parseStreamResponse(ctx, ch, resp.Body, model.ID, p.name)
	}()

	return ch
}

// Complete sends messages to the provider and returns the full response.
func (p *OpenAICompatProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	if err := p.apiKeyOrErr(); err != nil {
		return nil, err
	}

	events := p.Stream(ctx, model, messages, tools, opts)
	return p.collectFromStream(events)
}

func (p *OpenAICompatProvider) headers() map[string]string {
	h := map[string]string{
		"Content-Type":  "application/json",
	}
	if p.apiKey != "" {
		h["Authorization"] = "Bearer " + p.apiKey
	}
	// Add extra headers (e.g., OpenRouter routing headers)
	for k, v := range p.config.ExtraHeaders {
		h[k] = v
	}
	// Add model-specific headers
	if p.config.ExtraHeaders != nil {
		for k, v := range p.config.ExtraHeaders {
			h[k] = v
		}
	}
	return h
}

func (p *OpenAICompatProvider) buildRequest(model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) ([]byte, error) {
	// When max_tokens is not explicitly set, use the model's configured value
	// or fall back to 8192. Without this, Ollama's default is often too low
	// for thinking models (reasoning + response share the budget).
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		if model.MaxTokens > 0 {
			maxTokens = model.MaxTokens
		} else {
			maxTokens = 8192
		}
	}

	req := openAICompatRequest{
		Model:       model.ID,
		Stream:      true,
		MaxTokens:   maxTokens,
		Temperature: opts.Temperature,
	}

	// Add system message
	if opts.SystemPrompt != "" {
		req.Messages = append(req.Messages, openAICompatMessage{
			Role:    "system",
			Content: opts.SystemPrompt,
		})
	}

	// Add conversation messages
	isDeepSeek := strings.Contains(strings.ToLower(model.ID), "deepseek")
	for _, msg := range messages {
		cMsg := messageToOpenAICompat(msg)
		if cMsg != nil {
			// DeepSeek requires all assistant messages to have reasoning_content
			if isDeepSeek && msg.Role == types.RoleAssistant {
				thinking := extractThinking(msg)
				cMsg.ReasoningContent = &thinking
			}
			req.Messages = append(req.Messages, *cMsg)
		}
	}

	// Add tools
	if len(tools) > 0 {
		for _, t := range tools {
			req.Tools = append(req.Tools, openAICompatTool{
				Type: "function",
				Function: &openAICompatFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  toolDefToSchema(t),
				},
			})
		}
	}

	// OpenRouter: add reasoning effort from thinking level
	if p.config.ThinkingLevel != "" {
		req.Reasoning = &openRouterReasoning{
			Effort: thinkingLevelToEffort(p.config.ThinkingLevel),
		}
	}

	// OpenRouter: add provider routing preferences
	if p.config.ProviderRouting != nil {
		req.Provider = p.config.ProviderRouting
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// reasoningFieldOrder lists the SSE delta field names that may carry reasoning tokens.
// The first non-empty field wins — avoids duplication (e.g. chutes.ai returns both
// reasoning_content and reasoning with identical content).
var reasoningFieldOrder = []string{"reasoning_content", "reasoning", "reasoning_text"}

// textAccum accumulates streaming text deltas into a single ContentBlock.
// Emits EventTextStart on first delta, EventTextDelta on each delta,
// and EventTextEnd when finalized.
type textAccum struct {
	builder    strings.Builder
	started    bool
	blockIndex int // index in msg.Content, -1 if not active
}

func (a *textAccum) start(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage) {
	msg.Content = append(msg.Content, types.ContentBlock{
		Type: types.BlockText,
	})
	a.blockIndex = len(msg.Content) - 1
	a.started = true
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventTextStart,
		Message: msg,
	})
}

func (a *textAccum) write(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage, delta string) {
	if !a.started {
		a.start(ctx, ch, msg)
	}
	a.builder.WriteString(delta)
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventTextDelta,
		Delta:   delta,
		Message: msg,
	})
}

func (a *textAccum) finish(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage) {
	if !a.started {
		return
	}
	msg.Content[a.blockIndex].Text = a.builder.String()
	a.builder.Reset()
	a.started = false
	a.blockIndex = -1
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventTextEnd,
		Message: msg,
	})
}

// thinkingAccum accumulates streaming thinking/reasoning deltas into a single ContentBlock.
// Tracks signature (Anthropic) and fieldUsed (OpenAI-compat reasoning field switching).
type thinkingAccum struct {
	builder    strings.Builder
	started    bool
	blockIndex int // index in msg.Content, -1 if not active
	signature  string
	fieldUsed  string // which reasoning field produced this block (OpenAI-compat)
}

func (a *thinkingAccum) start(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage, field string) {
	msg.Content = append(msg.Content, types.ContentBlock{
		Type: types.BlockThinking,
	})
	a.blockIndex = len(msg.Content) - 1
	a.started = true
	a.fieldUsed = field
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventThinkingStart,
		Message: msg,
	})
}

func (a *thinkingAccum) write(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage, delta string) {
	if !a.started {
		a.start(ctx, ch, msg, "")
	}
	a.builder.WriteString(delta)
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventThinkingDelta,
		Delta:   delta,
		Message: msg,
	})
}

func (a *thinkingAccum) finish(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage) {
	if !a.started {
		return
	}
	text := a.builder.String()
	if a.signature != "" {
		text = text + "\x00" + a.signature
	}
	msg.Content[a.blockIndex].Text = text
	a.builder.Reset()
	a.started = false
	a.blockIndex = -1
	a.signature = ""
	a.fieldUsed = ""
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventThinkingEnd,
		Message: msg,
	})
}

// toolCallAccum accumulates streaming tool call data per index.
type toolCallAccum struct {
	ID           string
	Name         string
	ArgsJSON     strings.Builder
	startedEvent bool
}

func (p *OpenAICompatProvider) parseStreamResponse(ctx context.Context, ch chan<- types.StreamEvent, reader io.Reader, modelID string, providerName string) {
	sendEvent(ctx, ch, types.StreamEvent{Type: types.EventStart})

	msg := &types.AgentMessage{
		Role:  types.RoleAssistant,
		Model: modelID,
		API:   "openai-completions",
	}

	accumulators := make(map[int]*toolCallAccum)

	var textAcc textAccum
	var thinkAcc thinkingAccum

	// extractReasoning returns the first non-empty reasoning field and its name.
	extractReasoning := func(delta openAICompatDelta) (string, string) {
		for _, field := range reasoningFieldOrder {
			var val string
			switch field {
			case "reasoning_content":
				val = delta.ReasoningContent
			case "reasoning":
				val = delta.Reasoning
			case "reasoning_text":
				val = delta.ReasoningText
			}
			if val != "" {
				return field, val
			}
		}
		return "", ""
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	sseFields := make(map[string]string)
	flushSSEEvent := func() bool {
		if len(sseFields) == 0 {
			return true
		}
		defer func() { sseFields = make(map[string]string) }()

		event := sseFields["event"]
		data := sseFields["data"]

		if event == "error" {
			sendEvent(ctx, ch, types.StreamEvent{
				Type:  types.EventError,
				Error: providerName + " stream error: " + data,
			})
			return false
		}

		if data == "" {
			return true
		}

		if data == "[DONE]" {
			p.flushToolCalls(ctx, ch, msg, accumulators)
			textAcc.finish(ctx, ch, msg)
			thinkAcc.finish(ctx, ch, msg)
			sendEvent(ctx, ch, types.StreamEvent{
				Type:    types.EventDone,
				Message: msg,
			})
			return false
		}

		var resp openAICompatStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			if strings.Contains(data, "error") {
				sendEvent(ctx, ch, types.StreamEvent{
					Type:  types.EventError,
					Error: fmt.Sprintf("%s error: %s", providerName, data),
				})
				return false
			}
			return true
		}

		if len(resp.Choices) == 0 {
			if strings.Contains(data, "error") {
				sendEvent(ctx, ch, types.StreamEvent{
					Type:  types.EventError,
					Error: fmt.Sprintf("%s error: %s", providerName, data),
				})
				return false
			}
			return true
		}

		choice := resp.Choices[0]

		// Reasoning delta
		reasoningField, reasoningText := extractReasoning(choice.Delta)
		if reasoningText != "" {
			textAcc.finish(ctx, ch, msg)
			if thinkAcc.fieldUsed != "" && reasoningField != thinkAcc.fieldUsed {
				thinkAcc.finish(ctx, ch, msg)
			}
			if !thinkAcc.started {
				thinkAcc.start(ctx, ch, msg, reasoningField)
			}
			thinkAcc.write(ctx, ch, msg, reasoningText)
		}

		// Text delta
		if choice.Delta.Content != "" {
			thinkAcc.finish(ctx, ch, msg)
			textAcc.write(ctx, ch, msg, choice.Delta.Content)
		}

		// Tool call deltas
		if len(choice.Delta.ToolCalls) > 0 {
			thinkAcc.finish(ctx, ch, msg)
			textAcc.finish(ctx, ch, msg)
			for _, tc := range choice.Delta.ToolCalls {
				accum, exists := accumulators[tc.Index]
				if !exists {
					accum = &toolCallAccum{}
					accumulators[tc.Index] = accum
				}
				if tc.ID != "" {
					accum.ID = tc.ID
				}
				if tc.Function.Name != "" {
					accum.Name = tc.Function.Name
					if !accum.startedEvent {
						sendEvent(ctx, ch, types.StreamEvent{
							Type:  types.EventToolCallStart,
							Delta: tc.Function.Name,
						})
						accum.startedEvent = true
					}
				}
				if tc.Function.Arguments != "" {
					accum.ArgsJSON.WriteString(tc.Function.Arguments)
				}
			}
		}

		// Finish reason
		if choice.FinishReason != "" {
			p.flushToolCalls(ctx, ch, msg, accumulators)
			textAcc.finish(ctx, ch, msg)
			thinkAcc.finish(ctx, ch, msg)

			usage := types.Usage{}
			if resp.Usage != nil {
				usage.Input = resp.Usage.PromptTokens
				usage.Output = resp.Usage.CompletionTokens
				usage.TotalTokens = resp.Usage.TotalTokens
			}
			sendEvent(ctx, ch, types.StreamEvent{
				Type:    types.EventDone,
				Message: msg,
				Usage:   &usage,
			})
			return false
		}

		return true
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			p.flushToolCalls(ctx, ch, msg, accumulators)
			textAcc.finish(ctx, ch, msg)
			thinkAcc.finish(ctx, ch, msg)
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if !flushSSEEvent() {
				return
			}
			continue
		}

		if colon := strings.Index(line, ":"); colon != -1 {
			key := strings.TrimSpace(line[:colon])
			value := strings.TrimSpace(line[colon+1:])
			if key == "data" {
				if existing, ok := sseFields["data"]; ok {
					sseFields["data"] = existing + "\n" + value
				} else {
					sseFields["data"] = value
				}
			} else {
				sseFields[key] = value
			}
		}
	}

	// Flush last event if any
	flushSSEEvent()

	// SSE ended without finish_reason — flush whatever we have.
	p.flushToolCalls(ctx, ch, msg, accumulators)
	textAcc.finish(ctx, ch, msg)
	thinkAcc.finish(ctx, ch, msg)
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventDone,
		Message: msg,
	})
}

// flushToolCalls parses accumulated tool call arguments, creates ToolCallBlocks,
// and emits EventToolCallEnd for each. Keys are sorted for deterministic order.
func (p *OpenAICompatProvider) flushToolCalls(
	ctx context.Context, ch chan<- types.StreamEvent,
	msg *types.AgentMessage, accumulators map[int]*toolCallAccum,
) {
	if len(accumulators) == 0 {
		return
	}

	// Sort keys for deterministic tool call ordering
	indexes := make([]int, 0, len(accumulators))
	for k := range accumulators {
		indexes = append(indexes, k)
	}
	slices.Sort(indexes)

	for _, idx := range indexes {
		accum := accumulators[idx]
		args, _ := parseArgs(accum.ArgsJSON.String())

		id := accum.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", idx)
		}

		msg.Content = append(msg.Content, types.ContentBlock{
			Type: types.BlockToolCall,
			ToolCall: &types.ToolCallBlock{
				ID:        id,
				Name:      accum.Name,
				Arguments: args,
			},
		})

		sendEvent(ctx, ch, types.StreamEvent{
			Type:    types.EventToolCallEnd,
			Message: msg,
		})
	}
}

func (p *OpenAICompatProvider) collectFromStream(ch <-chan types.StreamEvent) (*types.AgentMessage, error) {
	var lastMsg *types.AgentMessage

	for event := range ch {
		switch event.Type {
		case types.EventError:
			return nil, fmt.Errorf("stream error: %s", event.Error)
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
		return nil, fmt.Errorf("no response received")
	}

	return lastMsg, nil
}

// repairJSON escapes raw control characters and fixes bad backslash escapes
// in JSON strings. Matches PI's repairJson behavior.
func repairJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/16) // slight growth for escaping

	inString := false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if !inString {
			b.WriteRune(c)
			if c == '"' {
				inString = true
			}
			continue
		}

		if c == '"' {
			b.WriteRune(c)
			inString = false
			continue
		}

		if c == '\\' {
			// Check if there's a next character
			if i+1 >= len(runes) {
				// Trailing backslash — escape it
				b.WriteString("\\\\")
				continue
			}
			next := runes[i+1]
			// Valid JSON escape sequences
			if isValidJSONEscape(next) {
				b.WriteRune(c)
				b.WriteRune(next)
				if next == 'u' {
					// \uXXXX — consume 4 hex digits
					for j := 0; j < 4 && i+2+j < len(runes); j++ {
						b.WriteRune(runes[i+2+j])
					}
					i += 4
				}
				i++
				continue
			}
			// Invalid escape — double the backslash
			b.WriteString("\\\\")
			continue
		}

		// Control characters inside string — escape them
		if c >= 0x00 && c <= 0x1f {
			b.WriteString(escapeControlChar(c))
			continue
		}

		b.WriteRune(c)
	}
	return b.String()
}

func isValidJSONEscape(c rune) bool {
	switch c {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	}
	return false
}

func escapeControlChar(c rune) string {
	switch c {
	case '\b':
		return "\\b"
	case '\f':
		return "\\f"
	case '\n':
		return "\\n"
	case '\r':
		return "\\r"
	case '\t':
		return "\\t"
	default:
		return fmt.Sprintf("\\u%04x", c)
	}
}

// parseArgs parses accumulated JSON string arguments into a map.
// Uses a fallback chain matching PI's parseStreamingJson strategy:
// 1. Standard json.Unmarshal
// 2. repairJSON + json.Unmarshal
// 3. Synthetic _parse_error fallback
func parseArgs(s string) (map[string]any, error) {
	if s == "" {
		return make(map[string]any), nil
	}

	var result map[string]any

	// Try standard parse
	if err := json.Unmarshal([]byte(s), &result); err == nil {
		return result, nil
	}

	// Try repair + parse
	repaired := repairJSON(s)
	if repaired != s {
		if err := json.Unmarshal([]byte(repaired), &result); err == nil {
			return result, nil
		}
	}

	// Last resort: return synthetic error
	return map[string]any{"_parse_error": s}, nil
}

// repairAndParse attempts to parse JSON, trying repairJSON if standard parse fails.
// Used by tests.
func repairAndParse(s string, v any) error {
	if err := json.Unmarshal([]byte(s), v); err == nil {
		return nil
	}
	repaired := repairJSON(s)
	return json.Unmarshal([]byte(repaired), v)
}

// OpenAI-compatible request/response types (Chat Completions API)

type openAICompatRequest struct {
	Model           string                  `json:"model"`
	Messages        []openAICompatMessage   `json:"messages"`
	Stream          bool                    `json:"stream"`
	MaxTokens       int                     `json:"max_tokens,omitempty"`
	Temperature     float64                 `json:"temperature,omitempty"`
	Tools           []openAICompatTool      `json:"tools,omitempty"`
	Reasoning       *openRouterReasoning    `json:"reasoning,omitempty"`
	Provider        map[string]any          `json:"provider,omitempty"`
}

// openRouterReasoning represents OpenRouter's reasoning effort parameter.
type openRouterReasoning struct {
	Effort string `json:"effort"`
}

type openAICompatMessage struct {
	Role             string                      `json:"role"`
	Content          string                      `json:"content"`
	ToolCalls        []openAICompatOutgoingTool  `json:"tool_calls,omitempty"`
	ToolCallID       string                      `json:"tool_call_id,omitempty"`
	ReasoningContent *string                     `json:"reasoning_content,omitempty"`
}

type openAICompatOutgoingTool struct {
	ID       string                        `json:"id"`
	Type     string                        `json:"type"`
	Function *openAICompatOutgoingFunction `json:"function"`
}

type openAICompatOutgoingFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAICompatTool struct {
	Type     string                   `json:"type"`
	Function *openAICompatFunction    `json:"function"`
}

type openAICompatFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

type openAICompatStreamResponse struct {
	Choices []openAICompatChoice  `json:"choices"`
	Usage   *openAICompatUsage    `json:"usage,omitempty"`
}

type openAICompatChoice struct {
	Delta        openAICompatDelta `json:"delta"`
	FinishReason string            `json:"finish_reason"`
	Index        int               `json:"index"`
}

type openAICompatDelta struct {
	Content        string                      `json:"content"`
	Reasoning      string                      `json:"reasoning"`
	ReasoningText  string                      `json:"reasoning_text"`
	ReasoningContent string                    `json:"reasoning_content"`
	ToolCalls      []openAICompatToolCallDelta `json:"tool_calls,omitempty"`
}

type openAICompatToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type openAICompatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func messageToOpenAICompat(msg types.AgentMessage) *openAICompatMessage {
	switch msg.Role {
	case types.RoleUser:
		text := extractText(msg)
		if text == "" {
			return nil
		}
		return &openAICompatMessage{Role: "user", Content: text}
	case types.RoleAssistant:
		// Extract tool calls from the message
		var toolCalls []openAICompatOutgoingTool
		for _, block := range msg.Content {
			if block.Type == types.BlockToolCall && block.ToolCall != nil {
				argsJSON, _ := json.Marshal(block.ToolCall.Arguments)
				toolCalls = append(toolCalls, openAICompatOutgoingTool{
					ID:   block.ToolCall.ID,
					Type: "function",
					Function: &openAICompatOutgoingFunction{
						Name:      block.ToolCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
		// Extract text content (may be empty if model only called tools)
		text := extractText(msg)
		// If message has tool calls, we must include it even if text is empty
		if len(toolCalls) > 0 {
			return &openAICompatMessage{
				Role:      "assistant",
				Content:   text, // can be empty string — that's OK with tool_calls
				ToolCalls: toolCalls,
			}
		}
		if text == "" {
			return nil
		}
		return &openAICompatMessage{Role: "assistant", Content: text}
	case types.RoleToolResult:
		text := extractText(msg)
		return &openAICompatMessage{
			Role:       "tool",
			Content:    text,
			ToolCallID: msg.ToolCallID,
		}
	default:
		return nil
	}
}

// extractThinking extracts thinking/reasoning text from an assistant message.
func extractThinking(msg types.AgentMessage) string {
	var text string
	for _, block := range msg.Content {
		if block.Type == types.BlockThinking {
			text += block.Text
		}
	}
	return text
}

// thinkingLevelToEffort maps tau ThinkingLevel to OpenRouter reasoning effort.
func thinkingLevelToEffort(level types.ThinkingLevel) string {
	switch level {
	case types.ThinkingOff, "":
		return "none"
	case types.ThinkingMinimal, types.ThinkingLow:
		return "low"
	case types.ThinkingMedium:
		return "medium"
	case types.ThinkingHigh:
		return "high"
	case types.ThinkingXHigh:
		return "xhigh"
	default:
		return "medium"
	}
}

// Ensure OpenAICompatProvider implements Provider
var _ Provider = (*OpenAICompatProvider)(nil)
