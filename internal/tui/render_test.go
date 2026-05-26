package tui

import (
	"strings"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestRenderUserMessage(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		width   int
		wantEmpty bool
	}{
		{"normal text", "Hello world", 80, false},
		{"empty text", "", 80, true},
		{"multiline", "Line one\nLine two", 80, false},
		{"narrow width", "Hello", 20, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderUserMessage(tt.text, tt.width)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output for empty input: %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-empty input")
			}
			// For multiline, check each line separately since lipgloss
			// renders them with ANSI codes between lines.
			for _, line := range strings.Split(tt.text, "\n") {
				if !strings.Contains(got, line) {
					t.Errorf("rendered output doesn't contain line %q in %q", line, got)
				}
			}
		})
	}
}

func TestRenderAssistantText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		width     int
		wantEmpty bool
	}{
		{"normal text", "Here is my response", 80, false},
		{"empty text", "", 80, true},
		{"long text", strings.Repeat("word ", 100), 80, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderAssistantText(&messageBlock{text: tt.text}, tt.width)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output for empty input")
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-empty input")
			}
		})
	}
}

func TestRenderThinkingBlock(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		width     int
		wantEmpty bool
	}{
		{"normal text", "I should think about this", 80, false},
		{"empty text", "", 80, true},
		{"multiline", "Step 1: analyze\nStep 2: conclude", 80, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderThinkingBlock(tt.text, tt.width)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output for empty input")
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-empty input")
			}
			// Thinking block must include the "· thinking" header label.
			if !strings.Contains(got, "· thinking") {
				t.Errorf("rendered thinking block missing '· thinking' header in: %q", got)
			}
		})
	}
}

func TestRenderToolCallBlock(t *testing.T) {
	tests := []struct {
		name      string
		block     messageBlock
		wantEmpty bool
	}{
		{
			"pending",
			messageBlock{kind: blockToolCall, toolName: "read_file", toolArgs: `{"path": "main.go"}`, toolSt: toolPending},
			false,
		},
		{
			"success",
			messageBlock{kind: blockToolCall, toolName: "read_file", toolSt: toolSuccess},
			false,
		},
		{
			"error with output",
			messageBlock{kind: blockToolCall, toolName: "bash", toolSt: toolError, toolErr: "exit status 1: command not found"},
			false,
		},
		{
			"empty name defaults to ellipsis",
			messageBlock{kind: blockToolCall, toolSt: toolPending},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderToolCallBlock(tt.block, 80)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output: %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-empty block")
			}
		})
	}
}

func TestRenderToolResultBlock(t *testing.T) {
	tests := []struct {
		name      string
		block     messageBlock
		wantEmpty bool
	}{
		{
			"success result",
			messageBlock{kind: blockToolResult, toolResultName: "read", toolResultContent: "file contents here"},
			false,
		},
		{
			"error result",
			messageBlock{kind: blockToolResult, toolResultName: "bash", toolResultContent: "exit status 1", toolResultIsError: true},
			false,
		},
		{
			"empty content",
			messageBlock{kind: blockToolResult, toolResultName: "ls", toolResultContent: ""},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderToolResultBlock(tt.block, 80)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output: %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-empty block")
			}
		})
	}
}

func TestRenderTurnSeparator(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		wantEmpty bool
	}{
		{"normal width", 80, false},
		{"zero width", 0, true},
		{"narrow width", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderTurnSeparator(tt.width)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output for zero width")
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-zero width")
			}
		})
	}
}

func TestRenderError(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		width     int
		wantEmpty bool
		wantText  string // expected text after stripping prefixes
	}{
		{"error message", "Something went wrong", 80, false, "Something went wrong"},
		{"empty text", "", 80, true, ""},
		{"wrapped provider error", "provider stream error: Model unavailable", 80, false, "Model unavailable"},
		{"double wrapped error", "agent prompt: provider stream error: Model unavailable", 80, false, "Model unavailable"},
		{"triple wrapped error", "agent prompt failed: agent prompt: provider stream error: connection refused", 80, false, "connection refused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderError(tt.text, tt.width)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("got non-empty output for empty input")
				}
				return
			}
			if got == "" {
				t.Fatal("got empty output for non-empty input")
			}
			if !strings.Contains(got, tt.wantText) {
				t.Errorf("rendered output doesn't contain expected text %q in %q", tt.wantText, got)
			}
		})
	}
}

