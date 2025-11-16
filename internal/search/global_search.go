package search

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	maxDisplayResults         = 10000
	defaultMaxIndexResults    = 1000000
	envMaxIndexResults        = "RDIR_INDEX_MAX_RESULTS"
	envIndexMaxWorkers        = "RDIR_INDEX_MAX_WORKERS"
	indexProgressInterval     = 150 * time.Millisecond
	batchIntervalFast         = 75 * time.Millisecond
	batchIntervalSlow         = 200 * time.Millisecond
	batchForceSize            = 400
	batchFastThreshold        = 40
	initialImmediateBatchSize = 10
	mergeStatusMinimumResults = 200
	indexStreamBatchSize      = 128
)

// indexSnapshot captures the current state of the incremental index build.
type indexSnapshot struct {
	Count int
	Ready bool
	Err   error
}

// indexObserver delivers streaming index updates to a subscriber.
type indexObserver struct {
	gs *GlobalSearcher
	id int
	ch chan indexSnapshot
}

// GlobalSearcher handles recursive directory searching with fuzzy matching.
type GlobalSearcher struct {
	matcher        *FuzzyMatcher
	rootPath       string
	ignoreProvider *ignoreProvider
	hideHidden     bool

	maxIndexResults int
	progress        IndexTelemetry
	progressCb      func(IndexTelemetry)

	indexMu          sync.Mutex
	indexEntries     []indexedEntry
	indexRuneBuckets map[rune][]int
	indexTotalFiles  int
	cache            *searchCache
	indexGen         int
	indexReady       bool
	indexErr         error
	indexBuilding    bool
	indexWatchers    map[int]chan indexSnapshot
	nextWatcherID    int
	pendingBroadcast int

	cancelMu sync.Mutex
	cancel   context.CancelFunc
	token    int
}

// NewGlobalSearcher creates a new global searcher from a root path.
func NewGlobalSearcher(rootPath string, hideHidden bool, progressCb func(IndexTelemetry)) *GlobalSearcher {
	maxIndexResults := parseEnvInt(envMaxIndexResults, defaultMaxIndexResults)
	if maxIndexResults < maxDisplayResults {
		maxIndexResults = maxDisplayResults
	}

	progress := IndexTelemetry{
		RootPath:        rootPath,
		MaxIndexResults: maxIndexResults,
		UseIndex:        true,
		Disabled:        false,
	}

	gs := &GlobalSearcher{
		matcher:         NewFuzzyMatcher(),
		rootPath:        rootPath,
		ignoreProvider:  newIgnoreProvider(rootPath),
		hideHidden:      hideHidden,
		maxIndexResults: maxIndexResults,
		progress:        progress,
		progressCb:      progressCb,
		indexWatchers:   make(map[int]chan indexSnapshot),
		cache:           newSearchCache(),
	}

	return gs
}

// SearchRecursive performs a blocking search by delegating to the async pipeline.
func (gs *GlobalSearcher) SearchRecursive(query string, caseSensitive bool) []GlobalSearchResult {
	done := make(chan []GlobalSearchResult, 1)
	gs.SearchRecursiveAsync(query, caseSensitive, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if !isDone || inProgress {
			return
		}
		select {
		case done <- results:
		default:
		}
	})
	return <-done
}

// RootPath returns the directory where the searcher was initialized.
func (gs *GlobalSearcher) RootPath() string {
	return gs.rootPath
}

// HideHidden reports whether the searcher filters out hidden filesystem entries.
func (gs *GlobalSearcher) HideHidden() bool {
	return gs.hideHidden
}

// CurrentProgress exposes the latest indexing telemetry snapshot.
func (gs *GlobalSearcher) CurrentProgress() IndexTelemetry {
	return gs.currentProgress()
}

// CachedResults returns cached results for the exact query if available.
func (gs *GlobalSearcher) CachedResults(query string, caseSensitive bool) ([]GlobalSearchResult, bool) {
	return gs.lookupCache(query, caseSensitive)
}

// CancelOngoingSearch stops any in-flight work.
func (gs *GlobalSearcher) CancelOngoingSearch() {
	gs.cancelOngoingSearch()
}

