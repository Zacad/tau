package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

func main() {
	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working dir: %v\n", err)
		os.Exit(1)
	}

	// Create tool registry with all 7 built-in tools
	reg := tools.NewRegistry(
		tools.WithBeforeToolCall(func(ctx types.BeforeToolCallContext) (*types.BeforeToolCallResult, error) {
			fmt.Printf("\n  ⚡ BeforeToolCall hook: %s\n", ctx.ToolName)
			return &types.BeforeToolCallResult{Allowed: true}, nil
		}),
		tools.WithAfterToolCall(func(ctx types.AfterToolCallContext) (*types.AfterToolCallResult, error) {
			fmt.Printf("  ✓ AfterToolCall hook: %s\n", ctx.ToolName)
			return nil, nil
		}),
	)

	maxChars := tools.DefaultMaxOutputChars
	reg.Register(tools.NewReadTool(workingDir, maxChars))
	reg.Register(tools.NewWriteTool(workingDir, maxChars))
	reg.Register(tools.NewEditTool(workingDir, maxChars))
	reg.Register(tools.NewBashTool(workingDir, maxChars, false))
	reg.Register(tools.NewGrepTool(workingDir, maxChars))
	reg.Register(tools.NewFindTool(workingDir, maxChars))
	reg.Register(tools.NewLsTool(workingDir, maxChars))

	fmt.Printf("🔧 Tool Registry: %d tools registered\n", len(reg.Names()))
	for _, n := range reg.Names() {
		fmt.Printf("   • %s\n", n)
	}
	fmt.Println()

	model := "llama3.2"
	ollamaURL := "http://localhost:11434/v1/chat/completions"

	fmt.Printf("🤖 Model: %s (Ollama)\n", model)
	fmt.Printf("📂 Working dir: %s\n", workingDir)
	fmt.Println()
	fmt.Println("Type a prompt or 'quit' to exit.")
	fmt.Println("The model can use all 7 tools: read, write, edit, bash, grep, find, ls")
	fmt.Println()

	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("You> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" || input == "quit" || input == "exit" {
			break
		}

		systemPrompt := "You are a helpful assistant with access to filesystem tools. " +
			"When asked to read, write, edit, search, or list files, use the appropriate tool. " +
			"Always use tools to fulfill requests. Be concise."

		// Build conversation history
		messages := []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input},
		}

		toolDefs := buildOllamaTools(reg.ToolDefinitions())

		resp, err := callOllama(ollamaURL, model, messages, toolDefs)
		if err != nil {
			fmt.Printf("❌ LLM error: %v\n", err)
			continue
		}

		// Show text response (if any)
		if resp.Content != "" {
			fmt.Printf("\n🤖 %s\n", resp.Content)
		}

		// Process tool calls
		if len(resp.ToolCalls) == 0 {
			fmt.Println()
			continue
		}

		fmt.Printf("\n🔧 Tool calls: %d\n", len(resp.ToolCalls))
		var toolCalls []tools.ToolCallRequest
		for _, tc := range resp.ToolCalls {
			fmt.Printf("   • %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
			toolCalls = append(toolCalls, tools.ToolCallRequest{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			})
		}

		// Add assistant message to history
		messages = append(messages, ollamaMessage{
			Role:       "assistant",
			Content:    resp.Content,
			ToolCalls:  resp.ToolCalls,
		})

		// Execute tools
		results := reg.ExecuteBatch(ctx, toolCalls)

		for _, res := range results {
			textResult := ""
			for _, cb := range res.Result.Content {
				textResult += cb.Text
			}
			status := "✓"
			if res.Result.IsError {
				status = "✗"
			}
			fmt.Printf("  %s %s: %s\n", status, res.Name, truncate(textResult, 200))

			// Add tool result to history
			messages = append(messages, ollamaMessage{
				Role:       "tool",
				Content:    textResult,
				ToolCallID: res.ID,
			})
		}

		// Ask LLM to process tool results
		fmt.Println("\n⏳ LLM processing tool results...")
		resp2, err := callOllama(ollamaURL, model, messages, toolDefs)
		if err != nil {
			fmt.Printf("❌ LLM error after tool execution: %v\n", err)
			continue
		}
		if resp2.Content != "" {
			fmt.Printf("\n🤖 %s\n", resp2.Content)
		}

		// If more tool calls, loop again (max 3 iterations to avoid infinite loops)
		moreCalls := resp2.ToolCalls
		for loop := 0; loop < 2 && len(moreCalls) > 0; loop++ {
			fmt.Printf("\n🔧 Additional tool calls: %d\n", len(moreCalls))
			toolCalls = nil
			for _, tc := range moreCalls {
				fmt.Printf("   • %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
				toolCalls = append(toolCalls, tools.ToolCallRequest{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: json.RawMessage(tc.Function.Arguments),
				})
			}

			messages = append(messages, ollamaMessage{
				Role:       "assistant",
				Content:    resp2.Content,
				ToolCalls:  moreCalls,
			})

			results = reg.ExecuteBatch(ctx, toolCalls)
			for _, res := range results {
				textResult := ""
				for _, cb := range res.Result.Content {
					textResult += cb.Text
				}
				status := "✓"
				if res.Result.IsError {
					status = "✗"
				}
				fmt.Printf("  %s %s: %s\n", status, res.Name, truncate(textResult, 200))

				messages = append(messages, ollamaMessage{
					Role:       "tool",
					Content:    textResult,
					ToolCallID: res.ID,
				})
			}

			fmt.Println("\n⏳ LLM processing tool results...")
			resp2, err = callOllama(ollamaURL, model, messages, toolDefs)
			if err != nil {
				fmt.Printf("❌ LLM error: %v\n", err)
				break
			}
			if resp2.Content != "" {
				fmt.Printf("\n🤖 %s\n", resp2.Content)
			}
			moreCalls = resp2.ToolCalls
		}

		fmt.Println()
	}
}

// ollamaMessage represents a message in the Ollama chat API format.
type ollamaMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []ollamaToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

// ollamaToolCall represents a tool call in the Ollama response.
type ollamaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ollamaResponse represents the Ollama chat completion response.
type ollamaResponse struct {
	Content   string
	ToolCalls []ollamaToolCall
}

// ollamaToolDefinition represents a tool definition for Ollama's API.
type ollamaToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

// callOllama makes a non-streaming request to Ollama's chat completions API.
func callOllama(url, model string, messages []ollamaMessage, tools []ollamaToolDefinition) (*ollamaResponse, error) {
	reqBody := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP error: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Role      string           `json:"role"`
				Content   string           `json:"content"`
				ToolCalls []ollamaToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := raw.Choices[0]
	resp := &ollamaResponse{
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
	}

	return resp, nil
}

// buildOllamaTools converts types.ToolDefinition to Ollama's tool format.
func buildOllamaTools(defs []types.ToolDefinition) []ollamaToolDefinition {
	result := make([]ollamaToolDefinition, 0, len(defs))
	for _, d := range defs {
		var td ollamaToolDefinition
		td.Type = "function"
		td.Function.Name = d.Name
		td.Function.Description = d.Description
		td.Function.Parameters = d.Parameters
		result = append(result, td)
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
