package provider

import (
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestModelRegistry_FindExact(t *testing.T) {
	r := NewModelRegistry()

	tests := []string{
		"gpt-4o",
		"claude-sonnet-4-6",
		"gemini-2.5-pro",
	}

	for _, id := range tests {
		t.Run(id, func(t *testing.T) {
			m, err := r.Find(id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.ID != id {
				t.Fatalf("expected ID %q, got %q", id, m.ID)
			}
		})
	}
}

func TestModelRegistry_FindPattern(t *testing.T) {
	r := NewModelRegistry()

	tests := []struct {
		pattern  string
		expectID string
	}{
		{"gpt-4o", "gpt-4o"},
		{"claude-sonnet-4", "claude-sonnet-4-6"},
		{"gemini-2.5-pro", "gemini-2.5-pro"},
		{"Gemini", "gemini-2.5-pro"},
		{"nonexistent", ""},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			m, err := r.Find(tc.pattern)
			if tc.expectID == "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.ID != tc.expectID {
				t.Fatalf("expected ID %q, got %q", tc.expectID, m.ID)
			}
		})
	}
}

func TestModelRegistry_Get(t *testing.T) {
	r := NewModelRegistry()

	m, err := r.Get("gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", m.ID)
	}

	_, err = r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}

func TestModelRegistry_ListAll(t *testing.T) {
	r := NewModelRegistry()
	models := r.ListAll()

	// Minimal built-in set: 1 per provider (openai, anthropic, google)
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
}

func TestModelRegistry_ListByProvider(t *testing.T) {
	r := NewModelRegistry()

	openAI := r.ListByProvider("openai")
	if len(openAI) != 1 {
		t.Fatalf("expected 1 OpenAI model, got %d", len(openAI))
	}

	anthropic := r.ListByProvider("anthropic")
	if len(anthropic) != 1 {
		t.Fatalf("expected 1 Anthropic model, got %d", len(anthropic))
	}

	google := r.ListByProvider("google")
	if len(google) != 1 {
		t.Fatalf("expected 1 Google model, got %d", len(google))
	}

	none := r.ListByProvider("unknown")
	if len(none) != 0 {
		t.Fatalf("expected 0 models for unknown provider, got %d", len(none))
	}
}

func TestModelRegistry_Register(t *testing.T) {
	r := NewModelRegistry()

	custom := types.Model{
		ID:       "custom-model",
		Name:     "Custom",
		Provider: "custom",
		API:      "openai-completions",
	}
	r.Register(custom)

	m, err := r.Get("custom-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "Custom" {
		t.Fatalf("expected Custom, got %s", m.Name)
	}
}

func TestModelRegistry_RemoveByProvider(t *testing.T) {
	r := NewModelRegistry()

	// Verify initial counts
	openAI := r.ListByProvider("openai")
	if len(openAI) != 1 {
		t.Fatalf("expected 1 OpenAI model initially, got %d", len(openAI))
	}

	anthropic := r.ListByProvider("anthropic")
	if len(anthropic) != 1 {
		t.Fatalf("expected 1 Anthropic model initially, got %d", len(anthropic))
	}

	totalBefore := len(r.ListAll())

	// Remove all OpenAI models
	r.RemoveByProvider("openai")

	// Verify OpenAI models are gone
	openAIAfter := r.ListByProvider("openai")
	if len(openAIAfter) != 0 {
		t.Fatalf("expected 0 OpenAI models after removal, got %d", len(openAIAfter))
	}

	// Verify other providers are unaffected
	anthropicAfter := r.ListByProvider("anthropic")
	if len(anthropicAfter) != 1 {
		t.Fatalf("expected 1 Anthropic model after removal, got %d", len(anthropicAfter))
	}

	googleAfter := r.ListByProvider("google")
	if len(googleAfter) != 1 {
		t.Fatalf("expected 1 Google model after removal, got %d", len(googleAfter))
	}

	// Verify total count decreased by 1
	totalAfter := len(r.ListAll())
	if totalAfter != totalBefore-1 {
		t.Fatalf("expected %d models after removal, got %d", totalBefore-1, totalAfter)
	}
}

func TestModelRegistry_RemoveByProvider_NonExistent(t *testing.T) {
	r := NewModelRegistry()

	// Removing models for non-existent provider should not panic
	r.RemoveByProvider("nonexistent-provider")

	// Verify no error occurred (this is a no-op)
	totalAfter := len(r.ListAll())
	if totalAfter != 3 {
		t.Fatalf("expected 3 models after no-op removal, got %d", totalAfter)
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		ref          string
		wantProvider string
		wantModelID  string
	}{
		{"", "", ""},
		{"gpt-4o", "", "gpt-4o"},
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"openrouter/openai/gpt-4o", "openrouter", "openai/gpt-4o"},
		{"anthropic/claude-sonnet-4-20250514", "anthropic", "claude-sonnet-4-20250514"},
		{"/gpt-4o", "", "gpt-4o"},
	}

	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			prov, modelID := ParseModelRef(tc.ref)
			if prov != tc.wantProvider {
				t.Errorf("provider = %q, want %q", prov, tc.wantProvider)
			}
			if modelID != tc.wantModelID {
				t.Errorf("modelID = %q, want %q", modelID, tc.wantModelID)
			}
		})
	}
}

func TestModelRegistry_CompoundKeys(t *testing.T) {
	r := NewModelRegistry()

	// Register same model ID under different providers
	r.Register(types.Model{ID: "test-model", Provider: "provider-a", Name: "Test A"})
	r.Register(types.Model{ID: "test-model", Provider: "provider-b", Name: "Test B"})

	// Both should exist
	all := r.ListAll()
	if len(all) < 5 { // 3 built-in + 2 new
		t.Fatalf("expected at least 5 models, got %d", len(all))
	}

	// Find by compound key should work
	m, err := r.Find("provider-a/test-model")
	if err != nil {
		t.Fatalf("find provider-a/test-model: %v", err)
	}
	if m.Provider != "provider-a" {
		t.Errorf("expected provider-a, got %s", m.Provider)
	}

	m, err = r.Find("provider-b/test-model")
	if err != nil {
		t.Fatalf("find provider-b/test-model: %v", err)
	}
	if m.Provider != "provider-b" {
		t.Errorf("expected provider-b, got %s", m.Provider)
	}

	// Bare ID should be ambiguous
	_, err = r.Find("test-model")
	if err == nil {
		t.Fatal("expected error for ambiguous bare model ID")
	}
}

func TestModelRegistry_FindExactProviderModel(t *testing.T) {
	r := NewModelRegistry()

	// Register same model ID under different providers
	r.Register(types.Model{ID: "shared-model", Provider: "alpha", Name: "Alpha"})
	r.Register(types.Model{ID: "shared-model", Provider: "beta", Name: "Beta"})

	// Exact provider/modelID should resolve correctly
	m, err := r.Find("alpha/shared-model")
	if err != nil {
		t.Fatalf("find alpha/shared-model: %v", err)
	}
	if m.Provider != "alpha" {
		t.Errorf("expected alpha, got %s", m.Provider)
	}

	m, err = r.Find("beta/shared-model")
	if err != nil {
		t.Fatalf("find beta/shared-model: %v", err)
	}
	if m.Provider != "beta" {
		t.Errorf("expected beta, got %s", m.Provider)
	}
}
