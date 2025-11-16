package search

import (
	"math"
	"runtime"
	"testing"
)

func TestFuzzyPrefixMaxASCIIAsmMatchesScalar(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	dpPrev := []float32{4.5, 3.1, -2.0, 7.3, 5.5, -1.2, 6.8, 0.0, 2.4, -3.6}
	start := 0
	end := len(dpPrev) - 1
	gap := float32(0.18)

	prefixAsm := make([]float32, len(dpPrev))
	prefixIdxAsm := make([]int32, len(dpPrev))
	runPrefixASCII(prefixAsm, prefixIdxAsm, dpPrev, start, end, gap)

	prefixRef := make([]float32, len(dpPrev))
	prefixIdxRef := make([]int32, len(dpPrev))
	scalarPrefixRef(prefixRef, prefixIdxRef, dpPrev, start, end, gap)

	for i := start; i <= end; i++ {
		delta := math.Abs(float64(prefixAsm[i] - prefixRef[i]))
		if delta > 1e-6 {
			t.Fatalf("prefix mismatch at %d: got %f want %f", i, prefixAsm[i], prefixRef[i])
		}
		if prefixIdxAsm[i] != prefixIdxRef[i] {
			t.Fatalf("index mismatch at %d: got %d want %d", i, prefixIdxAsm[i], prefixIdxRef[i])
		}
	}
}

func TestFuzzyPrefixMaxASCIIAsmWindowOffset(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	dpPrev := []float32{-2.5, 4.0, 1.5, 3.3, -0.2, 5.1, 2.2, -4.8, 6.6}
	start := 2
	end := 6
	gap := float32(0.11)

	prefixAsm := make([]float32, len(dpPrev))
	prefixIdxAsm := make([]int32, len(dpPrev))
	runPrefixASCII(prefixAsm, prefixIdxAsm, dpPrev, start, end, gap)

	prefixRef := make([]float32, len(dpPrev))
	prefixIdxRef := make([]int32, len(dpPrev))
	scalarPrefixRef(prefixRef, prefixIdxRef, dpPrev, start, end, gap)

	for i := start; i <= end; i++ {
		delta := math.Abs(float64(prefixAsm[i] - prefixRef[i]))
		if delta > 1e-6 {
			t.Fatalf("offset prefix mismatch at %d: got %f want %f", i, prefixAsm[i], prefixRef[i])
		}
		if prefixIdxAsm[i] != prefixIdxRef[i] {
			t.Fatalf("offset index mismatch at %d: got %d want %d", i, prefixIdxAsm[i], prefixIdxRef[i])
		}
	}
}

func TestFuzzyPrefixMaxASCIIAsmCopiesInput(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	dpPrev := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
	prefix := make([]float32, len(dpPrev))
	prefixIdx := make([]int32, len(dpPrev))
	start := 1
	end := 3
	gap := float32(0.25)

	// Use Go wrapper to forward to asm; avoids test calling abi symbol directly
	// First, sanity: verify single-element asm write works at an index
	asmCopyOne(prefix, prefixIdx, 1)
	if prefix[1] != 1.0 || prefixIdx[1] != -1 {
		t.Fatalf("asmCopyOne failed to write at index 1: got prefix=%f idx=%d", prefix[1], prefixIdx[1])
	}

	// Then test the range copy path
	asmCopyPrefixRange(prefix, prefixIdx, dpPrev, start, end, gap)
	for i := start; i <= end; i++ {
		if prefix[i] != dpPrev[i] {
			t.Fatalf("prefix[%d] copy mismatch: got %f want %f", i, prefix[i], dpPrev[i])
		}
		if prefixIdx[i] != -1 {
			t.Fatalf("prefixIdx[%d] copy mismatch: got %d want -1", i, prefixIdx[i])
		}
	}
}

func TestAsmSetIdxRange(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	idx := make([]int32, 6)
	asmSetIdxRange(idx, 2, 4)
	for i := range idx {
		if i >= 2 && i <= 4 {
			if idx[i] != -1 {
				t.Fatalf("idx[%d]=%d; want -1", i, idx[i])
			}
		} else if idx[i] != 0 {
			t.Fatalf("idx[%d]=%d; want 0 outside range", i, idx[i])
		}
	}
}

