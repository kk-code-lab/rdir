//go:build !arm64 || purego

package search

func (fm *FuzzyMatcher) matchRunesDPASCII(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte, spanMode spanRequest) (float64, bool, int, int, int, int, int, []MatchSpan, []int, bool) {
	score, matched, start, end, targetLen, matchCount, wordHits, spans, positions := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern, spanMode)
	return score, matched, start, end, targetLen, matchCount, wordHits, spans, positions, true
}

func runPrefixASCII(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	scalarPrefixRef(prefix, prefixIdx, dpPrev, start, end, gap)
}

func asmCopyOne(prefix []float32, prefixIdx []int32, i int) {
	// No-op on non-arm64 builds; tests that rely on asm behavior are skipped.
}

func asmSetIdxRange(prefixIdx []int32, start, end int) {
	// No-op placeholder for non-arm64 targets.
}

func asmCopyRangeF32(dst []float32, src []float32, start, end int) {
	// No-op placeholder for non-arm64 targets.
}

func asmCopyPrefixRange(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	// No-op placeholder for non-arm64 targets.
}
