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

func computeSegmentBoost(query, relPath string, details MatchDetails) float64 {
	if query == "" || relPath == "" {
		return 0
	}

	normalized := filepath.ToSlash(relPath)
	segments := strings.FieldsFunc(normalized, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return 0
	}

	queryLower := strings.ToLower(query)
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

		var rank int
		switch {
		case lower == queryLower:
			rank = segmentRankExact
		case baseLower == queryLower:
			rank = segmentRankExactBase
		case strings.HasPrefix(lower, queryLower):
			rank = segmentRankPrefix
		case strings.Contains(lower, queryLower):
			rank = segmentRankSubstring
		default:
			continue
		}

		if rank < bestRank || (rank == bestRank && idx < bestDepth) {
			bestRank = rank
			bestDepth = idx
			bestIsFinal = idx == len(segments)-1

			if rank == segmentRankExact && bestIsFinal {
				break
			}
		}
	}

	if bestRank == segmentRankNone {
		if matchCrossesSegments(normalized, details) {
			return -0.25
		}
		return 0
	}

	boost := segmentRankBaseBoost(bestRank)

	if bestIsFinal {
		boost += 0.25
	}

	if bestDepth >= 0 && bestDepth < len(segments)-1 {
		boost += float64((len(segments)-1)-bestDepth) * 0.12
	}

	if matchCrossesSegments(normalized, details) {
		boost -= 0.35
	}

	if boost < 0 {
		return 0
	}
	return boost
}

func segmentRankBaseBoost(rank int) float64 {
	switch rank {
	case segmentRankExact:
		return 2.3
	case segmentRankExactBase:
		return 1.9
	case segmentRankPrefix:
		return 1.1
	case segmentRankSubstring:
		return 0.35
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
