package search

import (
	"fmt"
	"strings"
	"testing"
)

func benchmarkAcquireRunes(b *testing.B, input string, fold bool) {
	b.Helper()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		runes, buf := acquireRunes(input, fold)
		if len(runes) == 0 && len(input) != 0 {
			b.Fatal("unexpected empty rune slice")
		}
		releaseRunes(buf)
	}
}

func BenchmarkAcquireRunesASCIIFolded(b *testing.B) {
	const sample = "src/pkg/ExampleASCIIPath/ComponentFile.go"
	benchmarkAcquireRunes(b, sample, true)
}

func BenchmarkAcquireRunesUnicodeFolded(b *testing.B) {
	const sample = "źródła/środowisko/Łódź_Księży_Młyn.go"
	benchmarkAcquireRunes(b, sample, true)
}

func BenchmarkAcquireRunesMixedExact(b *testing.B) {
	const sample = "pkg/Ωmega/Σigma/Δelta/File.go"
	benchmarkAcquireRunes(b, sample, false)
}

func benchmarkIndexRunes(b *testing.B, haystack, needle string) {
	b.Helper()

	haystackRunes, haystackBuf := acquireRunes(haystack, false)
	defer releaseRunes(haystackBuf)
	needleRunes, needleBuf := acquireRunes(needle, false)
	defer releaseRunes(needleBuf)

	if len(needleRunes) == 0 || len(haystackRunes) == 0 {
		b.Fatalf("invalid benchmark setup: haystack=%q needle=%q", haystack, needle)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := indexRunes(haystackRunes, needleRunes)
		if idx == -1 {
			b.Fatal("unexpected miss during benchmark")
		}
	}
}

func BenchmarkIndexRunesASCIIHit(b *testing.B) {
	haystack := strings.Repeat("src/pkg/", 16) + "fuzzy_matcher.go"
	needle := "fuzzy"
	benchmarkIndexRunes(b, haystack, needle)
}

func BenchmarkIndexRunesASCIIEndHit(b *testing.B) {
	haystack := strings.Repeat("foo/bar/", 12) + "matcher_impl_test.go"
	needle := "matcher_impl_test.go"
	benchmarkIndexRunes(b, haystack, needle)
}

func BenchmarkIndexRunesUnicodeHit(b *testing.B) {
	haystack := "ścieżka/do/pliku/żółć/łódź/fuzzy.go"
	needle := "łódź"
	benchmarkIndexRunes(b, haystack, needle)
}

func BenchmarkMatchRunesDPAscii(b *testing.B) {
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
		score, matched, start, end, _, _, _ := fm.matchRunesDP(patternRunes, textRunes, boundary, textASCII, patternASCII)
		if !matched {
			b.Fatalf("expected match, start=%d end=%d score=%f", start, end, score)
		}
	}
}

func BenchmarkMatchMultipleSynthetic(b *testing.B) {
	fm := NewFuzzyMatcher()
	pattern := "rdirmain"
	const fileCount = 2048

	texts := make([]string, fileCount)
	for i := range texts {
		texts[i] = fmt.Sprintf("src/example/%04d/rdir_cli_main.go", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matches := fm.MatchMultiple(pattern, texts)
		if len(matches) == 0 {
			b.Fatal("expected at least one match")
		}
	}
}
