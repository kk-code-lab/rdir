//go:build !arm64 || purego

package search

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
