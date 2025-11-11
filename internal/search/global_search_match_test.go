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
	score, matched, _ := gs.matchTokens(tokens, path, false, matchAll)
	if !matched {
		t.Fatalf("expected query %q to match %q", query, path)
	}
	if score <= 0 {
		t.Fatalf("expected positive score, got %.4f", score)
	}
}

func TestExtractTokenSpansSplitsNonContiguousMatches(t *testing.T) {
	text := []rune("modules")
	pattern := []rune("ml")
	spans := extractTokenSpans(pattern, text)
	if len(spans) != 2 {
		t.Fatalf("expected two spans for letters m and o, got %v", spans)
	}
	if spans[0].Start != 0 || spans[0].End != 0 {
		t.Fatalf("unexpected first span %+v", spans[0])
	}
	if spans[1].Start != 4 || spans[1].End != 4 {
		t.Fatalf("unexpected second span %+v", spans[1])
	}
}
