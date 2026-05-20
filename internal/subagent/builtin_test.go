package subagent

import (
	"context"
	"strings"
	"testing"

	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

func TestAllTypes_ReturnsSixTypes(t *testing.T) {
	all := AllTypes()
	if len(all) != 6 {
		t.Errorf("expected 6 types, got %d", len(all))
	}

	expected := map[Type]bool{
		TypeGeneral:          true,
		TypeResearcher:       true,
		TypeReviewer:         true,
		TypeImplementor:      true,
		TypeSecurityReviewer: true,
		TypeQA:               true,
	}

	for _, at := range all {
		if !expected[at] {
			t.Errorf("unexpected type: %q", at)
		}
		delete(expected, at)
	}

	if len(expected) > 0 {
		t.Errorf("missing types: %v", expected)
	}
}

func TestDefaultToolSet_EachType(t *testing.T) {
	tests := []struct {
		agentType Type
		expected  []string
	}{
		{TypeGeneral, []string{"read", "write", "edit", "bash", "grep", "find", "ls", "websearch", "webfetch"}},
		{TypeResearcher, []string{"read", "grep", "find", "ls", "bash", "websearch", "webfetch"}},
		{TypeReviewer, []string{"read", "grep", "find", "ls"}},
		{TypeImplementor, []string{"read", "write", "edit", "bash", "grep", "find", "ls"}},
		{TypeSecurityReviewer, []string{"read", "grep", "find", "bash"}},
		{TypeQA, []string{"read", "bash", "grep", "find", "ls", "write"}},
	}

	for _, tc := range tests {
		t.Run(string(tc.agentType), func(t *testing.T) {
			tools := DefaultToolSet(tc.agentType)
			if tools == nil {
				t.Fatalf("expected tool set for %q, got nil", tc.agentType)
			}
			if len(tools) != len(tc.expected) {
				t.Errorf("expected %d tools, got %d", len(tc.expected), len(tools))
			}

			got := make(map[string]bool)
			for _, name := range tools {
				got[name] = true
			}
			for _, name := range tc.expected {
				if !got[name] {
					t.Errorf("missing tool %q for type %q", name, tc.agentType)
				}
			}
		})
	}
}

func TestDefaultToolSet_UnknownType(t *testing.T) {
	tools := DefaultToolSet("unknown_type")
	if tools != nil {
		t.Errorf("expected nil for unknown type, got %v", tools)
	}
}

func TestDefaultSystemPrompt_EachType(t *testing.T) {
	for _, at := range AllTypes() {
		prompt := DefaultSystemPrompt(at)
		if prompt == "" {
			t.Errorf("expected non-empty system prompt for %q", at)
		}
		if len(prompt) < 50 {
			t.Errorf("system prompt for %q too short (%d chars)", at, len(prompt))
		}
	}
}

func TestDefaultSystemPrompt_UnknownType(t *testing.T) {
	prompt := DefaultSystemPrompt("unknown_type")
	if prompt != "" {
		t.Errorf("expected empty string for unknown type, got %q", prompt)
	}
}

func TestValidType(t *testing.T) {
	for _, at := range AllTypes() {
		if !ValidType(at) {
			t.Errorf("ValidType(%q) should be true", at)
		}
	}

	if ValidType("unknown") {
		t.Error("ValidType('unknown') should be false")
	}
}

func TestParseType(t *testing.T) {
	tests := []struct {
		input    string
		expected Type
		valid    bool
	}{
		{"general", TypeGeneral, true},
		{"researcher", TypeResearcher, true},
		{"reviewer", TypeReviewer, true},
		{"implementor", TypeImplementor, true},
		{"security_reviewer", TypeSecurityReviewer, true},
		{"security-reviewer", TypeSecurityReviewer, true},
		{"security reviewer", TypeSecurityReviewer, true},
		{"SECURITY_REVIEWER", TypeSecurityReviewer, true},
		{"qa", TypeQA, true},
		{"  QA  ", TypeQA, true},
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := ParseType(tc.input)
			if ok != tc.valid {
				t.Errorf("ParseType(%q) valid: expected %v, got %v", tc.input, tc.valid, ok)
			}
			if got != tc.expected {
				t.Errorf("ParseType(%q) type: expected %q, got %q", tc.input, tc.expected, got)
			}
		})
	}
}

func TestNewSubAgentByType_UnknownType(t *testing.T) {
	mp := &testutil.MockProvider{}

	_, err := NewSubAgentByType("nonexistent", mp, nil, SubAgentOpts{
		Task: "test",
	})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown agent type") {
		t.Errorf("expected 'unknown agent type' error, got: %v", err)
	}
}