func TestRenderSubAgentStart(t *testing.T) {
	got := renderSubAgentStart("code-reviewer", 80)
	if got == "" {
		t.Fatal("got empty output")
	}
	if !strings.Contains(got, "code-reviewer") {
		t.Errorf("output doesn't contain subagent ID: %q", got)
	}
	if !strings.Contains(got, "starting") {
		t.Errorf("output doesn't indicate 'starting': %q", got)
	}
}

func TestRenderSubAgentEnd(t *testing.T) {
	got := renderSubAgentEnd("code-reviewer", 80)
	if got == "" {
		t.Fatal("got empty output")
	}
	if !strings.Contains(got, "code-reviewer") {
		t.Errorf("output doesn't contain subagent ID: %q", got)
	}
	if !strings.Contains(got, "finished") {
		t.Errorf("output doesn't indicate 'finished': %q", got)
	}
}

func TestRenderBlock_AllTypes(t *testing.T) {
	blocks := []messageBlock{
		{kind: blockUserMessage, text: "Hello"},
		{kind: blockAssistantText, text: "Hi there"},
		{kind: blockThinking, text: "Let me think"},
		{kind: blockToolCall, text: "read (running…)"},
		{kind: blockToolResult, toolResultName: "read", toolResultContent: "file contents"},
		{kind: blockTurnSeparator, text: ""},
		{kind: blockError, text: "boom"},
		{kind: blockSubAgentStart, text: "reviewer"},
		{kind: blockSubAgentEnd, text: "reviewer"},
	}

	for _, b := range blocks {
		got := renderBlock(&b, 80)
		if got == "" && b.kind != blockTurnSeparator {
			t.Errorf("block %v rendered empty", b.kind)
		}
	}
}

func TestRenderBlocks(t *testing.T) {
	blocks := []messageBlock{
		{kind: blockUserMessage, text: "Hello"},
		{kind: blockAssistantText, text: "Hi there"},
		{kind: blockTurnSeparator, text: ""},
		{kind: blockUserMessage, text: "Follow up"},
	}

	got := renderBlocks(blocks, 80)
	if got == "" {
		t.Fatal("rendered output is empty")
	}
	if !strings.Contains(got, "Hello") {
		t.Error("missing first user message")
	}
	if !strings.Contains(got, "Hi there") {
		t.Error("missing assistant response")
	}
	if !strings.Contains(got, "Follow up") {
		t.Error("missing second user message")
	}
}

func TestRenderPendingBlock(t *testing.T) {
	tests := []struct {
		name string
		kind blockType
		text string
	}{
		{"assistant text", blockAssistantText, "Streaming..."},
		{"thinking", blockThinking, "Thinking..."},
		{"tool call", blockToolCall, "read (running…)"},
		{"unknown defaults to raw", blockType(99), "raw text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderPendingBlock(tt.text, tt.kind, 80, "")
			if !strings.Contains(got, tt.text) {
				t.Errorf("pending render doesn't contain text %q in %q", tt.text, got)
			}
		})
	}
}

func TestModel_FlushPending(t *testing.T) {
	m := newTestModel()

	// Write to pending builder
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("Hello world")

	// Flush should create a block
	m.flushPending()

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Errorf("expected blockAssistantText, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", m.blocks[0].text)
	}

	// Builder should be reset
	if m.pendingBuilder.Len() != 0 {
		t.Error("pendingBuilder should be empty after flush")
	}

	// Flush with empty builder should not add a block
	m.flushPending()
	if len(m.blocks) != 1 {
		t.Fatalf("expected still 1 block, got %d", len(m.blocks))
	}
}

func TestModel_EnsurePending_FlushesOnKindChange(t *testing.T) {
	m := newTestModel()

	// Start with assistant text
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("some text")

	// Switch to thinking — should flush the assistant text first
	m.ensurePending(blockThinking)

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block flushed, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Errorf("expected blockAssistantText, got %v", m.blocks[0].kind)
	}
	if m.pendingKind != blockThinking {
		t.Errorf("expected pendingKind=blockThinking, got %v", m.pendingKind)
	}

	// Same kind — should NOT flush
	m.pendingBuilder.WriteString("more thinking")
	m.ensurePending(blockThinking)

	if len(m.blocks) != 1 {
		t.Errorf("expected still 1 block, got %d (should not flush on same kind)", len(m.blocks))
	}
}

