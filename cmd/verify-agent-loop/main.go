// Simple verification program for Task 012 (Agent Loop) against a local Ollama instance.
// Usage: go run ./cmd/verify-agent-loop "your prompt"
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/adam/tau/internal/agent"
	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/verify-agent-loop \"your prompt\"")
		os.Exit(1)
	}

	prompt := os.Args[1]
	cwd, _ := os.Getwd()

	// --- Ollama provider (OpenAI-compatible) ---
	ollama := provider.NewOpenAICompatProvider("", provider.OpenAICompatConfig{
		BaseURL:      "http://localhost:11434/v1",
		APIPath:      "/chat/completions",
		ProviderName: "ollama",
	})

	model := types.Model{
		ID:       "llama3.2",
		Name:     "Llama 3.2",
		Provider: "ollama",
		API:      "openai-completions",
		BaseURL:  "http://localhost:11434/v1",
	}

	// --- Tool registry ---
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadTool(cwd, 20000))
	registry.Register(tools.NewWriteTool(cwd, 20000))
	registry.Register(tools.NewEditTool(cwd, 20000))
	registry.Register(tools.NewBashTool(cwd, 50000, false))
	registry.Register(tools.NewGrepTool(cwd, 20000))
	registry.Register(tools.NewFindTool(cwd, 20000))
	registry.Register(tools.NewLsTool(cwd, 20000))

	// --- Agent ---
	systemPrompt := "You are a helpful assistant. You have access to tools — use them when appropriate. Always explain what you're doing before calling a tool."

	a := agent.New(agent.Options{
		SystemPrompt: systemPrompt,
		WorkingDir:   cwd,
		Provider:     ollama,
		Model:        model,
		ToolRegistry: registry,
	})

	// --- Event subscription ---
	var currentText string
	unsub := a.Subscribe(func(e types.AgentEvent) {
		switch e.Type {
		case types.AgentEventStart:
			fmt.Println("\n🚀 Agent started")
		case types.AgentEventTextDelta:
			if currentText == "" {
				fmt.Print("\n🤖 Assistant: ")
			}
			if s, ok := e.Data.(string); ok {
				fmt.Print(s)
				currentText += s
			}
		case types.AgentEventToolExecStart:
			fmt.Printf("\n🔧 Tool executing...\n")
		case types.AgentEventToolExecEnd:
			fmt.Printf("✅ Tool done\n")
		case types.AgentEventMessageEnd:
			fmt.Println()
		case types.AgentEventTurnEnd:
			fmt.Println("── turn end ──")
		case types.AgentEventAgentEnd:
			fmt.Println("🏁 Agent finished")
		case types.AgentEventError:
			fmt.Printf("❌ Error: %v\n", e.Data)
		}
	})
	defer unsub()

	// --- Abort on Ctrl+C ---
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		fmt.Println("\n⏹ Aborting...")
		a.Abort()
	}()

	// --- Run ---
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Println(strings.Repeat("─", 40))

	start := time.Now()
	err := a.Prompt(ctx, prompt)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n", err)
		os.Exit(1)
	}

	// --- Show transcript ---
	fmt.Println("\n📋 Transcript:")
	fmt.Println(strings.Repeat("─", 40))
	for _, msg := range a.Messages() {
		role := msg.Role
		text := extractText(msg)
		fmt.Printf("[%s] %s\n", role, text)
	}

	fmt.Printf("\n⏱ Duration: %s\n", elapsed)
	fmt.Printf("📊 Messages: %d\n", len(a.Messages()))
	fmt.Printf("📌 Final state: %s\n", a.State())
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
