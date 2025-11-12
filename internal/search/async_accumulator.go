package search

import (
	"sort"
	"sync"
	"time"
)

type asyncAccumulator struct {
	mu             sync.Mutex
	results        []GlobalSearchResult
	sorted         []GlobalSearchResult
	lastSize       int
	lastTime       time.Time
	callback       func([]GlobalSearchResult, bool, bool)
	immediateFlush bool
}

func newAsyncAccumulator(capacity int, callback func([]GlobalSearchResult, bool, bool), immediateFlush bool) *asyncAccumulator {
	if capacity <= 0 {
		capacity = 256
	}
	buf := borrowResultBuffer(capacity)
	return &asyncAccumulator{
		results:        buf,
		lastTime:       time.Now(),
		callback:       callback,
		immediateFlush: immediateFlush,
	}
}

func (a *asyncAccumulator) Close() {
	releaseResultBuffer(a.results)
	releaseResultBuffer(a.sorted)
}

func (a *asyncAccumulator) Add(result GlobalSearchResult) {
	a.mu.Lock()
	if len(a.results) == cap(a.results) {
		a.results = growResultBuffer(a.results, len(a.results)+1)
	}
	a.results = append(a.results, result)
	shouldForce := a.immediateFlush && len(a.results) <= initialImmediateBatchSize
	a.mu.Unlock()

	if shouldForce {
		a.Flush(true)
	}
}

func (a *asyncAccumulator) Flush(force bool) {
	chunk := a.collectChunk(force)
	if chunk == nil {
		return
	}
	sort.Slice(chunk, func(i, j int) bool {
		return compareResults(chunk[i], chunk[j]) < 0
	})
	a.emitChunk(chunk, false, true)
}

func (a *asyncAccumulator) collectChunk(force bool) []GlobalSearchResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	current := len(a.results)
	lastSize := a.lastSize
	lastTime := a.lastTime
	if !force && !shouldFlushBatch(lastSize, current, lastTime) {
		return nil
	}
	if current <= lastSize {
		return nil
	}
	chunkSize := current - lastSize
	chunk := borrowResultBuffer(chunkSize)
	chunk = chunk[:chunkSize]
	copy(chunk, a.results[lastSize:current])
	a.lastSize = current
	a.lastTime = time.Now()
	return chunk
}

func (a *asyncAccumulator) FlushRemaining() {
	chunk := a.collectChunk(true)
	if chunk == nil {
		return
	}
	sort.Slice(chunk, func(i, j int) bool {
		return compareResults(chunk[i], chunk[j]) < 0
	})
	a.emitChunk(chunk, true, true)
}

func (a *asyncAccumulator) FinalResults() []GlobalSearchResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	results := a.sorted
	if len(results) > maxDisplayResults {
		results = results[:maxDisplayResults]
	}
	copyBuf := make([]GlobalSearchResult, len(results))
	copy(copyBuf, results)
	return copyBuf
}

func (a *asyncAccumulator) emitChunk(chunk []GlobalSearchResult, isDone, inProgress bool) {
	defer releaseResultBuffer(chunk)

	a.mu.Lock()
	prev := a.sorted
	a.sorted = mergeResults(a.sorted, chunk)
	releaseResultBuffer(prev)
	snapshot := a.sorted
	if len(snapshot) > maxDisplayResults {
		snapshot = snapshot[:maxDisplayResults]
	}
	copyBuf := make([]GlobalSearchResult, len(snapshot))
	copy(copyBuf, snapshot)
	a.mu.Unlock()

	a.callback(copyBuf, isDone, inProgress)
}