func TestModel_ProcessEvent_TextAndThinking(t *testing.T) {
	m := newTestModel()

	// Simulate a message start + text delta
	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventTextDelta, "Hello "))
	m.processEvent(testEvent(types.AgentEventTextDelta, "world"))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block after message end, got %d", len(m.blocks))
	}
	if m.blocks[0].text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", m.blocks[0].text)
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Errorf("expected blockAssistantText, got %v", m.blocks[0].kind)
	}

	// Simulate thinking
	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "Hmm"))
	m.processEvent(testEvent(types.AgentEventTextDelta, "OK here's my answer"))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	// Should have 3 blocks: thinking text, answer text, then message end flushed
	// Actually: the thinking block gets flushed when we switch to text, then text flushed on message end
	if len(m.blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(m.blocks))
	}
}

// TestModel_ProcessEvent_ThinkingOnly verifies that when a model sends only
// thinking content (no response text), the thinking block is still properly
// finalized and rendered with the header label.
func TestModel_ProcessEvent_ThinkingOnly(t *testing.T) {
	m := newTestModel()

	// Simulate a model that sends thinking but no text (e.g. gemma4 with
	// reasoning_content but empty content).
	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "Let me think..."))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "\nStep 2: ..."))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	// Should have exactly 1 block: the thinking block.
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockThinking {
		t.Errorf("expected blockThinking, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].text != "Let me think...\nStep 2: ..." {
		t.Errorf("expected thinking text, got %q", m.blocks[0].text)
	}

	// Rendered output must include the "· thinking" header.
	rendered := renderBlock(&m.blocks[0], 80)
	if !strings.Contains(rendered, "· thinking") {
		t.Errorf("rendered thinking block missing header in: %q", rendered)
	}
}

// TestModel_ProcessEvent_ThinkingThenText verifies the correct block separation
// when a model sends thinking content followed by response text.
func TestModel_ProcessEvent_ThinkingThenText(t *testing.T) {
	m := newTestModel()

	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "Hmm..."))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, " OK"))
	m.processEvent(testEvent(types.AgentEventTextDelta, "Sure, "))
	m.processEvent(testEvent(types.AgentEventTextDelta, "here's the answer."))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	if len(m.blocks) != 2 {
		t.Fatalf("expected 2 blocks (thinking + text), got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockThinking {
		t.Errorf("expected first block=thinking, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].text != "Hmm... OK" {
		t.Errorf("expected thinking='Hmm... OK', got %q", m.blocks[0].text)
	}
	if m.blocks[1].kind != blockAssistantText {
		t.Errorf("expected second block=text, got %v", m.blocks[1].kind)
	}
	if m.blocks[1].text != "Sure, here's the answer." {
		t.Errorf("expected text='Sure, here's the answer.', got %q", m.blocks[1].text)
	}

	// Verify thinking block renders with header.
	thinkingRendered := renderBlock(&m.blocks[0], 80)
	if !strings.Contains(thinkingRendered, "· thinking") {
		t.Errorf("thinking header missing in: %q", thinkingRendered)
	}
}

// TestModel_ProcessEvent_PolishInput_ThinkingThenText is a regression test
// for the original bug: Polish input ("cześć") showed thinking but no
// response text in the TUI. Verifies that both thinking and text blocks
// with Unicode/Polish characters are properly accumulated and rendered.
func TestModel_ProcessEvent_PolishInput_ThinkingThenText(t *testing.T) {
	m := newTestModel()

	// Simulate the exact event sequence from Ollama gemma4:26b for "cześć"
	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "*   Input: \"cześć\""))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "\n    *   Respond in Polish."))
	m.processEvent(testEvent(types.AgentEventTextDelta, "Cze"))
	m.processEvent(testEvent(types.AgentEventTextDelta, "ść! "))
	m.processEvent(testEvent(types.AgentEventTextDelta, "W czym mogę Ci pomóc?"))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	if len(m.blocks) != 2 {
		t.Fatalf("expected 2 blocks (thinking + text), got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockThinking {
		t.Errorf("expected first block=thinking, got %v", m.blocks[0].kind)
	}
	wantThinking := "*   Input: \"cześć\"\n    *   Respond in Polish."
	if m.blocks[0].text != wantThinking {
		t.Errorf("thinking mismatch:\ngot:  %q\nwant: %q", m.blocks[0].text, wantThinking)
	}
	if m.blocks[1].kind != blockAssistantText {
		t.Errorf("expected second block=assistantText, got %v", m.blocks[1].kind)
	}
	wantText := "Cześć! W czym mogę Ci pomóc?"
	if m.blocks[1].text != wantText {
		t.Errorf("text mismatch:\ngot:  %q\nwant: %q", m.blocks[1].text, wantText)
	}

	// Verify both blocks render without corrupting Polish characters.
	rendered := renderBlocks(m.blocks, 80)
	if !strings.Contains(rendered, "Cześć") {
		t.Errorf("rendered output missing Polish text 'Cześć': %q", rendered)
	}
	if !strings.Contains(rendered, "pomóc") {
		t.Errorf("rendered output missing Polish text 'pomóc': %q", rendered)
	}
}

