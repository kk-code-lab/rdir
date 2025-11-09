package search

import "testing"

func TestComputeSegmentBoostPrefersExactSegments(t *testing.T) {
	fm := NewFuzzyMatcher()
	query := "orion"

	exactPath := "workspace/apps/ORION/README.md"
	exactScore, matchedExact, exactDetails := fm.MatchDetailed(query, exactPath)
	if !matchedExact {
		t.Fatalf("expected %q to match %q", query, exactPath)
	}

	substringPath := "third_party/tooling/kits/FreeRTOS/docs/README-with-orion-reference.txt"
	subScore, matchedSub, subDetails := fm.MatchDetailed(query, substringPath)
	if !matchedSub {
		t.Fatalf("expected %q to match %q", query, substringPath)
	}

	exactBoost := computeSegmentBoost(query, exactPath, exactDetails)
	subBoost := computeSegmentBoost(query, substringPath, subDetails)

	if exactScore+exactBoost <= subScore+subBoost {
		t.Fatalf("expected exact segment (%.3f) to beat substring (%.3f)", exactScore+exactBoost, subScore+subBoost)
	}
}

func TestComputeSegmentBoostEmptyQuery(t *testing.T) {
	var details MatchDetails
	if boost := computeSegmentBoost("", "foo/bar", details); boost != 0 {
		t.Fatalf("expected empty query boost=0, got %.3f", boost)
	}
}

func TestComputeSegmentBoostCrossSegmentPenalty(t *testing.T) {
	details := MatchDetails{Start: 0, End: 3}
	boost := computeSegmentBoost("abc", "a/bc", details)
	if boost >= 0 {
		t.Fatalf("expected cross-segment fuzzy match to be penalized, got %.3f", boost)
	}
}
