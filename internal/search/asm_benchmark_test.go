package search

import (
	"runtime"
	"strings"
	"testing"
)

func benchAsmSetIdxRange(b *testing.B, n int) {
	if runtime.GOARCH != "arm64" {
		b.Skip("arm64-only benchmark")
	}
	idx := make([]int32, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// touch slice to avoid dead-code elimination
		idx[0] = 0
		asmSetIdxRange(idx, 0, n-1)
	}
}

func benchAsmCopyRangeF32(b *testing.B, n int) {
	if runtime.GOARCH != "arm64" {
		b.Skip("arm64-only benchmark")
	}
	src := make([]float32, n)
	dst := make([]float32, n)
	for i := range src {
		src[i] = float32(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// touch slices to avoid dead-code elimination
		dst[0] = 0
		asmCopyRangeF32(dst, src, 0, n-1)
	}
}

func BenchmarkAsmSetIdxRange_8(b *testing.B)    { benchAsmSetIdxRange(b, 8) }
func BenchmarkAsmSetIdxRange_64(b *testing.B)   { benchAsmSetIdxRange(b, 64) }
func BenchmarkAsmSetIdxRange_1024(b *testing.B) { benchAsmSetIdxRange(b, 1024) }

func BenchmarkAsmCopyRangeF32_8(b *testing.B)    { benchAsmCopyRangeF32(b, 8) }
func BenchmarkAsmCopyRangeF32_64(b *testing.B)   { benchAsmCopyRangeF32(b, 64) }
func BenchmarkAsmCopyRangeF32_1024(b *testing.B) { benchAsmCopyRangeF32(b, 1024) }

// DP benchmarks with ASCII32 forced (compare vs existing BenchmarkMatchRunesDPAscii)
func BenchmarkMatchRunesDPAscii32Forced(b *testing.B) {
	if runtime.GOARCH != "arm64" {
		b.Skip("arm64-only benchmark")
	}
	prevSIMD := fuzzySIMDDPDisabled
	prevForce := ascii32Force
	fuzzySIMDDPDisabled = true
	ascii32Force = true
	defer func() {
		fuzzySIMDDPDisabled = prevSIMD
		ascii32Force = prevForce
	}()

	fm := NewFuzzyMatcher()
	text := strings.Repeat("pkg/subpkg/", 24) + "fuzzy_matcher_internal.go"
	pattern := "fmint"

	textRunes, textBuf := acquireRunes(text, false)
	defer releaseRunes(textBuf)
	patternRunes, patternBuf := acquireRunes(pattern, false)
	defer releaseRunes(patternBuf)
	boundary := acquireBoundaryBuffer(len(textRunes))
	defer releaseBoundaryBuffer(boundary)

	textASCII, textASCIIBuf := runeSliceToASCIIBytes(textRunes)
	defer releaseByteBuffer(textASCIIBuf)
	patternASCII, patternASCIIBuf := runeSliceToASCIIBytes(patternRunes)
	defer releaseByteBuffer(patternASCIIBuf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		score, matched, start, end, _, _, _, _ := fm.matchRunesDP(patternRunes, textRunes, boundary, textASCII, patternASCII, true)
		if !matched {
			b.Fatalf("expected match, start=%d end=%d score=%f", start, end, score)
		}
	}
}

// Additional DP benchmarks to measure impact across sizes
func benchDP(b *testing.B, pattern, text string, forceASCII32 bool) {
	if runtime.GOARCH != "arm64" && forceASCII32 {
		b.Skip("arm64-only benchmark")
	}
	prevSIMD := fuzzySIMDDPDisabled
	prevForce := ascii32Force
	if forceASCII32 {
		fuzzySIMDDPDisabled = true
		ascii32Force = true
		defer func() { fuzzySIMDDPDisabled = prevSIMD; ascii32Force = prevForce }()
	}

	fm := NewFuzzyMatcher()
	textRunes, textBuf := acquireRunes(text, false)
	defer releaseRunes(textBuf)
	patternRunes, patternBuf := acquireRunes(pattern, false)
	defer releaseRunes(patternBuf)
	boundary := acquireBoundaryBuffer(len(textRunes))
	defer releaseBoundaryBuffer(boundary)
	textASCII, textASCIIBuf := runeSliceToASCIIBytes(textRunes)
	defer releaseByteBuffer(textASCIIBuf)
	patternASCII, patternASCIIBuf := runeSliceToASCIIBytes(patternRunes)
	defer releaseByteBuffer(patternASCIIBuf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		score, matched, start, end, _, _, _, _ := fm.matchRunesDP(patternRunes, textRunes, boundary, textASCII, patternASCII, true)
		if !matched {
			b.Fatalf("expected match, start=%d end=%d score=%f", start, end, score)
		}
	}
}

// Short/mid/long scenarios (ASCII-only)
func BenchmarkDPAscii32_Short(b *testing.B) {
	benchDP(b, "main", "src/pkg/main.go", true)
}

func BenchmarkDPAscii32_Medium(b *testing.B) {
	benchDP(b, "fmint", strings.Repeat("pkg/subpkg/", 24)+"fuzzy_matcher_internal.go", true)
}

func BenchmarkDPAscii32_Long(b *testing.B) {
	benchDP(b, "rdirmain", strings.Repeat("src/example/", 64)+"rdir_cli_main.go", true)
}

// Scalar baselines for the same texts
func BenchmarkDPScalar_Short(b *testing.B) {
	benchDP(b, "main", "src/pkg/main.go", false)
}

func BenchmarkDPScalar_Medium(b *testing.B) {
	benchDP(b, "fmint", strings.Repeat("pkg/subpkg/", 24)+"fuzzy_matcher_internal.go", false)
}

func BenchmarkDPScalar_Long(b *testing.B) {
	benchDP(b, "rdirmain", strings.Repeat("src/example/", 64)+"rdir_cli_main.go", false)
}

// Pure Go baselines for comparison
func benchGoSetIdxRange(b *testing.B, n int) {
	idx := make([]int32, n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx[0] = 0
		for j := 0; j < n; j++ {
			idx[j] = -1
		}
	}
}

func benchGoCopyRangeF32(b *testing.B, n int) {
	src := make([]float32, n)
	dst := make([]float32, n)
	for i := range src {
		src[i] = float32(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst[0] = 0
		for j := 0; j < n; j++ {
			dst[j] = src[j]
		}
	}
}

func BenchmarkGoSetIdxRange_8(b *testing.B)    { benchGoSetIdxRange(b, 8) }
func BenchmarkGoSetIdxRange_64(b *testing.B)   { benchGoSetIdxRange(b, 64) }
func BenchmarkGoSetIdxRange_1024(b *testing.B) { benchGoSetIdxRange(b, 1024) }

func BenchmarkGoCopyRangeF32_8(b *testing.B)    { benchGoCopyRangeF32(b, 8) }
func BenchmarkGoCopyRangeF32_64(b *testing.B)   { benchGoCopyRangeF32(b, 64) }
func BenchmarkGoCopyRangeF32_1024(b *testing.B) { benchGoCopyRangeF32(b, 1024) }