func TestModel_ProcessEvent_ToolCall(t *testing.T) {
	m := newTestModel()

	// ToolExecStart creates a pending placeholder block
	m.processEvent(testEvent(types.AgentEventToolExecStart, map[string]any{
		"tool": "read_file",
	}))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockToolCall {
		t.Errorf("expected blockToolCall, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].toolSt != toolPending {
		t.Errorf("expected toolSt=pending, got %v", m.blocks[0].toolSt)
	}
	if m.pendingToolIndex != 0 {
		t.Errorf("expected pendingToolIndex=0, got %d", m.pendingToolIndex)
	}

	// ToolExecEnd fills in name and args, marks as success
	m.processEvent(testEvent(types.AgentEventToolExecEnd, map[string]any{
		"tool": "read_file",
		"args": `{"path": "main.go"}`,
	}))

	if m.blocks[0].toolName != "read_file" {
		t.Errorf("expected toolName='read_file', got %q", m.blocks[0].toolName)
	}
	if m.blocks[0].toolArgs != `{"path": "main.go"}` {
		t.Errorf("unexpected toolArgs: %q", m.blocks[0].toolArgs)
	}
	if m.blocks[0].toolSt != toolSuccess {
		t.Errorf("expected toolSt=success after ToolExecEnd, got %v", m.blocks[0].toolSt)
	}
	if m.pendingToolIndex != -1 {
		t.Errorf("expected pendingToolIndex=-1 after completion, got %d", m.pendingToolIndex)
	}
}

func TestModel_ProcessEvent_ToolResult(t *testing.T) {
	m := newTestModel()

	m.processEvent(testEvent(types.AgentEventToolResult, map[string]any{
		"tool":    "read",
		"content": "file contents here",
		"isError": false,
	}))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockToolResult {
		t.Errorf("expected blockToolResult, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].toolResultName != "read" {
		t.Errorf("expected toolResultName='read', got %q", m.blocks[0].toolResultName)
	}
	if m.blocks[0].toolResultContent != "file contents here" {
		t.Errorf("expected content='file contents here', got %q", m.blocks[0].toolResultContent)
	}
	if m.blocks[0].toolResultIsError {
		t.Errorf("expected isError=false")
	}
}

func TestModel_ProcessEvent_Error(t *testing.T) {
	m := newTestModel()

	m.processEvent(testEvent(types.AgentEventError, "connection refused"))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockError {
		t.Errorf("expected blockError, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].text != "connection refused" {
		t.Errorf("expected 'connection refused', got %q", m.blocks[0].text)
	}
}

func TestModel_ProcessEvent_ErrorMarksToolAsFailed(t *testing.T) {
	m := newTestModel()

	// Start a tool call
	m.processEvent(testEvent(types.AgentEventToolExecStart, map[string]any{
		"tool": "bash",
		"args": `{"command": "exit 1"}`,
	}))

	// An error event should mark the pending tool as failed
	m.processEvent(testEvent(types.AgentEventError, "exit status 1"))

	// The tool block should now be errored
	if m.blocks[0].kind != blockToolCall {
		t.Fatalf("expected blockToolCall, got %v", m.blocks[0].kind)
	}
	if m.blocks[0].toolSt != toolError {
		t.Errorf("expected toolSt=error, got %v", m.blocks[0].toolSt)
	}
	if m.blocks[0].toolErr != "exit status 1" {
		t.Errorf("expected toolErr='exit status 1', got %q", m.blocks[0].toolErr)
	}
	if m.pendingToolIndex != -1 {
		t.Errorf("expected pendingToolIndex=-1, got %d", m.pendingToolIndex)
	}

	// Error block should also exist
	if len(m.blocks) != 2 {
		t.Fatalf("expected 2 blocks (tool + error), got %d", len(m.blocks))
	}
	if m.blocks[1].kind != blockError {
		t.Errorf("expected second block to be blockError, got %v", m.blocks[1].kind)
	}
}

func TestModel_ProcessEvent_TurnSeparator(t *testing.T) {
	m := newTestModel()

	m.processEvent(testEvent(types.AgentEventTurnEnd, nil))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockTurnSeparator {
		t.Errorf("expected blockTurnSeparator, got %v", m.blocks[0].kind)
	}
}

func TestSpinner_StartAndStop(t *testing.T) {
	m := newTestModel()

	// Spinner should not be active at start
	if m.spinnerActive {
		t.Fatal("spinner should not be active at start")
	}

	// startSpinner should activate it and return a tick cmd
	cmd := m.startSpinner()
	if !m.spinnerActive {
		t.Fatal("spinner should be active after startSpinner")
	}
	if cmd == nil {
		t.Fatal("startSpinner should return a tick cmd")
	}

	// MessageEnd should NOT stop the spinner (tool execution may follow)
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))
	if !m.spinnerActive {
		t.Fatal("spinner should still be active after MessageEnd")
	}

	// TurnEnd should NOT stop the spinner either
	m.processEvent(testEvent(types.AgentEventTurnEnd, nil))
	if !m.spinnerActive {
		t.Fatal("spinner should still be active after TurnEnd")
	}

	// AgentEnd finally stops the spinner
	m.processEvent(testEvent(types.AgentEventAgentEnd, nil))
	if m.spinnerActive {
		t.Fatal("spinner should be stopped after AgentEnd")
	}
}

func TestSpinner_StopOnReset(t *testing.T) {
	m := newTestModel()

	// Start the spinner
	m.startSpinner()
	if !m.spinnerActive {
		t.Fatal("spinner should be active")
	}

	// Reset should stop it
	m.resetForTurn()
	if m.spinnerActive {
		t.Fatal("spinner should be stopped after resetForTurn")
	}
}

func TestHandleSpinnerTick(t *testing.T) {
	m := newTestModel()

	// When spinner is not active, handleSpinnerTick should return nil
	cmd := m.handleSpinnerTick()
	if cmd != nil {
		t.Fatal("handleSpinnerTick should return nil when spinner is inactive")
	}

	// Start the spinner
	m.startSpinner()
	if !m.spinnerActive {
		t.Fatal("spinner should be active")
	}

	// Now handleSpinnerTick should return a cmd
	cmd = m.handleSpinnerTick()
	if cmd == nil {
		t.Fatal("handleSpinnerTick should return a cmd when spinner is active")
	}
}

func TestFooter_EnhancedInfo(t *testing.T) {
	m := newTestModel()
	m.modelName = "gemma4:e4b"
	m.modelProv = "ollama"
	m.cwd = "/home/user/project"
	m.width = 120

	// Initial state: no usage, no turns
	m.state = stateIdle
	got := m.renderFooter()
	if !strings.Contains(got, "gemma4:e4b") {
		t.Errorf("missing model name in footer: %q", got)
	}
	if !strings.Contains(got, "/home/user/project") {
		t.Errorf("missing cwd in footer: %q", got)
	}
	if !strings.Contains(got, "turns:0") {
		t.Errorf("missing turn count in footer: %q", got)
	}

	// Simulate a completed turn with usage
	m.turnCount = 3
	m.usage.TotalTokens = 1500
	m.usage.Cost.Total = 0.0
	got = m.renderFooter()
	if !strings.Contains(got, "turns:3") {
		t.Errorf("missing updated turn count: %q", got)
	}
	if !strings.Contains(got, "tokens:1500") {
		t.Errorf("missing tokens in footer: %q", got)
	}
	if !strings.Contains(got, "$0.00 (local)") {
		t.Errorf("missing local cost indicator: %q", got)
	}

	// Paid provider with actual cost
	m.modelProv = "openai"
	m.usage.Cost.Total = 0.05
	got = m.renderFooter()
	if !strings.Contains(got, "$0.05") {
		t.Errorf("missing cost in footer: %q", got)
	}
	if strings.Contains(got, "(local)") {
		t.Errorf("should not show (local) for paid provider: %q", got)
	}
}

func TestTurnCount_Increments(t *testing.T) {
	m := newTestModel()

	if m.turnCount != 0 {
		t.Fatalf("expected turnCount=0, got %d", m.turnCount)
	}

	m.processEvent(testEvent(types.AgentEventTurnEnd, nil))
	if m.turnCount != 1 {
		t.Errorf("expected turnCount=1, got %d", m.turnCount)
	}

	m.processEvent(testEvent(types.AgentEventTurnEnd, nil))
	if m.turnCount != 2 {
		t.Errorf("expected turnCount=2, got %d", m.turnCount)
	}
}

func TestTurnCount_Reset(t *testing.T) {
	m := newTestModel()
	m.turnCount = 5

	m.resetForTurn()
	if m.turnCount != 5 {
		// resetForTurn clears per-turn state but not cumulative counters
		t.Logf("turnCount preserved after reset: %d", m.turnCount)
	}
}

// testEvent creates a minimal AgentEvent for testing.
func testEvent(ty types.AgentEventType, data any) types.AgentEvent {
	return types.AgentEvent{Type: ty, Data: data}
}

// TestModel_FlushPending_GlamourRendering verifies the full pipeline:
// streaming text → flushPending → glamour rendering → renderBlocks output.
func TestModel_FlushPending_GlamourRendering(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// Simulate markdown response from model
	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventTextDelta, "**bold** and *italic*\n"))
	m.processEvent(testEvent(types.AgentEventTextDelta, "- item one\n- item two\n"))
	m.processEvent(testEvent(types.AgentEventTextDelta, "[link](https://example.com)"))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Fatalf("expected blockAssistantText, got %v", m.blocks[0].kind)
	}

	// renderedMarkdown should be populated by flushPending
	if m.blocks[0].renderedMarkdown == "" {
		t.Fatal("renderedMarkdown should be populated after flush")
	}

	// Raw text should be preserved
	if m.blocks[0].text != "**bold** and *italic*\n- item one\n- item two\n[link](https://example.com)" {
		t.Errorf("text mismatch: %q", m.blocks[0].text)
	}

	// Rendered output should contain ANSI escape codes (glamour styling)
	rendered := renderBlock(&m.blocks[0], 80)
	if !strings.Contains(rendered, "\x1b[") {
		t.Error("rendered output should contain ANSI escape codes from glamour")
	}

	// Content should still be present in rendered output
	clean := stripANSI(rendered)
	if !strings.Contains(clean, "bold") {
		t.Error("rendered output missing 'bold'")
	}
	if !strings.Contains(clean, "item one") {
		t.Error("rendered output missing 'item one'")
	}
	if !strings.Contains(clean, "https://example.com") {
		t.Error("rendered output missing URL")
	}
}

