package search

import (
	"path/filepath"
	"strings"
)

const (
	segmentRankExact = iota
	segmentRankExactBase
	segmentRankPrefix
	segmentRankSubstring
	segmentRankNone
)

const (
	finalSegmentBonus       = 0.85
	segmentDepthPenaltyStep = 0.22
	crossSegmentPenalty     = 0.45
)

func computeSegmentBoost(query, relPath string, details MatchDetails) float64 {
	query = strings.TrimSpace(query)
	if query == "" || relPath == "" {
		return 0
	}

	normalized := filepath.ToSlash(relPath)
	segments := strings.FieldsFunc(normalized, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return 0
	}

	queryLower := strings.ToLower(query)
	tokens := strings.Fields(queryLower)
	if len(tokens) == 0 {
		tokens = []string{queryLower}
	}

	bestRank := segmentRankNone
	bestDepth := len(segments) + 1
	bestIsFinal := false
	found := false

	for _, token := range tokens {
		if token == "" {
			continue
		}
		rank, depth, isFinal, ok := bestSegmentForToken(token, segments)
		if !ok {
			continue
		}
		found = true
		if rank < bestRank || (rank == bestRank && depth < bestDepth) {
			bestRank = rank
			bestDepth = depth
			bestIsFinal = isFinal
		}
	}

	if !found || bestRank == segmentRankNone {
		if matchCrossesSegments(normalized, details) {
			return -0.25
		}
		return 0
	}

	boost := segmentRankBaseBoost(bestRank)

	if bestIsFinal {
		boost += finalSegmentBonus
	}

	if bestDepth >= 0 && bestDepth < len(segments)-1 {
		depthPenalty := float64((len(segments)-1)-bestDepth) * segmentDepthPenaltyStep
		boost -= depthPenalty
	}

	if matchCrossesSegments(normalized, details) {
		boost -= crossSegmentPenalty
	}

	if boost < 0 {
		boost = 0
	}
	return boost
}

func segmentRankBaseBoost(rank int) float64 {
	switch rank {
	case segmentRankExact:
		return 2.6
	case segmentRankExactBase:
		return 2.1
	case segmentRankPrefix:
		return 1.25
	case segmentRankSubstring:
		return 0.2
	default:
		return 0
	}
}

func matchCrossesSegments(path string, details MatchDetails) bool {
	if details.Start < 0 || details.End < details.Start {
		return false
	}
	runes := []rune(path)
	end := details.End
	if end >= len(runes) {
		end = len(runes) - 1
	}
	if details.Start >= len(runes) || details.Start < 0 || end < 0 {
		return false
	}

	for i := details.Start; i <= end && i < len(runes); i++ {
		if runes[i] == '/' {
			return true
		}
	}
	return false
}

func countPathSegments(path string) int {
	if path == "" || path == "." {
		return 1
	}
	normalized := filepath.ToSlash(path)
	normalized = strings.Trim(normalized, "/")
	if normalized == "" {
		return 1
	}
	return strings.Count(normalized, "/") + 1
}

func bestSegmentForToken(token string, segments []string) (rank int, depth int, isFinal bool, ok bool) {
	bestRank := segmentRankNone
	bestDepth := len(segments) + 1
	bestIsFinal := false

	for idx, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}

		lower := strings.ToLower(seg)
		baseLower := lower
		if dot := strings.LastIndex(lower, "."); dot > 0 {
			baseLower = lower[:dot]
		}

		var current int
		switch {
		case lower == token:
			current = segmentRankExact
		case baseLower == token:
			current = segmentRankExactBase
		case strings.HasPrefix(lower, token):
			current = segmentRankPrefix
		case strings.Contains(lower, token):
			current = segmentRankSubstring
		default:
			continue
		}

		if current < bestRank || (current == bestRank && idx < bestDepth) {
			bestRank = current
			bestDepth = idx
			bestIsFinal = idx == len(segments)-1
			if current == segmentRankExact && bestIsFinal {
				break
			}
		}
	}

	if bestRank == segmentRankNone {
		return 0, 0, false, false
	}
	return bestRank, bestDepth, bestIsFinal, true
}
