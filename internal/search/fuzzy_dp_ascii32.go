//go:build arm64 && !purego

package search

import "math"

func (fm *FuzzyMatcher) matchRunesDPASCII32(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte) (float64, bool, int, int, int, int, int, bool) {
	m := len(pattern)
	n := len(text)
	if n == 0 || m == 0 || m > n {
		score, matched, start, end, targetLen, matchCount, wordHits := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern)
		return score, matched, start, end, targetLen, matchCount, wordHits, true
	}

	cols := n
	if cols > ascii32MaxCols {
		cols = ascii32MaxCols
	}
	withinEnvelope := m <= ascii32MaxRows && n <= ascii32MaxCols

	s := acquireDPScratch(m, n)
	dpPrev32 := s.dpPrev32[:cols]
	dpCurr32 := s.dpCurr32[:cols]
	prefix32 := s.prefix32[:cols]
	prefixIdx32 := s.prefixIdx32[:cols]

	fallback := func() (float64, bool, int, int, int, int, int, bool) {
		releaseDPScratch(s)
		score, matched, start, end, targetLen, matchCount, wordHits := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern)
		return score, matched, start, end, targetLen, matchCount, wordHits, true
	}

	if cols > 0 {
		fuzzySetRangeF32NegInfAsm(&dpPrev32[0], cols)
		fuzzySetRangeF32NegInfAsm(&dpCurr32[0], cols)
	}

	negInf32 := float32(math.Inf(-1))
	validThreshold := negInf32 / 2
	gapPenalty32 := float32(fm.gapPenalty)
	consecutiveBonus32 := float32(fm.consecutiveBonus)
	charBonus32 := float32(fm.charBonus)
	wordBoundaryBonus32 := float32(fm.wordBoundaryBonus)
	leadingPenaltyStep := float32(fm.gapPenalty * 0.02)

	minActive := -1
	maxActive := -1
	firstRune := pattern[0]
	for j := 0; j < cols; j++ {
		if n-j < m {
			break
		}
		if text[j] != firstRune {
			continue
		}
		score := charBonus32
		if isWordBoundary(boundaryBuf, text, j) {
			score += wordBoundaryBonus32
		}
		score -= leadingPenaltyStep * float32(j)
		dpPrev32[j] = score
		if minActive == -1 {
			minActive = j
		}
		maxActive = j
	}
	if ascii32Debug {
		for idx, val := range dpPrev32 {
			if val > validThreshold {
				fuzzyLogf("ascii32: row0 seed idx=%d char=%q score=%f", idx, string(text[idx]), val)
			}
		}
	}
	dpFailed := minActive == -1

	rowsLimit := m
	if rowsLimit > ascii32MaxRows {
		rowsLimit = ascii32MaxRows
	}

	for i := 1; i < rowsLimit && !dpFailed; i++ {
		if cols > 0 {
			fuzzySetRangeF32NegInfAsm(&dpCurr32[0], cols)
		}

		windowStart := minActive - ascii32BeamWidth
		if windowStart < 0 {
			windowStart = 0
		}
		windowEnd := maxActive + ascii32BeamWidth
		if windowEnd >= cols {
			windowEnd = cols - 1
		}
		if windowStart > windowEnd || windowEnd < 0 {
			dpFailed = true
			break
		}

		ascii32BuildPrefixRange(prefix32, prefixIdx32, dpPrev32, windowStart, windowEnd, gapPenalty32)

		matchCount := ascii32CollectMatches(
			s,
			i,
			pattern[i],
			prefix32,
			prefixIdx32,
			dpPrev32,
			text,
			boundaryBuf,
			windowStart,
			windowEnd,
			m-i,
			charBonus32,
			wordBoundaryBonus32,
			gapPenalty32,
			consecutiveBonus32,
			negInf32,
			n,
		)
		if matchCount == 0 {
			if ascii32Debug {
				fuzzyLogf("ascii32: row=%d no matches pattern=%q text=%q window=[%d,%d] min=%d max=%d", i, string(pattern), string(text), windowStart, windowEnd, minActive, maxActive)
			}
			dpFailed = true
			break
		}

		matches := s.matchCols[:matchCount]
		nextMinActive := -1
		nextMaxActive := -1
		for idx := 0; idx < matchCount; idx += ascii32ChunkWidth {
			chunkEnd := idx + ascii32ChunkWidth
			if chunkEnd > matchCount {
				chunkEnd = matchCount
			}
			chunkMin, chunkMax := ascii32ProcessChunk(
				s,
				dpCurr32,
				matches,
				idx,
				chunkEnd,
				negInf32,
				validThreshold,
				i,
				s.cols,
				s.backtrack,
				s.backtrackGen,
				s.generation,
			)
			if chunkMin == -1 {
				continue
			}
			if nextMinActive == -1 || chunkMin < nextMinActive {
				nextMinActive = chunkMin
			}
			if chunkMax > nextMaxActive {
				nextMaxActive = chunkMax
			}
		}

		if nextMaxActive == -1 {
			if ascii32Debug {
				fuzzyLogf("ascii32: row=%d no active columns pattern=%q text=%q matchCount=%d", i, string(pattern), string(text), matchCount)
			}
			dpFailed = true
			break
		}

		dpPrev32, dpCurr32 = dpCurr32, dpPrev32
		minActive = nextMinActive - ascii32BeamMargin
		if minActive < 0 {
			minActive = 0
		}
		maxActive = nextMaxActive + ascii32BeamMargin
		if maxActive >= cols {
			maxActive = cols - 1
		}
	}

	if withinEnvelope {
		if dpFailed {
			releaseDPScratch(s)
			return 0.0, false, -1, -1, n, 0, 0, true
		}
		bestEnd := -1
		bestVal := negInf32
		for j := 0; j < cols; j++ {
			if dpPrev32[j] > bestVal {
				bestVal = dpPrev32[j]
				bestEnd = j
			}
		}
		if bestEnd == -1 {
			releaseDPScratch(s)
			return 0.0, false, -1, -1, n, 0, 0, true
		}
		positions := make([]int, m)
		k := bestEnd
		for i := m - 1; i >= 0; i-- {
			positions[i] = k
			if i > 0 {
				cell := i*s.cols + k
				if s.backtrackGen[cell] != s.generation {
					releaseDPScratch(s)
					return 0.0, false, -1, -1, n, 0, 0, true
				}
				k = s.backtrack[cell]
				if k == -1 {
					releaseDPScratch(s)
					return 0.0, false, -1, -1, n, 0, 0, true
				}
			}
		}
		score := float64(bestVal)
		trailingLen := n - positions[m-1] - 1
		if trailingLen > 20 {
			score -= fm.gapPenalty * 0.25 * float64((trailingLen-20)/10)
		}
		hits := 0
		for _, idx := range positions {
			if isStrongWordBoundary(boundaryBuf, text, idx) {
				hits++
			}
		}
		start := positions[0]
		end := positions[m-1]
		releaseDPScratch(s)
		return score, true, start, end, n, m, hits, true
	}

	return fallback()
}

