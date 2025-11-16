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
	score, matched, _ := gs.matchTokens(tokens, path, false, matchAll, true)
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
	_, matched, details := gs.matchTokens(tokens, path, false, matchAll, true)
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
