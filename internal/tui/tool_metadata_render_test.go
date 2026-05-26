package tui

import (
	"strings"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestRenderToolCallBlock_PendingShowsFormattedMetadata(t *testing.T) {
	got := stripANSI(renderToolCallBlock(messageBlock{
		kind:     blockToolCall,
		toolName: "read",
		toolArgs: `{"path":"main.go","limit":20}`,
		toolSt:   toolPending,
	}, 80))

	if !strings.Contains(got, "read") {
		t.Fatalf("pending tool render missing name: %q", got)
	}
	if !strings.Contains(got, "path: main.go") {
		t.Fatalf("pending tool render missing formatted metadata: %q", got)
	}
}

func TestFormatToolArgs_FindPatternAndSubagent(t *testing.T) {
	find := formatToolArgs("find", `{"pattern":"*.go","path":"internal"}`)
	if !strings.Contains(find, "pattern: *.go") || !strings.Contains(find, "path: internal") {
		t.Fatalf("find summary = %q", find)
	}

	sub := formatToolArgs("subagent", `{"type":"researcher","task":"look up provider behavior","timeout":"5m"}`)
	if !strings.Contains(sub, "type: researcher") || !strings.Contains(sub, "task: look up provider behavior") || !strings.Contains(sub, "timeout: 5m") {
		t.Fatalf("subagent summary = %q", sub)
	}
}

func TestModel_ProcessEvent_TypedStartShowsNameAndMetadata(t *testing.T) {
	m := newTestModel()
	m.processEvent(testEvent(types.AgentEventToolExecStart, types.ToolLifecycleEvent{
		CallID:      "tc1",
		ToolName:    "bash",
		ArgsSummary: "go test ./...",
	}))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].toolName != "bash" {
		t.Fatalf("toolName = %q, want bash", m.blocks[0].toolName)
	}
	if m.blocks[0].toolArgs != "go test ./..." {
		t.Fatalf("toolArgs = %q", m.blocks[0].toolArgs)
	}
}