func ascii32BuildPrefixRange(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	if len(prefix) == 0 || len(prefixIdx) == 0 || len(dpPrev) == 0 || end < 0 {
		return
	}
	runPrefixASCII(prefix, prefixIdx, dpPrev, start, end, gap)
}

func ascii32CollectMatches(s *dpScratch, row int, needle rune, prefix []float32, prefixIdx []int32, dpPrev []float32, text []rune, boundaryBuf *boundaryBuffer, windowStart, windowEnd int, remainingPattern int, charBonus, wordBonus, gap float32, consecutiveBonus float32, negInf float32, textLen int) int {
	matches := s.matchCols[:0]
	matches32 := s.matchCols32[:0]
	prefixVals := s.matchPrefix32[:0]
	prefixIdxVals := s.matchPrefixIdx32[:0]
	prevVals := s.matchPrev32[:0]
	prevIdxVals := s.matchPrevIdx[:0]

	if windowStart < 0 {
		windowStart = 0
	}
	if windowEnd >= len(text) {
		windowEnd = len(text) - 1
	}
	if windowEnd >= len(prefix) {
		windowEnd = len(prefix) - 1
	}
	for j := windowStart; j <= windowEnd; j++ {
		if textLen-j < remainingPattern {
			break
		}
		if text[j] != needle {
			continue
		}
		score := charBonus
		if isWordBoundary(boundaryBuf, text, j) {
			score += wordBonus
		}
		matches = append(matches, j)
		matches32 = append(matches32, int32(j))

		prefCandidate := negInf
		prefIdx := int32(-1)
		if prefixIdx[j] >= 0 && prefix[j] > negInf/2 {
			candidate := prefix[j] + score
			if prefixIdx[j] == int32(j-1) {
				candidate += consecutiveBonus
			}
			prefCandidate = candidate
			prefIdx = prefixIdx[j]
		}

		prevCandidate := negInf
		prevIdx := int32(-1)
		if j > 0 && dpPrev[j-1] > negInf/2 {
			prevCandidate = dpPrev[j-1] + score + consecutiveBonus
			prevIdx = int32(j - 1)
		}

		if ascii32Debug {
			fuzzyLogf("ascii32: row=%d needle=%q match@%d prefixCandidate=(%f,%d) prevCandidate=(%f,%d)", row, needle, j, prefCandidate, prefIdx, prevCandidate, prevIdx)
		}
		prefixVals = append(prefixVals, prefCandidate)
		prefixIdxVals = append(prefixIdxVals, prefIdx)
		prevVals = append(prevVals, prevCandidate)
		prevIdxVals = append(prevIdxVals, prevIdx)
	}

	s.matchCols = matches
	s.matchCols32 = matches32
	s.matchPrefix32 = prefixVals
	s.matchPrefixIdx32 = prefixIdxVals
	s.matchPrev32 = prevVals
	s.matchPrevIdx = prevIdxVals

	return len(matches)
}