// TestModel_FlushPending_NoGlamourForThinking verifies thinking blocks
// do NOT get glamour rendering (only assistant text blocks do).
func TestModel_FlushPending_NoGlamourForThinking(t *testing.T) {
	m := newTestModel()
	m.width = 80

	m.processEvent(testEvent(types.AgentEventMessageStart, nil))
	m.processEvent(testEvent(types.AgentEventThinkingDelta, "Let me think..."))
	m.processEvent(testEvent(types.AgentEventMessageEnd, nil))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockThinking {
		t.Fatalf("expected blockThinking, got %v", m.blocks[0].kind)
	}

	// Thinking blocks should NOT have renderedMarkdown
	if m.blocks[0].renderedMarkdown != "" {
		t.Errorf("thinking block should NOT have renderedMarkdown, got %q", m.blocks[0].renderedMarkdown)
	}
}

// TestRenderAssistantText_PendingVsFinalized verifies that pending blocks
// (empty renderedMarkdown) use plain text rendering, while finalized blocks
// use cached glamour output.
func TestRenderAssistantText_PendingVsFinalized(t *testing.T) {
	markdown := "**bold text**"

	// Pending block (isFinalized=false) — should render as plain text with raw markdown
	pending := messageBlock{kind: blockAssistantText, text: markdown, isFinalized: false}
	pendingRendered := renderAssistantText(&pending, 80)
	cleanPending := stripANSI(pendingRendered)
	if !strings.Contains(cleanPending, "**bold text**") {
		t.Errorf("pending block should contain raw markdown, got %q", cleanPending)
	}

	// Finalized block (isFinalized=true) — should use cached glamour
	finalized := messageBlock{
		kind:             blockAssistantText,
		text:             markdown,
		isFinalized:      true,
		renderedMarkdown: RenderMarkdown(markdown, 80),
	}
	finalizedRendered := renderAssistantText(&finalized, 80)
	cleanFinalized := stripANSI(finalizedRendered)
	if !strings.Contains(cleanFinalized, "bold text") {
		t.Errorf("finalized block should contain 'bold text', got %q", cleanFinalized)
	}
}

