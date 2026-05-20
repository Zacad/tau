package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/adam/tau/internal/types"
)

// OllamaProvider implements the Provider interface using Ollama's native
// /api/chat endpoint, which properly separates thinking from response content.
type OllamaProvider struct {
	baseProvider
	baseURL string
}

// NewOllamaProvider creates a new Ollama provider using the native API.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaProvider{
		baseProvider: baseProvider{
			name:                 "ollama",
			httpClient:           &DefaultHTTPClient{},
			skipAPIKeyValidation: true,
		},
		baseURL: baseURL,
	}
}

// ollamaChatRequest is the request body for Ollama's /api/chat endpoint.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Tools    []ollamaTool        `json:"tools,omitempty"`
	Stream   bool                `json:"stream"`
	Options  *ollamaOptions      `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content"`
	Thinking   string               `json:"thinking,omitempty"`
	ToolCalls  []ollamaToolCallResp `json:"tool_calls,omitempty"`
	ToolName   string               `json:"tool_name,omitempty"`
}

type ollamaTool struct {
	Type     string        `json:"type"`
	Function ollamaFuncDef `json:"function"`
}

type ollamaFuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ollamaToolCallResp struct {
	Function ollamaToolCallFunc `json:"function"`
}

type ollamaToolCallFunc struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaOptions struct {
	NumPredict   int     `json:"num_predict,omitempty"`
	Temperature  float64 `json:"temperature,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

// ollamaChatResponse is a single chunk from Ollama's streaming /api/chat response.
type ollamaChatResponse struct {
	Model      string `json:"model"`
	CreatedAt  string `json:"created_at"`
	Message    struct {
		Role      string               `json:"role"`
		Content   string               `json:"content"`
		Thinking  string               `json:"thinking"`
		ToolCalls []ollamaToolCallResp `json:"tool_calls"`
	} `json:"message"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason"`
}

// accumulatingCall tracks a tool call whose arguments may arrive incrementally
// across multiple streaming chunks.
type accumulatingCall struct {
	name    string
	args    map[string]any
}

// Stream sends messages to Ollama and returns a channel of streaming events.
// It reads the response body incrementally so events are emitted as chunks arrive.
func (p *OllamaProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)

		reqBody, err := p.buildRequest(model, messages, tools, opts)
		if err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}

		slog.Debug("ollama: request built", "model", model.ID, "body_len", len(reqBody))

		reqURL := strings.TrimRight(p.baseURL, "/") + "/api/chat"

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBody))
		if err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		slog.Debug("ollama: sending request", "url", reqURL, "model", model.ID)

		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Do(httpReq)
		if err != nil {
			ch <- types.StreamEvent{Type: types.EventError, Error: err.Error()}
			return
		}
		defer resp.Body.Close()

		slog.Debug("ollama: response received", "status", resp.StatusCode)

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			apiErr := types.ClassifyAPIError(resp.StatusCode, body)
			ch <- types.StreamEvent{
				Type:  types.EventError,
				Error: apiErr.UserMessage(),
			}
			return
		}

		p.parseStreamResponse(ctx, ch, resp.Body, model.ID)
	}()

	return ch
}

// Complete sends messages to Ollama and returns the full response.
func (p *OllamaProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	events := p.Stream(ctx, model, messages, tools, opts)
	return p.collectFromStream(events)
}

