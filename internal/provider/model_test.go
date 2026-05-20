package provider

import (
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestModelRegistry_FindExact(t *testing.T) {
	r := NewModelRegistry()

	tests := []string{
		"gpt-4o",
		"gpt-4o-mini",
		"o1",
		"o3",
		"claude-sonnet-4-20250514",
		"claude-3-7-sonnet-20250219",
		"claude-3-5-sonnet-20241022",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.0-flash",
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
		{"gpt-4o", "gpt-4o"},           // exact match
		{"claude-sonnet-4", "claude-sonnet-4-20250514"}, // substring
		{"gemini-2.5-pro", "gemini-2.5-pro"}, // exact
		{"claude-3-5-sonnet", "claude-3-5-sonnet-20241022"}, // substring
		{"Gemini", ""},                   // multiple matches
		{"nonexistent", ""},              // no match
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

	// We defined 10 built-in models
	if len(models) != 10 {
		t.Fatalf("expected 10 models, got %d", len(models))
	}
}

func TestModelRegistry_ListByProvider(t *testing.T) {
	r := NewModelRegistry()

	openAI := r.ListByProvider("openai")
	if len(openAI) != 4 {
		t.Fatalf("expected 4 OpenAI models, got %d", len(openAI))
	}

	anthropic := r.ListByProvider("anthropic")
	if len(anthropic) != 3 {
		t.Fatalf("expected 3 Anthropic models, got %d", len(anthropic))
	}

	google := r.ListByProvider("google")
	if len(google) != 3 {
		t.Fatalf("expected 3 Google models, got %d", len(google))
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
	if len(openAI) != 4 {
		t.Fatalf("expected 4 OpenAI models initially, got %d", len(openAI))
	}

	anthropic := r.ListByProvider("anthropic")
	if len(anthropic) != 3 {
		t.Fatalf("expected 3 Anthropic models initially, got %d", len(anthropic))
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
	if len(anthropicAfter) != 3 {
		t.Fatalf("expected 3 Anthropic models after removal, got %d", len(anthropicAfter))
	}

	googleAfter := r.ListByProvider("google")
	if len(googleAfter) != 3 {
		t.Fatalf("expected 3 Google models after removal, got %d", len(googleAfter))
	}

	// Verify total count decreased by 4
	totalAfter := len(r.ListAll())
	if totalAfter != totalBefore-4 {
		t.Fatalf("expected %d models after removal, got %d", totalBefore-4, totalAfter)
	}
}

func TestModelRegistry_RemoveByProvider_NonExistent(t *testing.T) {
	r := NewModelRegistry()

	// Removing models for non-existent provider should not panic
	r.RemoveByProvider("nonexistent-provider")

	// Verify no error occurred (this is a no-op)
	totalAfter := len(r.ListAll())
	if totalAfter != 10 {
		t.Fatalf("expected 10 models after no-op removal, got %d", totalAfter)
	}
}