// UsingIndex reports whether the searcher can currently answer queries from the index.
func (gs *GlobalSearcher) UsingIndex() bool {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	return gs.indexReady && len(gs.indexEntries) > 0
}

// ensureIndexStream starts the indexing goroutine if it is not already running.
func (gs *GlobalSearcher) ensureIndexStream() {
	gs.indexMu.Lock()
	if gs.indexReady || gs.indexBuilding {
		gs.indexMu.Unlock()
		return
	}
	gs.indexBuilding = true
	gs.indexMu.Unlock()

	start := time.Now()
	gs.emitProgress(func(p *IndexTelemetry) {
		p.Building = true
		p.Ready = false
		p.Disabled = false
		p.StartedAt = start
		p.UpdatedAt = start
		p.CompletedAt = time.Time{}
		p.FilesIndexed = len(gs.indexEntries)
		p.LastError = ""
	})

	go gs.buildIndex(start)
}

// newIndexObserver subscribes to incremental index updates.
func (gs *GlobalSearcher) newIndexObserver() *indexObserver {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()

	if gs.indexWatchers == nil {
		gs.indexWatchers = make(map[int]chan indexSnapshot)
	}

	id := gs.nextWatcherID
	gs.nextWatcherID++
	ch := make(chan indexSnapshot, 1)
	ch <- indexSnapshot{Count: len(gs.indexEntries), Ready: gs.indexReady, Err: gs.indexErr}
	gs.indexWatchers[id] = ch
	return &indexObserver{gs: gs, id: id, ch: ch}
}

// broadcastSnapshotLocked notifies all observers about the latest index state.
func (gs *GlobalSearcher) broadcastSnapshotLocked() {
	snapshot := indexSnapshot{Count: len(gs.indexEntries), Ready: gs.indexReady, Err: gs.indexErr}
	for id, ch := range gs.indexWatchers {
		if !sendSnapshot(ch, snapshot) {
			delete(gs.indexWatchers, id)
		}
	}
}

// sendSnapshot pushes the latest snapshot to a watcher without blocking.
func sendSnapshot(ch chan indexSnapshot, snap indexSnapshot) bool {
	if ch == nil {
		return false
	}
	select {
	case ch <- snap:
		return true
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snap:
			return true
		default:
			return false
		}
	}
}

func (obs *indexObserver) Next(ctx context.Context) (indexSnapshot, bool) {
	if obs == nil {
		return indexSnapshot{}, false
	}
	select {
	case snap, ok := <-obs.ch:
		if !ok {
			return indexSnapshot{}, false
		}
		return snap, true
	case <-ctx.Done():
		return indexSnapshot{}, false
	}
}

func (obs *indexObserver) Close() {
	if obs == nil || obs.gs == nil {
		return
	}
	obs.gs.indexMu.Lock()
	if ch, ok := obs.gs.indexWatchers[obs.id]; ok && ch == obs.ch {
		delete(obs.gs.indexWatchers, obs.id)
		close(ch)
	}
	obs.gs.indexMu.Unlock()
}

func parseEnvInt(name string, fallback int) int {
	val := os.Getenv(name)
	if val == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func (gs *GlobalSearcher) incrementIndexGeneration() {
	gs.indexMu.Lock()
	gs.indexGen++
	gs.indexMu.Unlock()
}

func (gs *GlobalSearcher) indexGeneration() int {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	return gs.indexGen
}

func (gs *GlobalSearcher) lookupCache(query string, caseSensitive bool) ([]GlobalSearchResult, bool) {
	if gs.cache == nil {
		return nil, false
	}
	key := cacheKey{
		rootPath: gs.rootPath,
		query:    normalizeCacheQuery(query, caseSensitive),
		caseSens: caseSensitive,
		indexGen: gs.indexGeneration(),
	}
	return gs.cache.get(key)
}

func (gs *GlobalSearcher) storeCache(query string, caseSensitive bool, results []GlobalSearchResult) {
	if gs.cache == nil || len(results) == 0 {
		return
	}
	key := cacheKey{
		rootPath: gs.rootPath,
		query:    normalizeCacheQuery(query, caseSensitive),
		caseSens: caseSensitive,
		indexGen: gs.indexGeneration(),
	}
	gs.cache.put(key, results)
}