func (p *OllamaProvider) buildRequest(model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) ([]byte, error) {
	req := ollamaChatRequest{
		Model:  model.ID,
		Stream: true,
	}

	numPredict := opts.MaxTokens
	if numPredict == 0 {
		numPredict = 32768
	}

	temp := opts.Temperature
	if temp == 0 {
		temp = 0.7
	}

	req.Options = &ollamaOptions{
		NumPredict:  numPredict,
		Temperature: temp,
	}

	slog.Debug("ollama: buildRequest", "model", model.ID, "thinking_level", opts.ThinkingLevel, "reasoning", model.Reasoning, "num_predict", numPredict)

	// Add thinking level for reasoning models
	if model.Reasoning && opts.ThinkingLevel != "" && opts.ThinkingLevel != types.ThinkingOff {
		req.Options.ThinkingLevel = model.MapThinkingLevel(opts.ThinkingLevel)
		slog.Debug("ollama: thinking level set", "model", model.ID, "level", req.Options.ThinkingLevel)
	}

	if opts.SystemPrompt != "" {
		req.Messages = append(req.Messages, ollamaChatMessage{
			Role:    "system",
			Content: opts.SystemPrompt,
		})
	}

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			text := extractText(msg)
			if text != "" {
				req.Messages = append(req.Messages, ollamaChatMessage{
					Role:    "user",
					Content: text,
				})
			}
	case types.RoleAssistant:
		text := extractText(msg)

		var toolCalls []ollamaToolCallResp
		for _, block := range msg.Content {
			if block.Type == types.BlockToolCall && block.ToolCall != nil {
				toolCalls = append(toolCalls, ollamaToolCallResp{
					Function: ollamaToolCallFunc{
						Name:      block.ToolCall.Name,
						Arguments: block.ToolCall.Arguments,
					},
				})
			}
		}

		ollamaMsg := ollamaChatMessage{
			Role:      "assistant",
			Content:   text,
			ToolCalls: toolCalls,
		}

		// Include thinking content only for reasoning models
		if model.Reasoning {
			ollamaMsg.Thinking = extractThinking(msg)
		}

		req.Messages = append(req.Messages, ollamaMsg)
		case types.RoleToolResult:
			text := extractText(msg)
			toolName := findToolNameForCallID(messages, msg.ToolCallID)
			req.Messages = append(req.Messages, ollamaChatMessage{
				Role:     "tool",
				Content:  text,
				ToolName: toolName,
			})
		}
	}

	if len(tools) > 0 {
		for _, t := range tools {
			req.Tools = append(req.Tools, ollamaTool{
				Type: "function",
				Function: ollamaFuncDef{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			})
		}
	}

	return json.Marshal(req)
}

func (p *OllamaProvider) parseStreamResponse(ctx context.Context, ch chan<- types.StreamEvent, reader io.Reader, modelID string) {
	sendEvent(ctx, ch, types.StreamEvent{Type: types.EventStart})

	msg := &types.AgentMessage{
		Role:  types.RoleAssistant,
		Model: modelID,
		API:   "ollama-chat",
	}

	thinkingActive := false
	var thinkingAccum strings.Builder
	var textAccum strings.Builder

	// Tool call argument accumulation across streaming chunks.
	// Ollama may send tool call arguments incrementally (like OpenAI).
	toolCallsByIndex := make(map[int]*accumulatingCall)

	scanner := bufio.NewScanner(reader)
	// Increase buffer size for potentially large JSON lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			p.flushToolCalls(ctx, ch, msg, toolCallsByIndex)
			p.flushAccumulated(ctx, ch, msg, &thinkingAccum, &textAccum, thinkingActive)
			return
		default:
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var resp ollamaChatResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			slog.Debug("ollama: failed to parse chunk", "error", err, "line", string(line[:min(len(line), 100)]))
			continue
		}

		// Thinking delta
		if resp.Message.Thinking != "" {
			if !thinkingActive {
				thinkingActive = true
				sendEvent(ctx, ch, types.StreamEvent{
					Type:    types.EventThinkingStart,
					Message: msg,
				})
			}
			thinkingAccum.WriteString(resp.Message.Thinking)
			msg.Content = append(msg.Content, types.ContentBlock{
				Type: types.BlockThinking,
				Text: resp.Message.Thinking,
			})
			sendEvent(ctx, ch, types.StreamEvent{
				Type:    types.EventThinkingDelta,
				Delta:   resp.Message.Thinking,
				Message: msg,
			})
			// Close thinking when content appears
			if resp.Message.Content != "" {
				thinkingActive = false
			}
		}

		// Text (content) delta
		if resp.Message.Content != "" {
			if thinkingActive {
				thinkingActive = false
				sendEvent(ctx, ch, types.StreamEvent{
					Type:    types.EventThinkingEnd,
					Message: msg,
				})
			}
			textAccum.WriteString(resp.Message.Content)
			msg.Content = append(msg.Content, types.ContentBlock{
				Type: types.BlockText,
				Text: resp.Message.Content,
			})
			sendEvent(ctx, ch, types.StreamEvent{
				Type:    types.EventTextDelta,
				Delta:   resp.Message.Content,
				Message: msg,
			})
		}

		// Tool calls — arguments may arrive incrementally across chunks.
		// Accumulate by index, emit events only once per tool call.
		if len(resp.Message.ToolCalls) > 0 {
			if thinkingActive {
				thinkingActive = false
				sendEvent(ctx, ch, types.StreamEvent{
					Type:    types.EventThinkingEnd,
					Message: msg,
				})
			}
			for i, tc := range resp.Message.ToolCalls {
				ac, exists := toolCallsByIndex[i]
				if !exists {
					ac = &accumulatingCall{name: tc.Function.Name, args: make(map[string]any)}
					toolCallsByIndex[i] = ac
				}
				// Merge arguments: new values override existing ones
				for k, v := range tc.Function.Arguments {
					ac.args[k] = v
				}
				// Update name if not yet set (first chunk might only have partial info)
				if ac.name == "" && tc.Function.Name != "" {
					ac.name = tc.Function.Name
				}
			}
		}

		if resp.Done {
			slog.Debug("ollama stream done", "reason", resp.DoneReason,
				"thinking_len", thinkingAccum.Len(),
				"text_len", textAccum.Len(),
				"tool_calls", len(resp.Message.ToolCalls))

			// Emit accumulated tool calls
			p.flushToolCalls(ctx, ch, msg, toolCallsByIndex)

			p.flushAccumulated(ctx, ch, msg, &thinkingAccum, &textAccum, thinkingActive)

			sendEvent(ctx, ch, types.StreamEvent{
				Type:    types.EventDone,
				Message: msg,
			})
			return
		}
	}

	// Scanner ended without done=true — flush what we have
	p.flushToolCalls(ctx, ch, msg, toolCallsByIndex)
	p.flushAccumulated(ctx, ch, msg, &thinkingAccum, &textAccum, thinkingActive)
	sendEvent(ctx, ch, types.StreamEvent{
		Type:    types.EventDone,
		Message: msg,
	})
}

