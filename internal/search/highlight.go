package search

import "strings"

func MergeMatchSpans(spans []MatchSpan) []MatchSpan {
	if len(spans) == 0 {
		return nil
	}
	merged := make([]MatchSpan, 0, len(spans))
	current := spans[0]
	for i := 1; i < len(spans); i++ {
		next := spans[i]
		if next.Start <= current.End {
			if next.End > current.End {
				current.End = next.End
			}
			continue
		}
		merged = append(merged, current)
		current = next
	}
	merged = append(merged, current)
	return merged
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
