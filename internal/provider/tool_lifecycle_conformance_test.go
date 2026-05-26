package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestProviderConformance_OpenAIResponsesToolLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `event: response.output_item.added
data: {"item": {"id": "fc_abc123", "type": "function_call", "name": "search", "call_id": "call_xyz", "arguments": ""}}

`)
		fmt.Fprint(w, `event: response.function_call_arguments.delta
data: {"delta": "{\"query\": \"scooters\"}"}

`)
		fmt.Fprint(w, `event: response.function_call_arguments.done
data: {"arguments": "{\"query\": \"scooters\"}"}

`)
		fmt.Fprint(w, `event: response.completed
data: {"usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}

`)
	}))
	defer server.Close()

	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})
	events := collectStream(provider.Stream(context.Background(), testModel(server.URL), nil, nil, types.StreamOptions{}))
	assertToolLifecycleStream(t, events, toolLifecycleWant{
		wantStart: true,
		wantEnd:   true,
		id:        "call_xyz|fc_abc123",
		name:      "search",
		argKey:    "query",
		argValue:  "scooters",
	})
}

func TestProviderConformance_OllamaToolLifecycle(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")
	streamData := []byte(`{"model":"gemma4:26b","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"bash","arguments":{"command":"ls -la"}}}]},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`)

	ch := make(chan types.StreamEvent, 64)
	go func() {
		p.parseStreamResponse(context.Background(), ch, bytes.NewReader(streamData), "gemma4:26b")
		close(ch)
	}()

	events := collectStream(ch)
	assertToolLifecycleStream(t, events, toolLifecycleWant{
		wantStart: true,
		wantEnd:   true,
		id:        "call_0",
		name:      "bash",
		argKey:    "command",
		argValue:  "ls -la",
	})
}

type toolLifecycleWant struct {
	wantStart bool
	wantEnd   bool
	id        string
	name      string
	argKey    string
	argValue  any
}

func assertToolLifecycleStream(t *testing.T, events []types.StreamEvent, want toolLifecycleWant) {
	t.Helper()
	var gotStart, gotEnd, gotDone bool
	var doneMsg *types.AgentMessage
	for _, e := range events {
		switch e.Type {
		case types.EventToolCallStart:
			gotStart = true
			if e.Delta != want.name {
				t.Fatalf("start delta = %q, want %q", e.Delta, want.name)
			}
		case types.EventToolCallEnd:
			gotEnd = true
			if e.Message != nil {
				assertEventMessageToolCall(t, e.Message, want)
			}
		case types.EventDone:
			gotDone = true
			doneMsg = e.Message
		}
	}
	if gotStart != want.wantStart {
		t.Fatalf("got start event = %v, want %v; events=%v", gotStart, want.wantStart, eventTypes(events))
	}
	if gotEnd != want.wantEnd {
		t.Fatalf("got end event = %v, want %v; events=%v", gotEnd, want.wantEnd, eventTypes(events))
	}
	if !gotDone {
		t.Fatalf("missing done event; events=%v", eventTypes(events))
	}
	assertEventMessageToolCall(t, doneMsg, want)
}

func assertEventMessageToolCall(t *testing.T, msg *types.AgentMessage, want toolLifecycleWant) {
	t.Helper()
	if msg == nil {
		t.Fatal("message is nil")
	}
	var calls []*types.ToolCallBlock
	for _, block := range msg.Content {
		if block.Type == types.BlockToolCall && block.ToolCall != nil {
			calls = append(calls, block.ToolCall)
		}
	}
	if len(calls) == 0 {
		t.Fatalf("message has no tool call blocks: %#v", msg.Content)
	}
	call := calls[len(calls)-1]
	if call.ID != want.id {
		t.Fatalf("tool call ID = %q, want %q", call.ID, want.id)
	}
	if call.Name != want.name {
		t.Fatalf("tool call name = %q, want %q", call.Name, want.name)
	}
	if got := call.Arguments[want.argKey]; got != want.argValue {
		t.Fatalf("tool arg %q = %#v, want %#v (args=%#v)", want.argKey, got, want.argValue, call.Arguments)
	}
}

func eventTypes(events []types.StreamEvent) []types.StreamEventType {
	typesOut := make([]types.StreamEventType, 0, len(events))
	for _, e := range events {
		typesOut = append(typesOut, e.Type)
	}
	return typesOut
}