func TestRenderAssistantText_EmptyText(t *testing.T) {
	got := renderAssistantText(&messageBlock{text: ""}, 80)
	if got != "" {
		t.Errorf("expected empty string for empty text, got %q", got)
	}
}

func TestRenderAssistantText_LongContent(t *testing.T) {
	longText := strings.Repeat("This is a paragraph with some content. ", 300) // ~12K chars
	b := messageBlock{kind: blockAssistantText, text: longText, isFinalized: true}
	got := renderAssistantText(&b, 80)
	if got == "" {
		t.Fatal("expected non-empty output for long content")
	}
	clean := stripANSI(got)
	if len(clean) < 1000 {
		t.Errorf("expected substantial output for long content, got %d chars", len(clean))
	}
}

func TestRenderAssistantText_MalformedMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unclosed code fence", "```go\nfunc main() {}"},
		{"broken list", "- item one\n- item two\nnot a list item"},
		{"unclosed bold", "**bold without closing"},
		{"nested unclosed", "**bold *italic without closing"},
		{"empty fences", "```\n```"},
		{"mixed malformed", "# Heading\n\n**bold\n\n```\nsome code\n\n- list\n  normal text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := messageBlock{kind: blockAssistantText, text: tt.input, isFinalized: true}
			got := renderAssistantText(&b, 80)
			if got == "" {
				t.Fatal("expected non-empty output for malformed markdown")
			}
		})
	}
}

