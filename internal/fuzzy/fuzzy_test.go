package fuzzy

import (
	"slices"
	"testing"
)

func TestMatch_NoMatch(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
	}{
		{"bogus", "session"},
		{"xyz", "help"},
		{"abc", "quit"},
		{"z", "help"},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.target, func(t *testing.T) {
			score, matched, positions := Match(tt.pattern, tt.target)
			if matched {
				t.Fatalf("expected no match for pattern=%q target=%q", tt.pattern, tt.target)
			}
			if score != 0 {
				t.Fatalf("expected score 0, got %d", score)
			}
			if positions != nil {
				t.Fatalf("expected nil positions, got %v", positions)
			}
		})
	}
}

func TestMatch_EmptyPattern(t *testing.T) {
	score, matched, positions := Match("", "session")
	if !matched {
		t.Fatal("empty pattern should match")
	}
	if score != 0 {
		t.Fatalf("expected score 0 for empty pattern, got %d", score)
	}
	if positions != nil {
		t.Fatalf("expected nil positions for empty pattern, got %v", positions)
	}
}

func TestMatch_ExactMatch(t *testing.T) {
	score, matched, positions := Match("session", "session")
	if !matched {
		t.Fatal("exact match should succeed")
	}
	if len(positions) != 7 {
		t.Fatalf("expected 7 positions, got %d", len(positions))
	}
	expectedScore := 7*10 + 50 + 6*15 + 100
	if score != expectedScore {
		t.Fatalf("expected score %d, got %d", expectedScore, score)
	}
}

func TestMatch_PrefixMatch(t *testing.T) {
	score, matched, positions := Match("ses", "session")
	if !matched {
		t.Fatal("prefix match should succeed")
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(positions))
	}
	expected := []int{0, 1, 2}
	if !slices.Equal(positions, expected) {
		t.Fatalf("expected positions %v, got %v", expected, positions)
	}
	expectedScore := 3*10 + 50 + 2*15 + 100
	if score != expectedScore {
		t.Fatalf("expected score %d, got %d", expectedScore, score)
	}
}

func TestMatch_NonPrefixConsecutive(t *testing.T) {
	score, matched, positions := Match("sion", "session")
	if !matched {
		t.Fatal("should match")
	}
	expected := []int{0, 4, 5, 6}
	if !slices.Equal(positions, expected) {
		t.Fatalf("expected positions %v, got %v", expected, positions)
	}
	expectedScore := 4*10 + 50 + 2*15 - 3
	if score != expectedScore {
		t.Fatalf("expected score %d, got %d", expectedScore, score)
	}
}

func TestMatch_SparseMatch(t *testing.T) {
	score, matched, positions := Match("sn", "session")
	if !matched {
		t.Fatal("should match")
	}
	expected := []int{0, 6}
	if !slices.Equal(positions, expected) {
		t.Fatalf("expected positions %v, got %v", expected, positions)
	}
	expectedScore := 2*10 + 50 - 5
	if score != expectedScore {
		t.Fatalf("expected score %d, got %d", expectedScore, score)
	}
}

func TestMatch_WordBoundary(t *testing.T) {
	score, matched, _ := Match("h", "/help")
	if !matched {
		t.Fatal("should match")
	}
	expectedScore := 10 + 25
	if score != expectedScore {
		t.Fatalf("expected word boundary score %d, got %d", expectedScore, score)
	}
}

func TestMatch_WordBoundaryStartOfString(t *testing.T) {
	score, matched, _ := Match("h", "help")
	if !matched {
		t.Fatal("should match")
	}
	expectedScore := 10 + 50 + 100
	if score != expectedScore {
		t.Fatalf("expected start-of-string score %d, got %d", expectedScore, score)
	}
}

func TestMatch_WordBoundaryDash(t *testing.T) {
	score, matched, _ := Match("b", "foo-bar")
	if !matched {
		t.Fatal("should match")
	}
	expectedScore := 10 + 25
	if score != expectedScore {
		t.Fatalf("expected word boundary score %d, got %d", expectedScore, score)
	}
}

func TestMatch_CaseInsensitive(t *testing.T) {
	score1, matched1, pos1 := Match("SES", "session")
	score2, matched2, pos2 := Match("ses", "session")

	if !matched1 || !matched2 {
		t.Fatal("both should match")
	}
	if score1 != score2 {
		t.Fatalf("case should not affect score: %d vs %d", score1, score2)
	}
	if !slices.Equal(pos1, pos2) {
		t.Fatalf("case should not affect positions: %v vs %v", pos1, pos2)
	}
}

func TestMatch_ScoringOrder(t *testing.T) {
	prefixScore, _, _ := Match("ses", "session")
	consecutiveScore, _, _ := Match("sion", "session")
	sparseScore, _, _ := Match("sn", "session")

	if prefixScore <= consecutiveScore {
		t.Fatalf("prefix score (%d) should be > consecutive score (%d)", prefixScore, consecutiveScore)
	}
	if consecutiveScore <= sparseScore {
		t.Fatalf("consecutive score (%d) should be > sparse score (%d)", consecutiveScore, sparseScore)
	}
}

func TestMatch_Positions(t *testing.T) {
	tests := []struct {
		pattern   string
		target    string
		positions []int
	}{
		{"h", "help", []int{0}},
		{"lp", "help", []int{2, 3}},
		{"ep", "help", []int{1, 3}},
		{"sk", "skills", []int{0, 1}},
		{"skill:", "skill:", []int{0, 1, 2, 3, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			_, matched, positions := Match(tt.pattern, tt.target)
			if !matched {
				t.Fatalf("expected match for pattern=%q target=%q", tt.pattern, tt.target)
			}
			if !slices.Equal(positions, tt.positions) {
				t.Fatalf("expected positions %v, got %v", tt.positions, positions)
			}
		})
	}
}

func TestMatch_ScoreNonNegative(t *testing.T) {
	score, matched, _ := Match("se", "session")
	if !matched {
		t.Fatal("should match")
	}
	if score < 0 {
		t.Fatalf("score should never be negative, got %d", score)
	}
}
