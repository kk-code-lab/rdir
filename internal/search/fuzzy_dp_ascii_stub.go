//go:build !arm64 || purego

package search

func (fm *FuzzyMatcher) matchRunesDPASCII(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte) (float64, bool, int, int, int, int, int, bool) {
	score, matched, start, end, targetLen, matchCount, wordHits := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern)
	return score, matched, start, end, targetLen, matchCount, wordHits, true
}

func runPrefixASCII(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	scalarPrefixRef(prefix, prefixIdx, dpPrev, start, end, gap)
}
