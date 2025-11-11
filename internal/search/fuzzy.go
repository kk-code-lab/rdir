package search

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// FuzzyMatch represents a single match result
type FuzzyMatch struct {
	FileIndex int
	Score     float64 // 0.0 to 1.0
}

// MatchSpan represents the inclusive [Start, End] range of a match in rune indexes.
type MatchSpan struct {
	Start int
	End   int
}

// MatchDetails exposes additional metadata about a fuzzy match.
// Start/End are rune indexes into the target text; TargetLength
// is the total rune length of the target text, used for tie-breaks.
type MatchDetails struct {
	Start        int
	End          int
	TargetLength int
	MatchCount   int
	WordHits     int
	Spans        []MatchSpan
}

const (
	boundaryWord = 1 << iota
	boundaryStrong
)

// FuzzyMatcher performs fuzzy pattern matching
// Algorithm: Similar to fzf/sublime text
// Scoring:
//   - Consecutive characters: +2 per char
//   - Character at word boundary (uppercase/after /): +1.5 per char
//   - Other characters: +1 per char
//   - Non-consecutive: -0.5 per gap
//   - Case mismatch: -0.1 per char
type FuzzyMatcher struct {
	// Thresholds (can be tuned)
	minScore                float64
	consecutiveBonus        float64
	wordBoundaryBonus       float64
	charBonus               float64
	gapPenalty              float64
	caseMismatchPenalty     float64
	substringBonus          float64
	prefixBonus             float64
	finalSegmentBonus       float64
	startPenaltyFactor      float64
	crossSegmentPenalty     float64
	wordHitBonus            float64
	substringBoundaryFactor float64
	substringInteriorFactor float64
}

// NewFuzzyMatcher creates a new fuzzy matcher with default settings
func NewFuzzyMatcher() *FuzzyMatcher {
	return &FuzzyMatcher{
		minScore:                0.0, // Accept all matches (filter by threshold later)
		consecutiveBonus:        1.2,
		wordBoundaryBonus:       0.6,
		charBonus:               1.2,
		gapPenalty:              0.18,
		caseMismatchPenalty:     0.1,
		substringBonus:          1.2,
		prefixBonus:             2.4,
		finalSegmentBonus:       2.0,
		startPenaltyFactor:      0.012,
		crossSegmentPenalty:     0.9,
		wordHitBonus:            3.2,
		substringBoundaryFactor: 0.3,
		substringInteriorFactor: 0.15,
	}
}

