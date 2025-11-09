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