func TestNewSubAgentByType_DefaultToGeneral(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa, err := NewSubAgentByType("", mp, nil, SubAgentOpts{
		Task: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa.Type != TypeGeneral {
		t.Errorf("expected default type to be general, got %q", sa.Type)
	}
}

func TestNewSubAgentByType_ToolFiltering(t *testing.T) {
	mp := &testutil.MockProvider{}

	parentTools := []Tool{
		&mockTool{name: "read", description: "Read a file", result: &types.ToolResult{}},
		&mockTool{name: "write", description: "Write a file", result: &types.ToolResult{}},
		&mockTool{name: "edit", description: "Edit a file", result: &types.ToolResult{}},
		&mockTool{name: "bash", description: "Run bash", result: &types.ToolResult{}},
		&mockTool{name: "grep", description: "Search content", result: &types.ToolResult{}},
		&mockTool{name: "find", description: "Search files", result: &types.ToolResult{}},
		&mockTool{name: "ls", description: "List directory", result: &types.ToolResult{}},
	}

	sa, err := NewSubAgentByType(TypeReviewer, mp, parentTools, SubAgentOpts{
		Task: "review code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTools := map[string]bool{"read": true, "grep": true, "find": true, "ls": true}
	if len(sa.Tools) != len(expectedTools) {
		t.Errorf("expected %d tools for reviewer, got %d", len(expectedTools), len(sa.Tools))
	}

	for _, tool := range sa.Tools {
		if !expectedTools[tool.Name()] {
			t.Errorf("unexpected tool %q for reviewer type", tool.Name())
		}
	}
}

func TestNewSubAgentByType_SystemPromptMerged(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa, err := NewSubAgentByType(TypeResearcher, mp, nil, SubAgentOpts{
		Task:         "research",
		SystemPrompt: "Custom prefix.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(sa.SystemPrompt, "Custom prefix.") {
		t.Error("expected custom system prompt prefix to be included")
	}
	if !strings.Contains(sa.SystemPrompt, "Research specialist") {
		t.Error("expected default system prompt to be appended")
	}
}

func TestNewSubAgentByType_SystemPromptDefaultOnly(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa, err := NewSubAgentByType(TypeQA, mp, nil, SubAgentOpts{
		Task: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(sa.SystemPrompt, "Quality Assurance") {
		t.Error("expected QA system prompt, got: " + sa.SystemPrompt[:50])
	}
}

func TestNewSubAgentByType_TypeFieldSet(t *testing.T) {
	mp := &testutil.MockProvider{}

	for _, at := range AllTypes() {
		sa, err := NewSubAgentByType(at, mp, nil, SubAgentOpts{
			Task: "test",
		})
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", at, err)
		}
		if sa.Type != at {
			t.Errorf("expected type %q, got %q", at, sa.Type)
		}
	}
}

func TestNewSubAgentByType_WithMockProvider(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "implementor response"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	parentTools := []Tool{
		&mockTool{name: "read", description: "Read", result: &types.ToolResult{}},
		&mockTool{name: "write", description: "Write", result: &types.ToolResult{}},
		&mockTool{name: "edit", description: "Edit", result: &types.ToolResult{}},
		&mockTool{name: "bash", description: "Bash", result: &types.ToolResult{}},
		&mockTool{name: "grep", description: "Grep", result: &types.ToolResult{}},
		&mockTool{name: "find", description: "Find", result: &types.ToolResult{}},
		&mockTool{name: "ls", description: "Ls", result: &types.ToolResult{}},
	}

	sa, err := NewSubAgentByType(TypeImplementor, mp, parentTools, SubAgentOpts{
		Task: "implement feature",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output != "implementor response" {
		t.Errorf("expected output 'implementor response', got %q", result.Output)
	}
}

func TestNewSubAgentByType_ParentToolNamesPassed(t *testing.T) {
	mp := &testutil.MockProvider{}

	parentTools := []Tool{
		&mockTool{name: "read", description: "Read", result: &types.ToolResult{}},
	}

	sa, err := NewSubAgentByType(TypeReviewer, mp, parentTools, SubAgentOpts{
		Task:            "review",
		ParentToolNames: []string{"read", "grep", "find", "ls"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sa.Tools) != 1 {
		t.Errorf("expected 1 tool (read), got %d", len(sa.Tools))
	}
}
