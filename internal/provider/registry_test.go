package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestRegistry_ResolveModelWithFallback_ExactProviderModel(t *testing.T) {
	r := NewRegistry()

	m, err := r.ResolveModelWithFallback("openai/gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gpt-4o" || m.Provider != "openai" {
		t.Fatalf("expected openai/gpt-4o, got %s/%s", m.Provider, m.ID)
	}

	m, err = r.ResolveModelWithFallback("anthropic/claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "claude-sonnet-4-20250514" || m.Provider != "anthropic" {
		t.Fatalf("expected anthropic/claude-sonnet-4-20250514, got %s/%s", m.Provider, m.ID)
	}
}

func TestRegistry_ResolveModelWithFallback_ExactBareID(t *testing.T) {
	r := NewRegistry()

	// gpt-4o is unique across providers
	m, err := r.ResolveModelWithFallback("gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", m.ID)
	}

	// gemini-2.5-pro is unique
	m, err = r.ResolveModelWithFallback("gemini-2.5-pro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gemini-2.5-pro" {
		t.Fatalf("expected gemini-2.5-pro, got %s", m.ID)
	}
}

func TestRegistry_ResolveModelWithFallback_PartialMatch(t *testing.T) {
	r := NewRegistry()

	// "sonnet" matches multiple Anthropic models — should pick one deterministically
	m, err := r.ResolveModelWithFallback("sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Provider != "anthropic" {
		t.Fatalf("expected anthropic provider, got %s", m.Provider)
	}

	// "gpt-4" matches multiple — should resolve deterministically
	m, err = r.ResolveModelWithFallback("gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(m.ID, "gpt-4") {
		t.Fatalf("expected gpt-4 prefix, got %s", m.ID)
	}

	// "gemini-2.5" matches two models — should pick latest
	m, err = r.ResolveModelWithFallback("gemini-2.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gemini-2.5-pro" {
		t.Fatalf("expected gemini-2.5-pro (latest alias), got %s", m.ID)
	}
}

func TestRegistry_ResolveModelWithFallback_Default(t *testing.T) {
	r := NewRegistry()
	r.SetDefaultModel("gpt-4o")

	m, err := r.ResolveModelWithFallback("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", m.ID)
	}
}

func TestRegistry_ResolveModelWithFallback_NoMatch(t *testing.T) {
	r := NewRegistry()

	_, err := r.ResolveModelWithFallback("nonexistent-model-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
	if !strings.Contains(err.Error(), "available:") {
		t.Fatalf("expected error to list available models, got: %v", err)
	}
}

func TestRegistry_ResolveModelWithFallback_NoDefault(t *testing.T) {
	r := NewRegistry()

	_, err := r.ResolveModelWithFallback("")
	if err == nil {
		t.Fatal("expected error when no pattern and no default")
	}
}

func TestIsAlias(t *testing.T) {
	tests := []struct {
		id      string
		isAlias bool
	}{
		{"gpt-4o", true},                       // no date suffix
		{"claude-sonnet-4", true},               // no date suffix
		{"gemini-2.5-pro", true},               // no date suffix
		{"gpt-4o-latest", true},                 // explicit -latest
		{"claude-sonnet-4-20250514", false},     // date suffix
		{"claude-3-7-sonnet-20250219", false},  // date suffix
		{"gemini-2.0-flash-20241201", false},   // date suffix
		{"short", true},                         // too short for date
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			result := isAlias(tc.id)
			if result != tc.isAlias {
				t.Errorf("isAlias(%q) = %v, want %v", tc.id, result, tc.isAlias)
			}
		})
	}
}

func TestRegistry_ResolveModelWithFallback_CustomModel(t *testing.T) {
	r := NewRegistry()

	// Register a custom model
	r.models.Register(types.Model{
		ID:       "llama3-8b",
		Name:     "Llama 3 8B",
		Provider: "ollama",
		API:      "openai-completions",
	})

	m, err := r.ResolveModelWithFallback("llama3-8b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "llama3-8b" || m.Provider != "ollama" {
		t.Fatalf("expected ollama/llama3-8b, got %s/%s", m.Provider, m.ID)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()

	// Register a test provider
	testProv := &testProvider{name: "test-provider"}
	r.Register(testProv)

	// Verify it's registered
	_, ok := r.Get("test-provider")
	if !ok {
		t.Fatal("expected test-provider to be registered")
	}

	// Unregister test-provider
	r.Unregister("test-provider")

	// Verify it's gone
	_, ok = r.Get("test-provider")
	if ok {
		t.Fatal("expected test-provider to be unregistered")
	}
}

// testProvider is a minimal provider implementation for testing.
type testProvider struct {
	name string
}

func (t *testProvider) Name() string {
	return t.name
}

func (t *testProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	return nil
}

func (t *testProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

func TestRegistry_Unregister_NonExistent(t *testing.T) {
	r := NewRegistry()

	// Unregistering a non-existent provider should not panic
	r.Unregister("nonexistent-provider")

	// Verify no error occurred (this is a no-op)
	providers := r.ListProviders()
	for _, p := range providers {
		if p == "nonexistent-provider" {
			t.Fatal("nonexistent-provider should not appear in provider list")
		}
	}
}