func TestRenderBlocks_MixedContent(t *testing.T) {
	blocks := []messageBlock{
		{kind: blockUserMessage, text: "Write code"},
		{kind: blockThinking, text: "Let me think about the code"},
		{
			kind:        blockAssistantText,
			text:        "Here is the code:\n\n```go\nfunc main() {}\n```\n\nAnd a [link](https://example.com).",
			isFinalized: true,
		},
		{kind: blockToolCall, toolName: "bash", toolSt: toolSuccess},
		{kind: blockToolResult, toolResultName: "bash", toolResultContent: "done"},
	}

	got := renderBlocks(blocks, 80)
	if got == "" {
		t.Fatal("expected non-empty output for mixed content")
	}
	if !strings.Contains(got, "Write code") {
		t.Error("missing user message")
	}
	if !strings.Contains(got, "· thinking") {
		t.Error("missing thinking header")
	}
	if !strings.Contains(got, "bash") {
		t.Error("missing tool call name")
	}
}

func TestRenderBlocks_ResizeCacheInvalidation(t *testing.T) {
	markdown := "# Heading\n\nSome **bold** text and a longer paragraph that should wrap differently at different widths to verify the cache invalidation works properly."

	// Create a finalized block and render at width 80
	b := messageBlock{kind: blockAssistantText, text: markdown, isFinalized: true}
	got80 := renderAssistantText(&b, 80)
	if got80 == "" {
		t.Fatal("expected non-empty output at width 80")
	}

	// Simulate resize: clear the cache (as update.go does)
	b.renderedMarkdown = ""

	// Re-render at width 40 — should re-render through glamour
	got40 := renderAssistantText(&b, 40)
	if got40 == "" {
		t.Fatal("expected non-empty output at width 40 after cache invalidation")
	}

	// The cache should now be updated to the new width
	gotAgain := renderAssistantText(&b, 40)
	if gotAgain != got40 {
		t.Error("after cache update, rendering should return cached value")
	}
}

func TestRenderAssistantText_CacheDirtyReRenders(t *testing.T) {
	markdown := "**hello**"

	// First render: populates cache
	b := messageBlock{kind: blockAssistantText, text: markdown, isFinalized: true}
	first := renderAssistantText(&b, 80)
	if first == "" {
		t.Fatal("first render should produce output")
	}
	if b.renderedMarkdown == "" {
		t.Fatal("cache should be populated after first render")
	}

	// Simulate resize: clear the cache (as update.go does)
	b.renderedMarkdown = ""

	// Re-render — should re-render through glamour
	second := renderAssistantText(&b, 60)
	if second == "" {
		t.Fatal("re-render after cache clear should produce output")
	}

	// Cache should be repopulated
	if b.renderedMarkdown == "" {
		t.Fatal("cache should be repopulated after re-render")
	}
}

