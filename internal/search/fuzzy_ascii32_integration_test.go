package search

import (
	"math"
	"runtime"
	"strings"
	"testing"
)

// Test that the experimental ASCII DP32 path (when forced) produces identical
// results to the scalar DP. We force the ASCII32 path and disable the SIMD DP
// gate so the test deterministically hits matchRunesDPASCII32.
func TestASCII32EqualsScalarWhenForced(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}

	// Force ASCII32 path and disable SIMD DP gate.
	prevSIMD := fuzzySIMDDPDisabled
	prevForce := ascii32Force
	fuzzySIMDDPDisabled = true
	ascii32Force = true
	defer func() {
		fuzzySIMDDPDisabled = prevSIMD
		ascii32Force = prevForce
	}()

	fm := NewFuzzyMatcher()
	cases := []struct{ pattern, text string }{
		{"a", "a_b_c"},
		{"main", "main.go"},
		{"mgo", "module/main.go"},
		{"rd", "src/pkg/rdir/main.go"},
		{"abc", "a_b_c_d"},
		// Longer, ASCII-only, to exercise wider bands and multiple rows
		{"fmi", strings.Repeat("pkg/subpkg/", 12) + "fuzzy_matcher_internal.go"},
		{"rdirmain", strings.Repeat("src/example/", 16) + "rdir_cli_main.go"},
	}

	for _, tc := range cases {
		// Scalar baseline
		prunes, pbuf := acquireRunes(tc.pattern, true)
		trunes, tbuf := acquireRunes(tc.text, true)
		asciiText, asciiTextBuf := runeSliceToASCIIBytes(trunes)
		asciiPattern, asciiPatternBuf := runeSliceToASCIIBytes(prunes)
		boundary := acquireBoundaryBuffer(len(trunes))
		sScore, sMatched, sStart, sEnd, sLen, sCount, sHits := fm.matchRunesDPScalar(prunes, trunes, boundary, asciiText, asciiPattern)
		releaseBoundaryBuffer(boundary)

		// ASCII32 path via matchRunesDP (SIMD gate disabled, ASCII32 forced)
		boundary2 := acquireBoundaryBuffer(len(trunes))
		aScore, aMatched, aStart, aEnd, aLen, aCount, aHits := fm.matchRunesDP(prunes, trunes, boundary2, asciiText, asciiPattern)
		releaseBoundaryBuffer(boundary2)

		releaseByteBuffer(asciiPatternBuf)
		releaseByteBuffer(asciiTextBuf)
		releaseRunes(pbuf)
		releaseRunes(tbuf)

		if sMatched != aMatched {
			t.Fatalf("case %q/%q: matched mismatch (scalar=%v ascii32=%v)", tc.pattern, tc.text, sMatched, aMatched)
		}
		if !sMatched {
			continue
		}
		if math.Abs(sScore-aScore) > 1e-6 || sStart != aStart || sEnd != aEnd || sLen != aLen || sCount != aCount || sHits != aHits {
			t.Fatalf("case %q/%q: metadata mismatch\n scalar: score=%f start=%d end=%d len=%d count=%d hits=%d\n ascii32: score=%f start=%d end=%d len=%d count=%d hits=%d",
				tc.pattern, tc.text,
				sScore, sStart, sEnd, sLen, sCount, sHits,
				aScore, aStart, aEnd, aLen, aCount, aHits,
			)
		}
	}
}
