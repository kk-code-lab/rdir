package search

import (
	"context"
	"os"
	"sync"
	"time"
)

const (
	maxDisplayResults         = 10000
	defaultIndexThreshold     = 0
	defaultMaxIndexResults    = 1000000
	envDisableIndex           = "RDIR_DISABLE_INDEX"
	envIndexThreshold         = "RDIR_INDEX_THRESHOLD"
	envMaxIndexResults        = "RDIR_INDEX_MAX_RESULTS"
	envIndexMaxWorkers        = "RDIR_INDEX_MAX_WORKERS"
	indexProgressInterval     = 150 * time.Millisecond
	maxIndexArenaSize         = int64(^uint32(0))
	batchIntervalFast         = 75 * time.Millisecond
	batchIntervalSlow         = 200 * time.Millisecond
	batchForceSize            = 400
	batchFastThreshold        = 40
	initialImmediateBatchSize = 10
	mergeStatusMinimumResults = 200
)

// GlobalSearcher handles recursive directory searching with fuzzy matching
type GlobalSearcher struct {
	matcher           *FuzzyMatcher
	rootPath          string
	ignoreProvider    *ignoreProvider
	hideHidden        bool
	useIndex          bool
	indexEntries      []indexedEntry
	indexPathArena    []byte
	indexLowerArena   []byte
	indexDisplayOrder []int
	indexErr          error
	indexReady        bool
	indexThreshold    int
	maxIndexResults   int
	progress          IndexTelemetry
	progressCb        func(IndexTelemetry)

	indexMu        sync.Mutex
	indexBuilding  bool
	indexBuildHint bool

	cancelMu sync.Mutex
	cancel   context.CancelFunc
	token    int
}

// NewGlobalSearcher creates a new global searcher from a root path
func NewGlobalSearcher(rootPath string, hideHidden bool, progressCb func(IndexTelemetry)) *GlobalSearcher {
	// Use rootPath as-is (don't normalize, as it's already from state.CurrentPath)
	// This preserves the path as the user sees it

	useIndex := os.Getenv(envDisableIndex) != "1"
	indexThreshold := parseEnvInt(envIndexThreshold, defaultIndexThreshold)
	maxIndexResults := parseEnvInt(envMaxIndexResults, defaultMaxIndexResults)
	if maxIndexResults < maxDisplayResults {
		maxIndexResults = maxDisplayResults
	}

	progress := IndexTelemetry{
		RootPath:        rootPath,
		Threshold:       indexThreshold,
		MaxIndexResults: maxIndexResults,
		UseIndex:        useIndex,
		Disabled:        !useIndex,
	}

	return &GlobalSearcher{
		matcher:         NewFuzzyMatcher(),
		rootPath:        rootPath,
		ignoreProvider:  newIgnoreProvider(rootPath),
		hideHidden:      hideHidden,
		useIndex:        useIndex,
		indexThreshold:  indexThreshold,
		maxIndexResults: maxIndexResults,
		progress:        progress,
		progressCb:      progressCb,
	}
}

// SearchRecursive performs global search from rootPath
// Returns sorted results by fuzzy match score
func (gs *GlobalSearcher) SearchRecursive(query string, caseSensitive bool) []GlobalSearchResult {
	if gs.maybeUseIndex() {
		return gs.searchIndex(query, caseSensitive)
	}

	return gs.searchWalk(query, caseSensitive)
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

// CancelOngoingSearch stops any in-flight work.
func (gs *GlobalSearcher) CancelOngoingSearch() {
	gs.cancelOngoingSearch()
}

// UsingIndex reports whether the searcher can currently answer queries from the index.
func (gs *GlobalSearcher) UsingIndex() bool {
	ready, count, use := gs.indexSnapshot()
	return use && ready && count > 0
}

// IndexThreshold returns the threshold configured for the searcher.
func (gs *GlobalSearcher) IndexThreshold() int {
	return gs.indexThreshold
}

// TriggerIndexBuild starts the index build asynchronously if it is enabled and not already running.
func (gs *GlobalSearcher) TriggerIndexBuild() {
	gs.indexMu.Lock()
	if !gs.useIndex || gs.indexReady || gs.indexBuilding {
		gs.indexMu.Unlock()
		return
	}
	gs.indexBuilding = true
	gs.indexBuildHint = true
	gs.indexMu.Unlock()

	start := time.Now()
	gs.emitProgress(func(p *IndexTelemetry) {
		p.Building = true
		p.Ready = false
		p.Disabled = false
		p.StartedAt = start
		p.UpdatedAt = start
		p.CompletedAt = time.Time{}
		p.Duration = 0
		p.FilesIndexed = 0
		p.LastError = ""
	})

	go gs.buildIndex(start)
}