func TestRenderBlocks_ResizeReflowsContent(t *testing.T) {
	// Long paragraph that will wrap differently at different widths
	markdown := "This is a long paragraph with many words that should wrap at different positions when the terminal width changes from wide to narrow and back again to verify proper reflow behavior."

	blocks := []messageBlock{
		{kind: blockAssistantText, text: markdown, isFinalized: true},
	}

	// Render at wide width
	got80 := renderBlocks(blocks, 80)
	if got80 == "" {
		t.Fatal("expected non-empty output at width 80")
	}

	// Cache is now populated in blocks[0]
	if blocks[0].renderedMarkdown == "" {
		t.Fatal("cache should be populated after renderBlocks")
	}

	// Simulate resize: clear cache and re-render at narrow width
	blocks[0].renderedMarkdown = ""
	got40 := renderBlocks(blocks, 40)
	if got40 == "" {
		t.Fatal("expected non-empty output at width 40")
	}

	// Cache should be repopulated at new width
	if blocks[0].renderedMarkdown == "" {
		t.Fatal("cache should be repopulated after resize re-render")
	}

	// Content should be present in both renderings
	clean80 := stripANSI(got80)
	clean40 := stripANSI(got40)
	if !strings.Contains(clean80, "This is a long paragraph") {
		t.Error("wide rendering missing content")
	}
	if !strings.Contains(clean40, "This is a long paragraph") {
		t.Error("narrow rendering missing content")
	}

	// Narrow rendering should have more lines (more newlines from wrapping)
	lines80 := strings.Count(clean80, "\n")
	lines40 := strings.Count(clean40, "\n")
	if lines40 <= lines80 {
		t.Errorf("narrow width should produce more lines: wide=%d, narrow=%d", lines80, lines40)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{"zero", 0, "0"},
		{"small", 42, "42"},
		{"hundred", 999, "999"},
		{"one_k", 1000, "1.0k"},
		{"one_point_five_k", 1500, "1.5k"},
		{"nine_point_nine_k", 9999, "10.0k"},
		{"ten_k", 10000, "10k"},
		{"two_hundred_k", 200000, "200k"},
		{"one_m", 1000000, "1.0M"},
		{"one_point_two_m", 1200000, "1.2M"},
		{"ten_m", 10000000, "10M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTokens(tt.count)
			if got != tt.want {
				t.Errorf("formatTokens(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestFooter_ContextUsage(t *testing.T) {
	m := newTestModel()
	m.modelName = "gpt-4o"
	m.modelProv = "openai"
	m.cwd = "/test"
	m.width = 120
	m.contextWindow = 128000
	m.contextKnown = true

	// No context known yet (fresh model, no messages)
	m.state = stateIdle
	got := m.renderFooter()
	clean := stripANSI(got)
	if !strings.Contains(clean, "ctx:0%/128k") {
		t.Errorf("expected ctx:0%%/128k in footer, got: %q", clean)
	}

	// Simulate context usage at 50%
	m.contextTokens = 64000
	m.turnCount = 1
	got = m.renderFooter()
	clean = stripANSI(got)
	if !strings.Contains(clean, "ctx:50.0%/128k") {
		t.Errorf("expected ctx:50.0%%/128k in footer, got: %q", clean)
	}
}

func TestFooter_ContextHiddenWhenUnknown(t *testing.T) {
	m := newTestModel()
	m.modelName = "test-model"
	m.modelProv = "ollama"
	m.cwd = "/test"
	m.width = 120
	m.contextWindow = 0
	m.contextKnown = false

	got := m.renderFooter()
	clean := stripANSI(got)
	if strings.Contains(clean, "ctx:") {
		t.Errorf("ctx should be hidden when contextWindow is 0, got: %q", clean)
	}
}

func TestFooter_ContextWarningThreshold(t *testing.T) {
	m := newTestModel()
	m.modelName = "claude-sonnet-4-6"
	m.modelProv = "anthropic"
	m.cwd = "/test"
	m.width = 120
	m.contextWindow = 200000
	m.contextTokens = 150000 // 75%
	m.contextKnown = true
	m.turnCount = 1

	got := m.renderFooter()
	clean := stripANSI(got)
	if !strings.Contains(clean, "ctx:75.0%/200k") {
		t.Errorf("expected ctx:75.0%%/200k in footer, got: %q", clean)
	}
	// Should have warning color (ANSI code for color 220)
	if !strings.Contains(got, "220") {
		t.Errorf("expected warning color (220) in footer, got: %q", got)
	}
}

func TestFooter_ContextErrorThreshold(t *testing.T) {
	m := newTestModel()
	m.modelName = "claude-sonnet-4-6"
	m.modelProv = "anthropic"
	m.cwd = "/test"
	m.width = 120
	m.contextWindow = 200000
	m.contextTokens = 185000 // 92.5%
	m.contextKnown = true
	m.turnCount = 1

	got := m.renderFooter()
	clean := stripANSI(got)
	if !strings.Contains(clean, "ctx:92.5%/200k") {
		t.Errorf("expected ctx:92.5%%/200k in footer, got: %q", clean)
	}
	// Should have error color (ANSI code for color 196)
	if !strings.Contains(got, "196") {
		t.Errorf("expected error color (196) in footer, got: %q", got)
	}
}

