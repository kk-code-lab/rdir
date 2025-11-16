//go:build arm64 && !purego

package search

import (
	"fmt"

	"golang.org/x/sys/cpu"
)

const ascii32PrefixVerifyTolerance = 1e-6

//go:noescape
func fuzzyPrefixMaxASCIIAsm(prefix *float32, prefixIdx *int32, dpPrev *float32, count int, gap float32)

//go:noescape
func fuzzyWriteOneAsm(prefix *float32, prefixIdx *int32)

//go:noescape
func fuzzySetIdxRangeAsm(prefixIdx *int32, count int)

//go:noescape
func fuzzyCopyRangeF32Asm(dst *float32, src *float32, count int)

//go:noescape
func fuzzySetRangeF32NegInfAsm(dst *float32, count int)

func (fm *FuzzyMatcher) matchRunesDPASCII(pattern, text []rune, boundaryBuf *boundaryBuffer, asciiText, asciiPattern []byte, spanMode spanRequest) (float64, bool, int, int, int, int, int, []MatchSpan, []int, bool) {
	if !cpu.ARM64.HasASIMD {
		score, matched, start, end, targetLen, matchCount, wordHits, spans, positions := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern, spanMode)
		return score, matched, start, end, targetLen, matchCount, wordHits, spans, positions, true
	}

	score, matched, start, end, targetLen, matchCount, wordHits, spans, positions := fm.matchRunesDPScalar(pattern, text, boundaryBuf, asciiText, asciiPattern, spanMode)
	return score, matched, start, end, targetLen, matchCount, wordHits, spans, positions, true
}

func runPrefixASCII(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	if len(prefix) == 0 || len(prefixIdx) == 0 || len(dpPrev) == 0 {
		return
	}
	if start < 0 {
		start = 0
	}
	if end >= len(prefix) {
		end = len(prefix) - 1
	}
	if end >= len(prefixIdx) {
		end = len(prefixIdx) - 1
	}
	if start > end {
		return
	}
	if !ascii32PrefixASMDisabled {
		cnt := end - start + 1
		if cnt > 0 {
			fuzzyPrefixMaxASCIIAsm(&prefix[start], &prefixIdx[start], &dpPrev[start], cnt, gap)
			if ascii32VerifyPrefixASM {
				verifyLen := end + 1
				if verifyLen < 0 {
					verifyLen = 0
				}
				verifyPrefix := make([]float32, verifyLen)
				verifyPrefixIdx := make([]int32, verifyLen)
				scalarPrefixRef(verifyPrefix, verifyPrefixIdx, dpPrev, start, end, gap)
				for idx := start; idx <= end; idx++ {
					delta := prefix[idx] - verifyPrefix[idx]
					if delta < 0 {
						delta = -delta
					}
					if delta > ascii32PrefixVerifyTolerance || prefixIdx[idx] != verifyPrefixIdx[idx] {
						copy(prefix[start:end+1], verifyPrefix[start:end+1])
						copy(prefixIdx[start:end+1], verifyPrefixIdx[start:end+1])
						msg := fmt.Sprintf("ascii32 prefix asm mismatch at column %d: asm=(%f,%d) scalar=(%f,%d)", idx, prefix[idx], prefixIdx[idx], verifyPrefix[idx], verifyPrefixIdx[idx])
						panic(msg)
					}
				}
			}
		}
	}

	// The scalar reference remains authoritative for production paths; asm writes
	// above serve as experimental hooks gated by env flags and are overwritten
	// here to ensure determinism.
	scalarPrefixRef(prefix, prefixIdx, dpPrev, start, end, gap)
}

// (removed old direct-asm wrapper; see asmCopyPrefixRange below)

// asmCopyOne is a tiny wrapper to verify that writing through pointers from
// asm affects the Go slices as expected.
func asmCopyOne(prefix []float32, prefixIdx []int32, i int) {
	if i < 0 || i >= len(prefix) || i >= len(prefixIdx) {
		return
	}
	fuzzyWriteOneAsm(&prefix[i], &prefixIdx[i])
}

// asmSetIdxRange sets prefixIdx[start:end] = -1 via asm.
func asmSetIdxRange(prefixIdx []int32, start, end int) {
	if len(prefixIdx) == 0 {
		return
	}
	if start < 0 {
		start = 0
	}
	if end >= len(prefixIdx) {
		end = len(prefixIdx) - 1
	}
	if start > end {
		return
	}
	cnt := end - start + 1
	fuzzySetIdxRangeAsm(&prefixIdx[start], cnt)
}

// asmCopyRangeF32 copies src[start:end] into dst[start:end] via asm.
func asmCopyRangeF32(dst []float32, src []float32, start, end int) {
	if len(dst) == 0 || len(src) == 0 {
		return
	}
	if start < 0 {
		start = 0
	}
	if end >= len(dst) {
		end = len(dst) - 1
	}
	if end >= len(src) {
		end = len(src) - 1
	}
	if start > end {
		return
	}
	cnt := end - start + 1
	fuzzyCopyRangeF32Asm(&dst[start], &src[start], cnt)
}

// High-level safe range copy used by tests: sets idx range first, then copies floats.
func asmCopyPrefixRange(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	asmSetIdxRange(prefixIdx, start, end)
	asmCopyRangeF32(prefix, dpPrev, start, end)
}
