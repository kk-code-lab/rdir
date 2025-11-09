package search

import (
	"sort"
	"testing"
)

func TestCompareResultsOrdering(t *testing.T) {
	results := []GlobalSearchResult{
		{
			FilePath:     "span-long",
			Score:        3.0,
			MatchStart:   5,
			MatchEnd:     15,
			MatchCount:   3,
			WordHits:     1,
			PathSegments: 4,
			PathLength:   100,
			InputOrder:   30,
			HasMatch:     true,
		},
		{
			FilePath:     "stable-2",
			Score:        1.0,
			MatchStart:   3,
			MatchEnd:     6,
			MatchCount:   2,
			WordHits:     1,
			PathSegments: 6,
			PathLength:   120,
			InputOrder:   2,
			HasMatch:     true,
		},
		{
			FilePath:     "word-hits",
			Score:        3.0,
			MatchStart:   8,
			MatchEnd:     9,
			MatchCount:   4,
			WordHits:     3,
			PathSegments: 3,
			PathLength:   80,
			InputOrder:   15,
			HasMatch:     true,
		},
		{
			FilePath:     "no-match",
			Score:        0.2,
			MatchStart:   -1,
			MatchEnd:     -1,
			PathSegments: 2,
			PathLength:   70,
			InputOrder:   50,
			HasMatch:     false,
		},
		{
			FilePath:     "span-short",
			Score:        3.0,
			MatchStart:   10,
			MatchEnd:     11,
			MatchCount:   3,
			WordHits:     1,
			PathSegments: 4,
			PathLength:   60,
			InputOrder:   10,
			HasMatch:     true,
		},
		{
			FilePath:     "match-count",
			Score:        3.0,
			MatchStart:   10,
			MatchEnd:     11,
			MatchCount:   6,
			WordHits:     1,
			PathSegments: 4,
			PathLength:   60,
			InputOrder:   9,
			HasMatch:     true,
		},
		{
			FilePath:     "score-high",
			Score:        5.0,
			MatchStart:   4,
			MatchEnd:     6,
			MatchCount:   4,
			WordHits:     2,
			PathSegments: 2,
			PathLength:   40,
			InputOrder:   1,
			HasMatch:     true,
		},
		{
			FilePath:     "stable-1",
			Score:        1.0,
			MatchStart:   3,
			MatchEnd:     6,
			MatchCount:   2,
			WordHits:     1,
			PathSegments: 6,
			PathLength:   120,
			InputOrder:   1,
			HasMatch:     true,
		},
	}

	sort.Slice(results, func(i, j int) bool {
		return compareResults(results[i], results[j]) < 0
	})

	want := []string{
		"score-high",  // higher score first
		"match-count", // same score/span, more matched chars
		"word-hits",   // more word hits after match count tie-break
		"span-short",  // shorter span beats longer span
		"span-long",
		"stable-1", // identical metrics ordered by InputOrder
		"stable-2",
		"no-match", // items without matches sorted last
	}

	if len(results) != len(want) {
		t.Fatalf("unexpected result count: got %d want %d", len(results), len(want))
	}

	got := make([]string, len(results))
	for i, res := range results {
		got[i] = res.FilePath
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got order %v want %v", got, want)
		}
	}
}

func TestMergeResultsMaintainsOrdering(t *testing.T) {
	left := []GlobalSearchResult{
		{FilePath: "L1", Score: 5.0, MatchStart: 1, MatchEnd: 3, MatchCount: 3, WordHits: 2, PathSegments: 2, PathLength: 20, HasMatch: true},
		{FilePath: "L2", Score: 3.0, MatchStart: 2, MatchEnd: 4, MatchCount: 2, WordHits: 1, PathSegments: 3, PathLength: 30, HasMatch: true},
	}
	right := []GlobalSearchResult{
		{FilePath: "R1", Score: 4.0, MatchStart: 1, MatchEnd: 2, MatchCount: 2, WordHits: 2, PathSegments: 2, PathLength: 25, HasMatch: true},
		{FilePath: "R2", Score: 1.0, MatchStart: -1, MatchEnd: -1, PathSegments: 5, PathLength: 50, HasMatch: false},
	}

	merged := mergeResults(left, right)

	want := []string{"L1", "R1", "L2", "R2"}
	if len(merged) != len(want) {
		t.Fatalf("unexpected merged count: got %d want %d", len(merged), len(want))
	}

	for i, res := range merged {
		if res.FilePath != want[i] {
			t.Fatalf("merged[%d] = %q, want %q", i, res.FilePath, want[i])
		}
	}

	releaseResultBuffer(merged)
}