// flushToolCalls emits accumulated tool calls as events and adds them to the message.
func (p *OllamaProvider) flushToolCalls(ctx context.Context, ch chan<- types.StreamEvent, msg *types.AgentMessage, toolCalls map[int]*accumulatingCall) {
	// Sort indices for deterministic ordering
	indices := make([]int, 0, len(toolCalls))
	for i := range toolCalls {
		indices = append(indices, i)
	}
	// Simple sort (indices are small)
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			if indices[i] > indices[j] {
				indices[i], indices[j] = indices[j], indices[i]
			}
		}
	}

	for _, i := range indices {
		ac := toolCalls[i]
		if ac.name == "" {
			continue
		}
		argsJSON, _ := json.Marshal(ac.args)
		slog.Debug("ollama: tool call flushed", "tool", ac.name, "index", i, "args", string(argsJSON))

		sendEvent(ctx, ch, types.StreamEvent{
			Type:  types.EventToolCallStart,
			Delta: ac.name,
		})

		msg.Content = append(msg.Content, types.ContentBlock{
			Type: types.BlockToolCall,
			ToolCall: &types.ToolCallBlock{
				ID:        fmt.Sprintf("call_%d", i),
				Name:      ac.name,
				Arguments: ac.args,
			},
		})

		sendEvent(ctx, ch, types.StreamEvent{
			Type:    types.EventToolCallEnd,
			Delta:   ac.name,
			Message: msg,
		})
	}
}

// flushAccumulated consolidates fragmented content blocks into single blocks
// and emits final thinking_end if needed.
func (p *OllamaProvider) flushAccumulated(
	ctx context.Context, ch chan<- types.StreamEvent,
	msg *types.AgentMessage,
	thinkingAccum, textAccum *strings.Builder,
	thinkingActive bool,
) {
	if thinkingActive {
		sendEvent(ctx, ch, types.StreamEvent{
			Type:    types.EventThinkingEnd,
			Message: msg,
		})
	}
	// Note: msg.Content already has individual blocks appended during streaming.
	// The accumulators are for collecting the full text if needed.
	_ = thinkingAccum
	_ = textAccum
}

func (p *OllamaProvider) collectFromStream(ch <-chan types.StreamEvent) (*types.AgentMessage, error) {
	var lastMsg *types.AgentMessage

	for event := range ch {
		switch event.Type {
		case types.EventError:
			return nil, fmt.Errorf("ollama stream error: %s", event.Error)
		case types.EventDone:
			if event.Message != nil {
				lastMsg = event.Message
			}
		case types.EventTextDelta, types.EventThinkingDelta:
			if event.Message != nil {
				lastMsg = event.Message
			}
		}
	}

	if lastMsg == nil {
		return nil, fmt.Errorf("no response received from Ollama")
	}

	return lastMsg, nil
}

func findToolNameForCallID(messages []types.AgentMessage, callID string) string {
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == types.BlockToolCall && block.ToolCall != nil && block.ToolCall.ID == callID {
				return block.ToolCall.Name
			}
		}
	}
	return ""
}

// Ensure OllamaProvider implements Provider.
var _ Provider = (*OllamaProvider)(nil)
