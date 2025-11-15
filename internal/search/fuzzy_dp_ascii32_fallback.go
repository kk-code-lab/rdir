//go:build !arm64 || purego

package search

func (fm *FuzzyMatcher) matchRunesDPASCII32(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte) (float64, bool, int, int, int, int, int, []MatchSpan, bool) {
	// ASCII32 optimization is only available on ARM64.
	// On other platforms, fall back to scalar implementation.
	score, matched, start, end, targetLen, matchCount, wordHits, spans := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern)
	return score, matched, start, end, targetLen, matchCount, wordHits, spans, true
}
