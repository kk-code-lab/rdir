//go:build !arm64 || purego

package search

func (fm *FuzzyMatcher) matchRunesDPASCII32(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte, spanMode spanRequest) (float64, bool, int, int, int, int, int, []MatchSpan, []int, bool) {
	// ASCII32 optimization is only available on ARM64.
	// On other platforms, fall back to scalar implementation.
	score, matched, start, end, targetLen, matchCount, wordHits, spans, positions := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern, spanMode)
	return score, matched, start, end, targetLen, matchCount, wordHits, spans, positions, true
}
