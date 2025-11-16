package search

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// Benchmark the index pipeline with different span strategies to capture the
// cost of building spans eagerly vs. carrying positions vs. rerunning for spans.
func BenchmarkIndexLazySpans(b *testing.B) {
	benchcases := []struct {
		name     string
		spanMode spanRequest
	}{
		{name: "SpanFull", spanMode: spanFull},
		{name: "SpanPositions", spanMode: spanPositions},
		{name: "SpanNoneRerun", spanMode: spanNone},
	}

	for _, bc := range benchcases {
		bc := bc
		b.Run(bc.name, func(b *testing.B) {
			benchmarkIndexSpanMode(b, bc.spanMode)
		})
	}
}

func benchmarkIndexSpanMode(b *testing.B, spanMode spanRequest) {
	gs := &GlobalSearcher{matcher: NewFuzzyMatcher()}
	query := "rdir main"
	tokens, matchAll := prepareQueryTokens(query, false)
	if matchAll {
		b.Fatal("benchmark query unexpectedly matches all")
	}
	gs.orderTokens(tokens)

	const totalEntries = 2048
	const topK = 128

	entries := make([]indexedEntry, totalEntries)
	for i := range entries {
		rel := fmt.Sprintf("src/example/%04d/rdir_main_%04d.go", i/8, i)
		entries[i] = indexedEntry{
			fullPath:  rel,
			relPath:   rel,
			lowerPath: rel,
			order:     i,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector := newTopCollector(topK)
		for idx := range entries {
			entry := &entries[idx]
			relPath := entry.relPath
			score, matched, details := gs.matchTokens(tokens, relPath, false, false, spanMode)
			if !matched {
				releasePositions(details.Positions)
				continue
			}

			score += computeSegmentBoost(query, relPath, details)

			pathLength := details.TargetLength
			if pathLength == 0 {
				pathLength = utf8.RuneCountInString(relPath)
			}
			pathSegments := countPathSegments(relPath)

			if !collector.Needs(score, pathLength, details.Start, details.End, details.MatchCount, details.WordHits, pathSegments, entry.order, true) {
				releasePositions(details.Positions)
				continue
			}

			finalDetails := gs.materializeIndexDetails(spanMode, tokens, relPath, false, false, details)
			result := gs.makeIndexedResult(entry, score, pathLength, finalDetails.Start, finalDetails.End, finalDetails.MatchCount, finalDetails.WordHits, pathSegments, true, finalDetails.Spans)
			collector.Store(result)
		}
		results := collector.Results()
		if len(results) == 0 {
			b.Fatal("collector returned no results")
		}
	}
}
