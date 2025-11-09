package search

import "math"

func scalarPrefixRef(prefix []float32, prefixIdx []int32, dpPrev []float32, start, end int, gap float32) {
	// This scalar helper mirrors the existing DP prefix logic and serves as
	// the authoritative reference while the NEON implementation is under
	// construction. NEON code must produce byte-for-byte identical outputs.
	if len(prefix) == 0 || len(prefixIdx) == 0 || len(dpPrev) == 0 {
		return
	}
	if start < 0 {
		start = 0
	}
	if end >= len(prefix) {
		end = len(prefix) - 1
	}
	if start > end {
		return
	}
	bestScore := float32(math.Inf(-1))
	bestIdx := int32(-1)
	for j := start; j <= end; j++ {
		if bestIdx != -1 {
			bestScore -= gap
		}
		if j > 0 {
			candidate := dpPrev[j-1]
			if candidate > bestScore {
				bestScore = candidate
				bestIdx = int32(j - 1)
			}
		}
		prefix[j] = bestScore
		prefixIdx[j] = bestIdx
	}
}
