package fuzzy

import "strings"

// Match performs fuzzy subsequence matching of pattern against target.
// Returns score (higher is better), whether it matched, and the indices in target that matched.
//
// Scoring rules:
//   - Base: 10 points per matched character
//   - Start-of-string bonus: +50 if first pattern char matches target[0]
//   - Prefix bonus: +100 if all chars match consecutively from start
//   - Consecutive bonus: +15 per consecutive match (after first in run)
//   - Word boundary bonus: +25 if match is at word boundary (after /, -, _, space)
//   - Span penalty: -1 per character gap between first and last match
func Match(pattern, target string) (score int, matched bool, positions []int) {
	if pattern == "" {
		return 0, true, nil
	}

	pattern = strings.ToLower(pattern)
	target = strings.ToLower(target)

	positions = findBestPositions(pattern, target)
	if positions == nil {
		return 0, false, nil
	}

	score = calcScore(pattern, target, positions)
	return score, true, positions
}

func findBestPositions(pattern, target string) []int {
	positions := make([]int, 0, len(pattern))
	prevIdx := -1

	for _, pc := range pattern {
		found := false
		for i := prevIdx + 1; i < len(target); i++ {
			if target[i] == byte(pc) {
				positions = append(positions, i)
				prevIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	return positions
}

func calcScore(pattern, target string, positions []int) int {
	score := len(pattern) * 10

	if positions[0] == 0 {
		score += 50
	}

	if positions[0] > 0 && isWordBoundary(target, positions[0]) {
		score += 25
	}

	for i := 1; i < len(positions); i++ {
		if positions[i] == positions[i-1]+1 {
			score += 15
		}

		if isWordBoundary(target, positions[i]) {
			score += 25
		}
	}

	isPrefix := positions[0] == 0
	for i := 1; i < len(positions); i++ {
		if positions[i] != positions[i-1]+1 {
			isPrefix = false
			break
		}
	}
	if isPrefix {
		score += 100
	}

	span := positions[len(positions)-1] - positions[0] + 1
	score -= (span - len(pattern))
	if score < 0 {
		score = 0
	}

	return score
}

func isWordBoundary(target string, pos int) bool {
	if pos == 0 {
		return true
	}
	prev := target[pos-1]
	return prev == '/' || prev == '-' || prev == '_' || prev == ' '
}
