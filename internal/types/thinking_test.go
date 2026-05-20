package types

import "testing"

func TestGetSupportedThinkingLevels(t *testing.T) {
	t.Run("non-reasoning model", func(t *testing.T) {
		m := Model{Reasoning: false}
		levels := m.GetSupportedThinkingLevels()
		if len(levels) != 1 || levels[0] != ThinkingOff {
			t.Errorf("expected [off], got %v", levels)
		}
	})

	t.Run("reasoning model with no map", func(t *testing.T) {
		m := Model{Reasoning: true}
		levels := m.GetSupportedThinkingLevels()
		expected := []ThinkingLevel{ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh}
		if len(levels) != len(expected) {
			t.Fatalf("expected %d levels, got %d", len(expected), len(levels))
		}
		for i, l := range expected {
			if levels[i] != l {
				t.Errorf("level[%d] = %s, want %s", i, levels[i], l)
			}
		}
	})

	t.Run("xhigh explicitly supported", func(t *testing.T) {
		m := Model{
			Reasoning: true,
			ThinkingLevelMap: map[string]string{
				"xhigh": "xhigh",
			},
		}
		levels := m.GetSupportedThinkingLevels()
		found := false
		for _, l := range levels {
			if l == ThinkingXHigh {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected xhigh to be supported")
		}
	})

	t.Run("xhigh not supported", func(t *testing.T) {
		m := Model{Reasoning: true}
		levels := m.GetSupportedThinkingLevels()
		for _, l := range levels {
			if l == ThinkingXHigh {
				t.Error("xhigh should not be supported without explicit mapping")
			}
		}
	})

	t.Run("minimal explicitly disabled", func(t *testing.T) {
		m := Model{
			Reasoning: true,
			ThinkingLevelMap: map[string]string{
				"minimal": "",
			},
		}
		levels := m.GetSupportedThinkingLevels()
		for _, l := range levels {
			if l == ThinkingMinimal {
				t.Error("minimal should be disabled")
			}
		}
	})
}

func TestMapThinkingLevel(t *testing.T) {
	t.Run("no map returns level as-is", func(t *testing.T) {
		m := Model{Reasoning: true}
		got := m.MapThinkingLevel(ThinkingHigh)
		if got != "high" {
			t.Errorf("expected 'high', got %q", got)
		}
	})

	t.Run("map remaps level", func(t *testing.T) {
		m := Model{
			Reasoning: true,
			ThinkingLevelMap: map[string]string{
				"xhigh": "max",
			},
		}
		got := m.MapThinkingLevel(ThinkingXHigh)
		if got != "max" {
			t.Errorf("expected 'max', got %q", got)
		}
	})

	t.Run("missing key returns level as-is", func(t *testing.T) {
		m := Model{
			Reasoning: true,
			ThinkingLevelMap: map[string]string{
				"high": "deep",
			},
		}
		got := m.MapThinkingLevel(ThinkingLow)
		if got != "low" {
			t.Errorf("expected 'low', got %q", got)
		}
	})
}

func TestAllThinkingLevels(t *testing.T) {
	levels := AllThinkingLevels()
	expected := []ThinkingLevel{ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh}
	if len(levels) != len(expected) {
		t.Fatalf("expected %d levels, got %d", len(expected), len(levels))
	}
	for i, l := range expected {
		if levels[i] != l {
			t.Errorf("level[%d] = %s, want %s", i, levels[i], l)
		}
	}
}

func TestThinkingLevelDescription(t *testing.T) {
	tests := []struct {
		level ThinkingLevel
		want  string
	}{
		{ThinkingOff, "No reasoning"},
		{ThinkingMinimal, "Very brief reasoning (~1k tokens)"},
		{ThinkingLow, "Light reasoning (~2k tokens)"},
		{ThinkingMedium, "Moderate reasoning (~4k tokens)"},
		{ThinkingHigh, "Deep reasoning (~8k tokens)"},
		{ThinkingXHigh, "Maximum reasoning (~16k tokens)"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			got := ThinkingLevelDescription(tt.level)
			if got != tt.want {
				t.Errorf("ThinkingLevelDescription(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}