func ascii32ProcessChunk(
	s *dpScratch,
	dpCurr []float32,
	matches []int,
	chunkStart int,
	chunkEnd int,
	negInf float32,
	threshold float32,
	row int,
	cols int,
	backtrack []int,
	backtrackGen []uint32,
	generation uint32,
) (int, int) {
	count := chunkEnd - chunkStart
	if count <= 0 {
		return -1, -1
	}

	matchPrefix := s.matchPrefix32[chunkStart:chunkEnd]
	matchPrefixIdx := s.matchPrefixIdx32[chunkStart:chunkEnd]
	matchPrev := s.matchPrev32[chunkStart:chunkEnd]
	matchPrevIdx := s.matchPrevIdx[chunkStart:chunkEnd]

	return ascii32ProcessChunkScalar(
		dpCurr,
		matches,
		matchPrefix,
		matchPrefixIdx,
		matchPrev,
		matchPrevIdx,
		chunkStart,
		chunkEnd,
		negInf,
		threshold,
		row,
		cols,
		backtrack,
		backtrackGen,
		generation,
	)
}

func ascii32ProcessChunkScalar(
	dpCurr []float32,
	matches []int,
	matchPrefix []float32,
	matchPrefixIdx []int32,
	matchPrev []float32,
	matchPrevIdx []int32,
	chunkStart int,
	chunkEnd int,
	negInf float32,
	threshold float32,
	row int,
	cols int,
	backtrack []int,
	backtrackGen []uint32,
	generation uint32,
) (int, int) {
	minIdx := -1
	maxIdx := -1
	for lane := 0; lane < chunkEnd-chunkStart; lane++ {
		idx := chunkStart + lane
		j := matches[idx]
		bestScore, bestIdx := ascii32ChooseBest(matchPrefix[lane], matchPrefixIdx[lane], matchPrev[lane], matchPrevIdx[lane], threshold, negInf)
		if bestIdx == -1 {
			continue
		}

		dpCurr[j] = bestScore
		cell := row*cols + j
		backtrack[cell] = int(bestIdx)
		backtrackGen[cell] = generation
		if minIdx == -1 || j < minIdx {
			minIdx = j
		}
		if j > maxIdx {
			maxIdx = j
		}
	}
	return minIdx, maxIdx
}

func ascii32ChooseBest(prefixScore float32, prefixIdx int32, prevScore float32, prevIdx int32, threshold float32, negInf float32) (float32, int32) {
	bestScore := prefixScore
	bestIdx := prefixIdx
	if prevScore > bestScore {
		bestScore = prevScore
		bestIdx = prevIdx
	}
	return ascii32ApplyThreshold(bestScore, bestIdx, threshold, negInf)
}

func ascii32ApplyThreshold(score float32, idx int32, threshold float32, negInf float32) (float32, int32) {
	if idx == -1 || score <= threshold {
		return negInf, -1
	}
	return score, idx
}
