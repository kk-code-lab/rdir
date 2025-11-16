package search

import "sync"

var resultSlicePool = sync.Pool{
	New: func() any {
		buf := make([]GlobalSearchResult, 0, 256)
		return &buf
	},
}

func borrowResultBuffer(sizeHint int) []GlobalSearchResult {
	if sizeHint <= 0 {
		sizeHint = 256
	}

	if v := resultSlicePool.Get(); v != nil {
		bufPtr := v.(*[]GlobalSearchResult)
		buf := *bufPtr
		if cap(buf) < sizeHint {
			newBuf := make([]GlobalSearchResult, 0, sizeHint)
			return newBuf
		}
		return buf[:0]
	}

	return make([]GlobalSearchResult, 0, sizeHint)
}

func releaseResultBuffer(buf []GlobalSearchResult) {
	if buf == nil {
		return
	}
	if cap(buf) > 1<<18 { // avoid holding on to very large arrays (~262k entries)
		return
	}
	buf = buf[:0]
	resultSlicePool.Put(&buf)
}

func growResultBuffer(buf []GlobalSearchResult, minCap int) []GlobalSearchResult {
	oldCap := cap(buf)
	if minCap <= oldCap {
		return buf
	}

	newCap := oldCap * 2
	if newCap < minCap {
		newCap = minCap
	}
	if newCap < 256 {
		newCap = 256
	}

	newBuf := borrowResultBuffer(newCap)
	newBuf = append(newBuf, buf...)
	releaseResultBuffer(buf)
	return newBuf
}

var candidateIndexPool = sync.Pool{
	New: func() any {
		buf := make([]int, 0, 1024)
		return &buf
	},
}

func borrowCandidateBuffer(sizeHint int) []int {
	if sizeHint <= 0 {
		sizeHint = 1024
	}
	if v := candidateIndexPool.Get(); v != nil {
		bufPtr := v.(*[]int)
		buf := *bufPtr
		if cap(buf) < sizeHint {
			return make([]int, 0, sizeHint)
		}
		return buf[:0]
	}
	return make([]int, 0, sizeHint)
}

func releaseCandidateBuffer(buf []int) {
	if buf == nil {
		return
	}
	if cap(buf) > 1<<17 { // avoid holding very large buffers (~131k)
		return
	}
	buf = buf[:0]
	candidateIndexPool.Put(&buf)
}
