package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestProviderConformance_AnthropicToolLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[]}}

`)
		fmt.Fprint(w, `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"read","input":{}}}

`)
		fmt.Fprint(w, `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"main.go\"}"}}

`)
		fmt.Fprint(w, `event: content_block_stop
data: {"type":"content_block_stop","index":0}

`)
		fmt.Fprint(w, `event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":5}}

`)
	}))
	defer server.Close()

	provider := NewAnthropicProviderWithClient("sk-test-key", &testHTTPClient{})
	model := types.Model{ID: "claude-test", API: "anthropic-messages", BaseURL: server.URL}
	events := collectStream(provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{}))
	assertToolLifecycleStream(t, events, toolLifecycleWant{
		wantStart: true,
		wantEnd:   true,
		id:        "toolu_123",
		name:      "read",
		argKey:    "path",
		argValue:  "main.go",
	})
}

func TestProviderConformance_GoogleToolLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"command":"pwd"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}

`)
	}))
	defer server.Close()

	provider := NewGoogleProviderWithClient("sk-test-key", &testHTTPClient{})
	model := types.Model{ID: "gemini-test", API: "google-generative-ai", BaseURL: server.URL}
	events := collectStream(provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{}))
	assertToolLifecycleStream(t, events, toolLifecycleWant{
		wantStart: true,
		wantEnd:   true,
		id:        "bash",
		name:      "bash",
		argKey:    "command",
		argValue:  "pwd",
	})
}
