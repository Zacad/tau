package provider

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

func TestOllamaProvider_WithThinkingLevel(t *testing.T) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		t.Skipf("Ollama is not available: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Skipf("Ollama is not available: status %d", resp.StatusCode)
	}

	ollama := NewOllamaProvider("http://localhost:11434")
	model := types.Model{
		ID:        "gemma4:26b",
		Provider:  "ollama",
		API:       "ollama-chat",
		BaseURL:   "http://localhost:11434",
		Reasoning: true,
	}

	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "say hello"}}},
	}

	for _, level := range []types.ThinkingLevel{"", "low", "medium", "high"} {
		t.Run(string(level), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			opts := types.StreamOptions{ThinkingLevel: level}
			ch := ollama.Stream(ctx, model, messages, nil, opts)

			eventCount := 0
			for event := range ch {
				eventCount++
				if event.Type == types.EventDone {
					t.Logf("Done after %d events", eventCount)
					return
				}
				if event.Type == types.EventError {
					t.Fatalf("Error: %s", event.Error)
				}
			}
			t.Fatal("Channel closed without done event")
		})
	}
}
