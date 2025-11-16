package search

import "testing"

func TestMatchTokensAllowsNonContiguousMultiTokenQueries(t *testing.T) {
	gs := &GlobalSearcher{
		matcher: NewFuzzyMatcher(),
	}

	query := "fcl dsp"
	tokens, matchAll := prepareQueryTokens(query, false)
	if matchAll {
		t.Fatalf("expected tokens for query %q", query)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	path := "root/project/docs/DSP/html/ftv2cl.png"
	score, matched, _ := gs.matchTokens(tokens, path, false, matchAll, spanFull)
	if !matched {
		t.Fatalf("expected query %q to match %q", query, path)
	}
	if score <= 0 {
		t.Fatalf("expected positive score, got %.4f", score)
	}
}

func TestMatchTokensPropagatesMatcherSpans(t *testing.T) {
	gs := &GlobalSearcher{
		matcher: NewFuzzyMatcher(),
	}

	query := "readme new"
	tokens, matchAll := prepareQueryTokens(query, false)
	if matchAll {
		t.Fatalf("expected tokens for query %q", query)
	}

	path := "third_party/newlib-cygwin/newlib/README"
	_, matched, details := gs.matchTokens(tokens, path, false, matchAll, spanFull)
	if !matched {
		t.Fatalf("expected query %q to match %q", query, path)
	}
	if len(details.Spans) == 0 {
		t.Fatalf("expected spans for query %q", query)
	}

	pathRunes := []rune(path)
	foundReadme := false
	for _, span := range details.Spans {
		if span.Start < 0 || span.End >= len(pathRunes) {
			t.Fatalf("span %+v out of range for path %q", span, path)
		}
		sub := string(pathRunes[span.Start : span.End+1])
		if sub == "README" {
			foundReadme = true
			break
		}
	}

	if !foundReadme {
		t.Fatalf("expected one of the spans to cover README, got %+v", details.Spans)
	}
}

func TestMatchTokensSpanPositionsAggregatesPositions(t *testing.T) {
	gs := &GlobalSearcher{
		matcher: NewFuzzyMatcher(),
	}
	tokens := []queryToken{
		{pattern: "ab", runes: []rune("ab")},
		{pattern: "cd", runes: []rune("cd")},
	}

	_, matched, details := gs.matchTokens(tokens, "ab_cd", false, false, spanPositions)
	if !matched {
		t.Fatalf("expected match for ab_cd")
	}
	if len(details.Spans) != 0 {
		t.Fatalf("expected spans to be empty for spanPositions, got %+v", details.Spans)
	}
	if len(details.Positions) != 4 {
		t.Fatalf("expected 4 positions, got %d", len(details.Positions))
	}

	spans := MergeMatchSpans(makeMatchSpansFromPositions(details.Positions))
	if len(spans) != 2 || spans[0].Start != 0 || spans[0].End != 1 || spans[1].Start != 3 || spans[1].End != 4 {
		t.Fatalf("unexpected spans from positions: %+v", spans)
	}
	releasePositions(details.Positions)
}

func TestMatchTokensSpanNoneSkipsSpansAndPositions(t *testing.T) {
	gs := &GlobalSearcher{
		matcher: NewFuzzyMatcher(),
	}
	tokens := []queryToken{
		{pattern: "main", runes: []rune("main")},
	}

	_, matched, details := gs.matchTokens(tokens, "src/main.go", false, false, spanNone)
	if !matched {
		t.Fatalf("expected match for src/main.go")
	}
	if len(details.Spans) != 0 {
		t.Fatalf("expected no spans for spanNone, got %+v", details.Spans)
	}
	if len(details.Positions) != 0 {
		t.Fatalf("expected no positions for spanNone, got %d", len(details.Positions))
	}
}
