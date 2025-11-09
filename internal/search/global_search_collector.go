package search

import (
	"container/heap"
	"math"
	"sort"
	"time"
)

const (
	resultScoreEpsilon = 1e-9
	maxIntValue        = int(^uint(0) >> 1)
)

type resultMinHeap []GlobalSearchResult

func (h resultMinHeap) Len() int           { return len(h) }
func (h resultMinHeap) Less(i, j int) bool { return compareResults(h[i], h[j]) > 0 }
func (h resultMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *resultMinHeap) Push(x any) {
	*h = append(*h, x.(GlobalSearchResult))
}

func (h *resultMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type topCollector struct {
	max  int
	minH resultMinHeap
}

func newTopCollector(max int) *topCollector {
	if max <= 0 {
		max = maxDisplayResults
	}
	tc := &topCollector{
		max:  max,
		minH: make(resultMinHeap, 0, max),
	}
	heap.Init(&tc.minH)
	return tc
}

func (tc *topCollector) Consider(res GlobalSearchResult) {
	tc.Store(res)
}

func (tc *topCollector) Needs(score float64, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments, order int, hasMatch bool) bool {
	if tc.max <= 0 {
		return false
	}
	if tc.minH.Len() < tc.max {
		return true
	}
	candidate := GlobalSearchResult{
		Score:        score,
		PathLength:   pathLength,
		MatchStart:   matchStart,
		MatchEnd:     matchEnd,
		MatchCount:   matchCount,
		WordHits:     wordHits,
		PathSegments: pathSegments,
		InputOrder:   order,
		HasMatch:     hasMatch,
	}
	return compareResults(candidate, tc.minH[0]) < 0
}

func (tc *topCollector) Store(res GlobalSearchResult) {
	if tc.max <= 0 {
		return
	}

	if tc.minH.Len() < tc.max {
		heap.Push(&tc.minH, res)
		return
	}

	if compareResults(res, tc.minH[0]) >= 0 {
		return
	}

	heap.Pop(&tc.minH)
	heap.Push(&tc.minH, res)
}

func (tc *topCollector) Results() []GlobalSearchResult {
	results := make([]GlobalSearchResult, tc.minH.Len())
	copy(results, tc.minH)

	sort.Slice(results, func(i, j int) bool {
		return compareResults(results[i], results[j]) < 0
	})

	return results
}

// mergeResults merges two sorted-by-score result slices and keeps the ordering stable.
func mergeResults(left, right []GlobalSearchResult) []GlobalSearchResult {
	result := borrowResultBuffer(len(left) + len(right))
	result = result[:0]
	i, j := 0, 0

	for i < len(left) && j < len(right) {
		if compareResults(left[i], right[j]) <= 0 {
			result = append(result, left[i])
			i++
		} else {
			result = append(result, right[j])
			j++
		}
	}

	result = append(result, left[i:]...)
	result = append(result, right[j:]...)

	return result
}

func shouldFlushBatch(lastSize, currentSize int, lastTime time.Time) bool {
	if currentSize <= lastSize {
		return false
	}
	delta := currentSize - lastSize
	if lastSize == 0 && currentSize >= initialImmediateBatchSize {
		return true
	}
	if delta >= batchForceSize {
		return true
	}

	interval := batchIntervalSlow
	if delta <= batchFastThreshold {
		interval = batchIntervalFast
	}
	return time.Since(lastTime) >= interval
}

func compareResults(a, b GlobalSearchResult) int {
	if diff := compareScore(a.Score, b.Score); diff != 0 {
		return diff
	}

	if a.HasMatch && b.HasMatch {
		if diff := compareMatchSpan(a.MatchStart, a.MatchEnd, b.MatchStart, b.MatchEnd); diff != 0 {
			return diff
		}
		if diff := compareIntDesc(a.MatchCount, b.MatchCount); diff != 0 {
			return diff
		}
		if diff := compareIntDesc(a.WordHits, b.WordHits); diff != 0 {
			return diff
		}
		if diff := compareMatchIndexValues(a.MatchStart, a.PathLength, b.MatchStart, b.PathLength); diff != 0 {
			return diff
		}
		if diff := compareMatchIndexValues(a.MatchEnd, a.PathLength, b.MatchEnd, b.PathLength); diff != 0 {
			return diff
		}
		if diff := compareInt(a.PathSegments, b.PathSegments); diff != 0 {
			return diff
		}
		if diff := compareInt(a.PathLength, b.PathLength); diff != 0 {
			return diff
		}
	} else if a.HasMatch && !b.HasMatch {
		return -1
	} else if !a.HasMatch && b.HasMatch {
		return 1
	}

	return compareInt(a.InputOrder, b.InputOrder)
}

func compareScore(a, b float64) int {
	if math.Abs(a-b) <= resultScoreEpsilon {
		return 0
	}
	if a > b {
		return -1
	}
	return 1
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareIntDesc(a, b int) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

func compareMatchIndexValues(idxA, lenA, idxB, lenB int) int {
	normA := normalizeMatchIndex(idxA, lenA)
	normB := normalizeMatchIndex(idxB, lenB)
	switch {
	case normA < normB:
		return -1
	case normA > normB:
		return 1
	default:
		return 0
	}
}

func normalizeMatchIndex(idx, pathLength int) int {
	if idx >= 0 {
		return idx
	}
	if pathLength > 0 {
		return pathLength
	}
	return maxIntValue
}

func matchSpanLength(start, end int) int {
	if start < 0 || end < start {
		return maxIntValue
	}
	return end - start
}

func compareMatchSpan(startA, endA, startB, endB int) int {
	spanA := matchSpanLength(startA, endA)
	spanB := matchSpanLength(startB, endB)
	switch {
	case spanA < spanB:
		return -1
	case spanA > spanB:
		return 1
	default:
		return 0
	}
}