func patternHasUppercase(pattern string) bool {
	for _, r := range pattern {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// Match calculates match score between pattern and text
// Returns:
//   - score: 0.0 if no match, higher for better matches
//   - matched: true if all pattern characters are found in order
func (fm *FuzzyMatcher) Match(pattern, text string) (score float64, matched bool) {
	if pattern == "" {
		return 1.0, true // Empty pattern matches everything
	}

	score, matched, _ = fm.MatchDetailed(pattern, text)
	return score, matched
}

// MatchDetailed returns the fuzzy match score together with metadata that describes
// where in the target text the match landed. Callers that need tie-break data (e.g.
// deterministic sorting) can use this instead of Match.
func (fm *FuzzyMatcher) MatchDetailed(pattern, text string) (score float64, matched bool, details MatchDetails) {
	return fm.matchDetailedWithMode(pattern, text, patternHasUppercase(pattern))
}

// MatchDetailedWithMode behaves like MatchDetailed but callers can control case sensitivity explicitly.
func (fm *FuzzyMatcher) MatchDetailedWithMode(pattern, text string, caseSensitive bool) (float64, bool, MatchDetails) {
	return fm.matchDetailedWithMode(pattern, text, caseSensitive)
}

// MatchDetailedFromRunes lets callers reuse precomputed rune slices for both the pattern and target text.
// Callers are responsible for providing runes that already reflect any desired case folding.
func (fm *FuzzyMatcher) MatchDetailedFromRunes(pattern string, patternRunes []rune, text string, textRunes []rune) (float64, bool, MatchDetails) {
	if len(patternRunes) == 0 {
		return 1.0, true, MatchDetails{
			Start:        0,
			End:          -1,
			TargetLength: len(textRunes),
			MatchCount:   0,
			WordHits:     0,
			Spans:        nil,
		}
	}
	score, matched, details, _ := fm.matchWithRunes(pattern, text, patternRunes, textRunes)
	return score, matched, details
}

func (fm *FuzzyMatcher) matchDetailedWithMode(pattern, text string, caseSensitive bool) (score float64, matched bool, details MatchDetails) {
	if pattern == "" {
		return 1.0, true, MatchDetails{
			Start:        0,
			End:          -1,
			TargetLength: utf8.RuneCountInString(text),
			MatchCount:   0,
			WordHits:     0,
			Spans:        nil,
		}
	}

	fold := !caseSensitive
	patternRunes, patternBuf := acquireRunes(pattern, fold)
	defer releaseRunes(patternBuf)
	textRunes, textBuf := acquireRunes(text, fold)
	defer releaseRunes(textBuf)

	score, matched, details, substringIdx := fm.matchWithRunes(pattern, text, patternRunes, textRunes)
	if matched && fuzzyDebugEnabled() {
		fuzzyLogf("pattern=%q text=%q score=%.6f start=%d end=%d len=%d matches=%d wordHits=%d substrIdx=%d caseSensitive=%v",
			pattern, text, score, details.Start, details.End, details.TargetLength, details.MatchCount, details.WordHits, substringIdx, caseSensitive)
	}
	return score, matched, details
}

func (fm *FuzzyMatcher) matchWithRunes(pattern, text string, patternRunes, textRunes []rune) (float64, bool, MatchDetails, int) {
	asciiCandidate := runesAreASCII(textRunes) && runesAreASCII(patternRunes)
	var asciiText []byte
	var asciiPattern []byte
	var asciiTextBuf *byteBuffer
	var asciiPatternBuf *byteBuffer
	if asciiCandidate {
		asciiText, asciiTextBuf = runeSliceToASCIIBytes(textRunes)
		asciiPattern, asciiPatternBuf = runeSliceToASCIIBytes(patternRunes)
		defer releaseByteBuffer(asciiTextBuf)
		defer releaseByteBuffer(asciiPatternBuf)
	}

	var substringIdx int
	if asciiCandidate {
		substringIdx = indexRunesASCIIBytes(asciiText, asciiPattern)
	} else {
		substringIdx = indexRunes(textRunes, patternRunes)
	}
	baseScore := 0.0
	matched := false
	start, end := -1, -1
	targetLen := len(textRunes)
	matchCount := len(patternRunes)
	wordHits := 0
	var boundaryBuf *boundaryBuffer
	defer func() {
		if boundaryBuf != nil {
			releaseBoundaryBuffer(boundaryBuf)
		}
	}()

	if substringIdx != -1 {
		var contiguousScore float64
		var contiguousWordHits int
		contiguousScore, start, end, contiguousWordHits = fm.contiguousMatchScore(patternRunes, textRunes, nil, substringIdx)
		if start >= 0 {
			baseScore = contiguousScore
			wordHits = contiguousWordHits
			matched = true
		}
	}

	if !matched {
		if boundaryBuf == nil {
			boundaryBuf = acquireBoundaryBuffer(len(textRunes))
		}
		var dpScore float64
		var dpMatched bool
		dpScore, dpMatched, start, end, targetLen, matchCount, wordHits = fm.matchRunesDP(patternRunes, textRunes, boundaryBuf, asciiText, asciiPattern)
		if dpMatched {
			baseScore = dpScore
			matched = true
		}
	}

	if !matched || start < 0 || end < start || end >= targetLen {
		return 0, false, MatchDetails{
			Start:        -1,
			End:          -1,
			TargetLength: targetLen,
			MatchCount:   0,
			WordHits:     0,
			Spans:        nil,
		}, substringIdx
	}

	score := baseScore
	if substringIdx != -1 {
		substringBonus := fm.substringBonus
		if substringIdx > 0 {
			prev := textRunes[substringIdx-1]
			switch prev {
			case '/', '\\':
				// keep full bonus
			case '-', '_', ' ', '.', ':':
				substringBonus *= fm.substringBoundaryFactor
			default:
				substringBonus *= fm.substringInteriorFactor
			}
		}
		score += substringBonus
		if substringIdx == 0 {
			score += fm.prefixBonus
		}
	}

	if start <= end {
		segmentRunes := textRunes[start : end+1]
		crossSegments := 0
		for _, r := range segmentRunes {
			if r == '/' {
				crossSegments++
			}
		}
		if crossSegments > 0 {
			score -= fm.crossSegmentPenalty * float64(crossSegments)
		}
	}

	lastSlashRune := -1
	for idx, r := range textRunes {
		if r == '/' {
			lastSlashRune = idx
		}
	}
	if lastSlashRune != -1 && start <= lastSlashRune {
		score -= fm.startPenaltyFactor * float64(lastSlashRune-start)
	}

	inFinalSegment := lastSlashRune == -1 || start >= lastSlashRune+1
	if inFinalSegment || (substringIdx != -1 && substringIdx >= lastSlashRune+1) {
		score += fm.finalSegmentBonus
	}

	score += fm.wordHitBonus * float64(wordHits)

	return score, true, MatchDetails{
		Start:        start,
		End:          end,
		TargetLength: targetLen,
		MatchCount:   matchCount,
		WordHits:     wordHits,
		Spans: []MatchSpan{
			{Start: start, End: end},
		},
	}, substringIdx
}

func (fm *FuzzyMatcher) matchRunesDP(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte) (float64, bool, int, int, int, int, int) {
	useASCII := asciiText != nil && asciiPattern != nil && len(asciiText) == len(text) && len(asciiPattern) == len(pattern)
	if useASCII && fuzzySIMDDPEnabled() {
		if score, matched, start, end, targetLen, matchCount, wordHits, ok := fm.matchRunesDPASCII(pattern, text, boundaryBuf, asciiText, asciiPattern); ok {
			return score, matched, start, end, targetLen, matchCount, wordHits
		}
	}
	// Experimental: float32 ASCII DP (NEON primitives). Guarded by env flag.
	if useASCII && fuzzyASCII32Enabled() {
		if score, matched, start, end, targetLen, matchCount, wordHits, ok := fm.matchRunesDPASCII32(pattern, text, boundaryBuf, asciiText, asciiPattern); ok {
			return score, matched, start, end, targetLen, matchCount, wordHits
		}
	}
	return fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern)
}

func (fm *FuzzyMatcher) matchRunesDPScalar(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte) (float64, bool, int, int, int, int, int) {
	m := len(pattern)
	n := len(text)
	if n == 0 || m > n {
		return 0.0, false, -1, -1, n, 0, 0
	}

	const dpBeamWidth = 96
	const dpBeamMargin = 48

	negInf := math.Inf(-1)
	scratch := acquireDPScratch(m, n)
	defer releaseDPScratch(scratch)
	dpPrev := scratch.dpPrev
	dpCurr := scratch.dpCurr
	for j := range dpPrev {
		dpPrev[j] = negInf
	}
	for j := range dpCurr {
		dpCurr[j] = negInf
	}
	backtrack := scratch.backtrack
	backtrackGen := scratch.backtrackGen
	cols := scratch.cols

	minActive := -1
	maxActive := -1
	for j := 0; j < n; j++ {
		if n-j < m {
			break
		}
		if pattern[0] != text[j] {
			continue
		}
		score := fm.charBonus
		if isWordBoundary(boundaryBuf, text, j) {
			score += fm.wordBoundaryBonus
		}
		leadingPenalty := fm.gapPenalty * 0.02 * float64(j)
		score -= leadingPenalty
		dpPrev[j] = score
		if minActive == -1 {
			minActive = j
		}
		maxActive = j
	}

	if maxActive == -1 {
		return 0.0, false, -1, -1, n, 0, 0
	}

	for i := 1; i < m; i++ {
		for j := range dpCurr {
			dpCurr[j] = negInf
		}

		windowStart := minActive - dpBeamWidth
		if windowStart < 0 {
			windowStart = 0
		}
		windowEnd := maxActive + dpBeamWidth
		if windowEnd >= n {
			windowEnd = n - 1
		}

		bestScoreNorm := negInf
		bestIdx := -1
		nextMinActive := -1
		nextMaxActive := -1

		for j := windowStart; j <= windowEnd; j++ {
			if n-j < m-i {
				break
			}
			if bestIdx != -1 && bestScoreNorm > negInf/2 {
				bestScoreNorm -= fm.gapPenalty
			}
			if j > 0 && dpPrev[j-1] > bestScoreNorm {
				bestScoreNorm = dpPrev[j-1]
				bestIdx = j - 1
			}

			if pattern[i] != text[j] {
				continue
			}

			charScore := fm.charBonus
			if isWordBoundary(boundaryBuf, text, j) {
				charScore += fm.wordBoundaryBonus
			}

			bestScore := negInf
			prevIdx := -1

			if bestIdx != -1 && bestScoreNorm > negInf/2 {
				score := bestScoreNorm + charScore
				if bestIdx == j-1 {
					score += fm.consecutiveBonus
				}
				bestScore = score
				prevIdx = bestIdx
			}

			if j > 0 && dpPrev[j-1] > negInf/2 {
				score := dpPrev[j-1] + charScore + fm.consecutiveBonus
				if score > bestScore {
					bestScore = score
					prevIdx = j - 1
				}
			}

			if prevIdx == -1 || bestScore <= negInf/2 {
				continue
			}

			dpCurr[j] = bestScore
			cell := i*cols + j
			backtrack[cell] = prevIdx
			backtrackGen[cell] = scratch.generation
			if nextMinActive == -1 {
				nextMinActive = j
			}
			nextMaxActive = j
		}

		dpPrev, dpCurr = dpCurr, dpPrev
		if nextMaxActive == -1 {
			return 0.0, false, -1, -1, n, 0, 0
		}
		minActive = nextMinActive - dpBeamMargin
		if minActive < 0 {
			minActive = 0
		}
		maxActive = nextMaxActive + dpBeamMargin
		if maxActive >= n {
			maxActive = n - 1
		}
	}

	bestEnd := maxIndex(dpPrev)
	if bestEnd == -1 {
		return 0.0, false, -1, -1, n, 0, 0
	}

	positions := make([]int, m)
	k := bestEnd
	for i := m - 1; i >= 0; i-- {
		positions[i] = k
		if i > 0 {
			cell := i*cols + k
			if backtrackGen[cell] != scratch.generation {
				return 0.0, false, -1, -1, n, 0, 0
			}
			k = backtrack[cell]
			if k == -1 {
				return 0.0, false, -1, -1, n, 0, 0
			}
		}
	}

	bestScore := dpPrev[bestEnd]
	trailingLen := n - positions[m-1] - 1
	if trailingLen > 20 {
		bestScore -= fm.gapPenalty * 0.25 * float64((trailingLen-20)/10)
	}

	wordHits := 0
	for _, idx := range positions {
		if isStrongWordBoundary(boundaryBuf, text, idx) {
			wordHits++
		}
	}

	return bestScore, true, positions[0], positions[m-1], n, m, wordHits
}

func (fm *FuzzyMatcher) contiguousMatchScore(patternRunes, textRunes []rune, boundaryBuf *boundaryBuffer, start int) (float64, int, int, int) {
	matchLen := len(patternRunes)
	if matchLen == 0 {
		return 0, start - 1, start - 1, 0
	}
	end := start + matchLen - 1
	if end >= len(textRunes) {
		return 0, -1, -1, 0
	}

	score := 0.0
	wordHits := 0
	for i := 0; i < matchLen; i++ {
		idx := start + i
		charScore := fm.charBonus
		if boundaryBuf != nil {
			if isWordBoundary(boundaryBuf, textRunes, idx) {
				charScore += fm.wordBoundaryBonus
				if isStrongWordBoundary(boundaryBuf, textRunes, idx) {
					wordHits++
				}
			}
		} else {
			if isWordBoundaryRune(textRunes, idx) {
				charScore += fm.wordBoundaryBonus
				if isStrongWordBoundaryRune(textRunes, idx) {
					wordHits++
				}
			}
		}
		if i == 0 {
			leadingPenalty := fm.gapPenalty * 0.02 * float64(idx)
			charScore -= leadingPenalty
		} else {
			charScore += fm.consecutiveBonus
		}
		score += charScore
	}

	trailingLen := len(textRunes) - end - 1
	if trailingLen > 20 {
		score -= fm.gapPenalty * 0.25 * float64((trailingLen-20)/10)
	}

	return score, start, end, wordHits
}

func maxIndex(values []float64) int {
	best := math.Inf(-1)
	bestIdx := -1
	for i, v := range values {
		if v > best {
			best = v
			bestIdx = i
		}
	}
	if bestIdx == -1 || best <= math.Inf(-1)/2 {
		return -1
	}
	return bestIdx
}

func isWordBoundary(buf *boundaryBuffer, text []rune, idx int) bool {
	if buf != nil {
		return buf.boundaryBits(text, idx)&boundaryWord != 0
	}
	return isWordBoundaryRune(text, idx)
}

func isStrongWordBoundary(buf *boundaryBuffer, text []rune, idx int) bool {
	if buf != nil {
		return buf.boundaryBits(text, idx)&boundaryStrong != 0
	}
	return isStrongWordBoundaryRune(text, idx)
}

func isWordBoundaryRune(text []rune, idx int) bool {
	if idx == 0 {
		return true
	}
	prev := text[idx-1]
	curr := text[idx]
	switch prev {
	case '/', '\\', '-', '_', ' ', '.', ':':
		return true
	}
	if prev <= unicode.MaxASCII && curr <= unicode.MaxASCII {
		prevByte := byte(prev)
		currByte := byte(curr)
		if !isLetterByte(prevByte) && isLetterByte(currByte) {
			return true
		}
		if prevByte >= 'a' && prevByte <= 'z' && currByte >= 'A' && currByte <= 'Z' {
			return true
		}
		return false
	}
	if !isLetterRune(prev) && isLetterRune(curr) {
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(curr) {
		return true
	}
	return false
}

func isStrongWordBoundaryRune(text []rune, idx int) bool {
	if idx == 0 {
		return true
	}
	prev := text[idx-1]
	curr := text[idx]
	switch prev {
	case '/', '\\':
		return true
	case ' ', '-':
		return true
	case '_', '.', ':':
		return false
	}
	if prev <= unicode.MaxASCII && curr <= unicode.MaxASCII {
		prevByte := byte(prev)
		currByte := byte(curr)
		if !isLetterByte(prevByte) && isLetterByte(currByte) {
			return true
		}
		if prevByte >= 'a' && prevByte <= 'z' && currByte >= 'A' && currByte <= 'Z' {
			return true
		}
		return false
	}
	if !isLetterRune(prev) && isLetterRune(curr) {
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(curr) {
		return true
	}
	return false
}

// MatchMultiple finds fuzzy matches for a pattern in a list of texts
// Returns sorted matches by score (highest first)
func (fm *FuzzyMatcher) MatchMultiple(pattern string, texts []string) []FuzzyMatch {
	return fm.MatchMultipleInto(pattern, texts, nil)
}

// MatchMultipleInto reuses the provided destination slice when collecting matches.
// The returned slice is sorted in descending score order.
func (fm *FuzzyMatcher) MatchMultipleInto(pattern string, texts []string, dst []FuzzyMatch) []FuzzyMatch {
	return fm.MatchMultipleIntoWithMode(pattern, patternHasUppercase(pattern), texts, dst)
}

// MatchMultipleIntoWithMode mirrors MatchMultipleInto but lets callers choose case sensitivity.
func (fm *FuzzyMatcher) MatchMultipleIntoWithMode(pattern string, caseSensitive bool, texts []string, dst []FuzzyMatch) []FuzzyMatch {
	matches := dst[:0]

	if pattern == "" {
		if cap(matches) < len(texts) {
			matches = make([]FuzzyMatch, len(texts))
		} else {
			matches = matches[:len(texts)]
		}
		for idx := range texts {
			matches[idx] = FuzzyMatch{
				FileIndex: idx,
				Score:     1.0,
			}
		}
		return matches
	}

	fold := !caseSensitive

	patternRunes, patternBuf := acquireRunes(pattern, fold)
	defer releaseRunes(patternBuf)

	for idx, text := range texts {
		textRunes, textBuf := acquireRunes(text, fold)
		score, matched, details, substringIdx := fm.matchWithRunes(pattern, text, patternRunes, textRunes)
		releaseRunes(textBuf)
		if matched && score > fm.minScore {
			if fuzzyDebugEnabled() {
				fuzzyLogf("multi pattern=%q text=%q score=%.6f matchCount=%d wordHits=%d substrIdx=%d",
					pattern, text, score, details.MatchCount, details.WordHits, substringIdx)
			}
			matches = append(matches, FuzzyMatch{
				FileIndex: idx,
				Score:     score,
			})
		}
	}

	slices.SortFunc(matches, func(a, b FuzzyMatch) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})

	return matches
}

// Helper functions

func isLetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isLetterRune(r rune) bool {
	if r <= unicode.MaxASCII {
		return isLetterByte(byte(r))
	}
	return unicode.IsLetter(r)
}

func runesAreASCII(rs []rune) bool {
	for _, r := range rs {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func indexRunesASCII(haystack, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	hBytes, hBuf := runeSliceToASCIIBytes(haystack)
	defer releaseByteBuffer(hBuf)
	nBytes, nBuf := runeSliceToASCIIBytes(needle)
	defer releaseByteBuffer(nBuf)
	return indexRunesASCIIBytes(hBytes, nBytes)
}

func indexRunesASCIIBytes(haystack, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	return strings.Index(asciiBytesToString(haystack), asciiBytesToString(needle))
}

func runeSliceToASCIIBytes(rs []rune) ([]byte, *byteBuffer) {
	buf := acquireByteBuffer(len(rs))
	bytes := buf.data[:len(rs)]
	for i, r := range rs {
		bytes[i] = byte(r)
	}
	return bytes, buf
}

func asciiBytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

type runeBuffer struct {
	data []rune
}

type byteBuffer struct {
	data []byte
}

var runeBufferPool = sync.Pool{
	New: func() any {
		return &runeBuffer{}
	},
}

var byteBufferPool = sync.Pool{
	New: func() any {
		return &byteBuffer{}
	},
}

func acquireRunes(s string, fold bool) ([]rune, *runeBuffer) {
	buf := runeBufferPool.Get().(*runeBuffer)
	runes := buf.data
	if !fold {
		needed := utf8.RuneCountInString(s)
		if cap(runes) < needed {
			runes = make([]rune, needed)
		} else {
			runes = runes[:needed]
		}
		idx := 0
		for _, r := range s {
			runes[idx] = r
			idx++
		}
		buf.data = runes
		return runes, buf
	}

	isASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			isASCII = false
			break
		}
	}
	if isASCII {
		needed := len(s)
		if cap(runes) < needed {
			runes = make([]rune, needed)
		} else {
			runes = runes[:needed]
		}
		if !lowerASCIIInto(runes, s) {
			for i := 0; i < len(s); i++ {
				b := s[i]
				if b >= 'A' && b <= 'Z' {
					b += 'a' - 'A'
				}
				runes[i] = rune(b)
			}
		}
	} else {
		needed := utf8.RuneCountInString(s)
		if cap(runes) < needed {
			runes = make([]rune, needed)
		} else {
			runes = runes[:needed]
		}
		idx := 0
		for _, r := range s {
			runes[idx] = unicode.ToLower(r)
			idx++
		}
	}
	buf.data = runes
	return runes, buf
}

func acquireByteBuffer(length int) *byteBuffer {
	buf := byteBufferPool.Get().(*byteBuffer)
	if cap(buf.data) < length {
		buf.data = make([]byte, length)
	}
	buf.data = buf.data[:length]
	return buf
}

func lowerASCIIInto(dst []rune, src string) bool {
	if len(src) == 0 {
		return true
	}
	if len(dst) < len(src) {
		return false
	}
	return lowerASCIIUsingPlatform(dst, src)
}

func releaseRunes(buf *runeBuffer) {
	if buf == nil {
		return
	}
	buf.data = buf.data[:0]
	runeBufferPool.Put(buf)
}

func releaseByteBuffer(buf *byteBuffer) {
	if buf == nil {
		return
	}
	buf.data = buf.data[:0]
	byteBufferPool.Put(buf)
}

type boundaryBuffer struct {
	flags      []uint8
	gens       []uint32
	generation uint32
}

var boundaryBufferPool = sync.Pool{
	New: func() any {
		return &boundaryBuffer{}
	},
}

func acquireBoundaryBuffer(length int) *boundaryBuffer {
	buf := boundaryBufferPool.Get().(*boundaryBuffer)
	if cap(buf.flags) < length {
		buf.flags = make([]uint8, length)
		buf.gens = make([]uint32, length)
	}
	buf.flags = buf.flags[:length]
	buf.gens = buf.gens[:length]
	buf.generation++
	if buf.generation == 0 {
		for i := range buf.gens {
			buf.gens[i] = 0
		}
		buf.generation = 1
	}
	return buf
}

func releaseBoundaryBuffer(buf *boundaryBuffer) {
	if buf == nil {
		return
	}
	buf.flags = buf.flags[:0]
	buf.gens = buf.gens[:0]
	boundaryBufferPool.Put(buf)
}

func (b *boundaryBuffer) boundaryBits(text []rune, idx int) uint8 {
	if b == nil || idx < 0 || idx >= len(b.flags) {
		return 0
	}
	if b.gens[idx] == b.generation {
		return b.flags[idx]
	}
	var value uint8
	if idx == 0 {
		value = boundaryWord | boundaryStrong
	} else {
		if isWordBoundaryRune(text, idx) {
			value |= boundaryWord
		}
		if isStrongWordBoundaryRune(text, idx) {
			value |= boundaryStrong
		}
	}
	b.flags[idx] = value
	b.gens[idx] = b.generation
	return value
}

func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
	if runesAreASCII(haystack) && runesAreASCII(needle) {
		if idx := indexRunesASCII(haystack, needle); idx >= 0 {
			return idx
		}
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i] != needle[0] {
			continue
		}
		for j := 1; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

var fuzzyDebugEnv = os.Getenv("RDIR_DEBUG_FUZZY") == "1"
var fuzzyDebugFile = os.Getenv("RDIR_DEBUG_FUZZY_FILE")
var fuzzySIMDDPDisabled = os.Getenv("RDIR_DISABLE_SIMD_DP") == "1"

func fuzzyDebugEnabled() bool {
	return fuzzyDebugEnv
}

func fuzzySIMDDPEnabled() bool {
	return !fuzzySIMDDPDisabled
}

// Experimental: enable float32 ASCII DP path (NEON-accelerated primitives) via env.
var fuzzyASCII32DPEnabled = os.Getenv("RDIR_EXPERIMENTAL_ASCII_DP32") == "1"

// test-only override; same package tests can set this.
var ascii32Force bool

var ascii32Debug = os.Getenv("RDIR_DEBUG_ASCII32") == "1"
var ascii32PrefixASMDisabled = os.Getenv("RDIR_DISABLE_ASCII32_PREFIX_ASM") == "1"
var ascii32VerifyPrefixASM = os.Getenv("RDIR_VERIFY_ASCII32_PREFIX_ASM") == "1"

const (
	ascii32MaxRows    = 128
	ascii32MaxCols    = 4096
	ascii32BeamWidth  = 96
	ascii32BeamMargin = 48
	ascii32ChunkWidth = 8
)

func fuzzyASCII32Enabled() bool { return fuzzyASCII32DPEnabled || ascii32Force }

type dpScratch struct {
	dpPrev           []float64
	dpCurr           []float64
	prefix           []float64
	prefixIdx        []int
	matchCols        []int
	matchCols32      []int32
	dpPrev32         []float32
	dpCurr32         []float32
	prefix32         []float32
	prefixIdx32      []int32
	matchPrefix32    []float32
	matchPrefixIdx32 []int32
	matchPrev32      []float32
	matchPrevIdx     []int32
	backtrack        []int
	backtrackGen     []uint32
	cols             int
	rows             int
	generation       uint32
}

var dpScratchPool = sync.Pool{
	New: func() any {
		return &dpScratch{}
	},
}

func acquireDPScratch(rows, cols int) *dpScratch {
	s := dpScratchPool.Get().(*dpScratch)
	if cap(s.dpPrev) < cols {
		s.dpPrev = make([]float64, cols)
	}
	if cap(s.dpCurr) < cols {
		s.dpCurr = make([]float64, cols)
	}
	if cap(s.prefix) < cols {
		s.prefix = make([]float64, cols)
	}
	if cap(s.prefixIdx) < cols {
		s.prefixIdx = make([]int, cols)
	}
	if cap(s.matchCols) < cols {
		s.matchCols = make([]int, cols)
	}
	if cap(s.matchCols32) < cols {
		s.matchCols32 = make([]int32, cols)
	}
	if cap(s.dpPrev32) < cols {
		s.dpPrev32 = make([]float32, cols)
	}
	if cap(s.dpCurr32) < cols {
		s.dpCurr32 = make([]float32, cols)
	}
	if cap(s.prefix32) < cols {
		s.prefix32 = make([]float32, cols)
	}
	if cap(s.prefixIdx32) < cols {
		s.prefixIdx32 = make([]int32, cols)
	}
	if cap(s.matchPrefix32) < cols {
		s.matchPrefix32 = make([]float32, cols)
	}
	if cap(s.matchPrefixIdx32) < cols {
		s.matchPrefixIdx32 = make([]int32, cols)
	}
	if cap(s.matchPrev32) < cols {
		s.matchPrev32 = make([]float32, cols)
	}
	if cap(s.matchPrevIdx) < cols {
		s.matchPrevIdx = make([]int32, cols)
	}
	required := rows * cols
	if cap(s.backtrack) < required {
		s.backtrack = make([]int, required)
	}
	if cap(s.backtrackGen) < required {
		s.backtrackGen = make([]uint32, required)
	}
	s.dpPrev = s.dpPrev[:cols]
	s.dpCurr = s.dpCurr[:cols]
	s.prefix = s.prefix[:cols]
	s.prefixIdx = s.prefixIdx[:cols]
	s.matchCols = s.matchCols[:cols]
	s.matchCols32 = s.matchCols32[:cols]
	s.dpPrev32 = s.dpPrev32[:cols]
	s.dpCurr32 = s.dpCurr32[:cols]
	s.prefix32 = s.prefix32[:cols]
	s.prefixIdx32 = s.prefixIdx32[:cols]
	s.matchPrefix32 = s.matchPrefix32[:cols]
	s.matchPrefixIdx32 = s.matchPrefixIdx32[:cols]
	s.matchPrev32 = s.matchPrev32[:cols]
	s.matchPrevIdx = s.matchPrevIdx[:cols]
	s.backtrack = s.backtrack[:required]
	s.backtrackGen = s.backtrackGen[:required]
	s.generation++
	if s.generation == 0 {
		for i := range s.backtrackGen {
			s.backtrackGen[i] = 0
		}
		s.generation = 1
	}
	s.rows = rows
	s.cols = cols
	return s
}

func releaseDPScratch(s *dpScratch) {
	// Keep slices for reuse; simply reset bookkeeping.
	s.rows = 0
	s.cols = 0
	dpScratchPool.Put(s)
}

func fuzzyLogf(format string, args ...any) {
	if fuzzyDebugFile == "" {
		fmt.Printf("[fuzzy-debug] "+format+"\n", args...)
		return
	}
	abspath := fuzzyDebugFile
	if !filepath.IsAbs(abspath) {
		cwd, err := os.Getwd()
		if err == nil {
			abspath = filepath.Join(cwd, fuzzyDebugFile)
		}
	}
	f, err := os.OpenFile(abspath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Printf("[fuzzy-debug] open file error: %v\n", err)
		fmt.Printf("[fuzzy-debug] "+format+"\n", args...)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Printf("[fuzzy-debug] close file error: %v\n", cerr)
		}
	}()
	if _, err := fmt.Fprintf(f, "[fuzzy-debug] "+format+"\n", args...); err != nil {
		fmt.Printf("[fuzzy-debug] write file error: %v\n", err)
	}
}