func TestAsmSetIdxRangeUnrolled8(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	idx := make([]int32, 16)
	asmSetIdxRange(idx, 2, 9) // count = 8
	for i := range idx {
		if i >= 2 && i <= 9 {
			if idx[i] != -1 {
				t.Fatalf("idx[%d]=%d; want -1", i, idx[i])
			}
		} else if idx[i] != 0 {
			t.Fatalf("idx[%d]=%d; want 0 outside range", i, idx[i])
		}
	}
}

func TestAsmCopyRangeF32(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	src := []float32{10, 20, 30, 40, 50}
	dst := make([]float32, len(src))
	asmCopyRangeF32(dst, src, 1, 3)
	for i := range dst {
		if i >= 1 && i <= 3 {
			if dst[i] != src[i] {
				t.Fatalf("dst[%d]=%f; want %f", i, dst[i], src[i])
			}
		} else if dst[i] != 0 {
			t.Fatalf("dst[%d]=%f; want 0 outside range", i, dst[i])
		}
	}
}

func TestAsmCopyRangeF32Unrolled8(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only test")
	}
	src := []float32{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	dst := make([]float32, len(src))
	asmCopyRangeF32(dst, src, 2, 9) // count=8
	for i := range dst {
		if i >= 2 && i <= 9 {
			if dst[i] != src[i] {
				t.Fatalf("dst[%d]=%f; want %f", i, dst[i], src[i])
			}
		} else if dst[i] != 0 {
			t.Fatalf("dst[%d]=%f; want 0 outside range", i, dst[i])
		}
	}
}

func TestMatchRunesDPASCIIEqualsScalar(t *testing.T) {
	fm := NewFuzzyMatcher()
	cases := []struct {
		pattern string
		text    string
	}{
		{"main", "main.go"},
		{"mgo", "module/main.go"},
		{"rd", "src/pkg/rdir/main.go"},
		{"abc", "a_b_c_d"},
	}

	for _, tc := range cases {
		patternRunes, patternBuf := acquireRunes(tc.pattern, true)
		textRunes, textBuf := acquireRunes(tc.text, true)
		asciiText, asciiTextBuf := runeSliceToASCIIBytes(textRunes)
		asciiPattern, asciiPatternBuf := runeSliceToASCIIBytes(patternRunes)

		scalarBoundary := acquireBoundaryBuffer(len(textRunes))
		scoreScalar, matchedScalar, startScalar, endScalar, targetScalar, matchCountScalar, wordHitsScalar, spansScalar := fm.matchRunesDPScalar(patternRunes, textRunes, scalarBoundary, asciiText, asciiPattern, true)
		releaseBoundaryBuffer(scalarBoundary)

		asciiBoundary := acquireBoundaryBuffer(len(textRunes))
		scoreASCII, matchedASCII, startASCII, endASCII, targetASCII, matchCountASCII, wordHitsASCII, spansASCII, _ := fm.matchRunesDPASCII(patternRunes, textRunes, asciiBoundary, asciiText, asciiPattern, true)
		releaseBoundaryBuffer(asciiBoundary)

		releaseByteBuffer(asciiPatternBuf)
		releaseByteBuffer(asciiTextBuf)
		releaseRunes(patternBuf)
		releaseRunes(textBuf)

		if matchedScalar != matchedASCII {
			t.Fatalf("case %q/%q: matched mismatch (scalar=%v ascii=%v)", tc.pattern, tc.text, matchedScalar, matchedASCII)
		}
		if !matchedScalar {
			continue
		}
		if scoreScalar != scoreASCII {
			t.Fatalf("case %q/%q: score mismatch (scalar=%f ascii=%f)", tc.pattern, tc.text, scoreScalar, scoreASCII)
		}
		if startScalar != startASCII || endScalar != endASCII || targetScalar != targetASCII || matchCountScalar != matchCountASCII || wordHitsScalar != wordHitsASCII || !equalMatchSpans(spansScalar, spansASCII) {
			t.Fatalf("case %q/%q: metadata mismatch", tc.pattern, tc.text)
		}
	}
}
