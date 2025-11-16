package search

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

type indexedEntry struct {
	fullPath    string
	relPath     string
	lowerPath   string
	size        int64
	modUnixNano int64
	mode        uint32
	order       int
}

type indexFileRecord struct {
	fullPath    string
	relPath     string
	size        int64
	modUnixNano int64
	mode        uint32
}

func (gs *GlobalSearcher) searchIndex(query string, caseSensitive bool) []GlobalSearchResult {
	tokens, matchAll := prepareQueryTokens(query, caseSensitive)
	gs.orderTokens(tokens)
	entries := gs.snapshotEntries(0, -1)
	if matchAll {
		return gs.collectAllIndexFrom(entries)
	}

	collector := newTopCollector(maxDisplayResults)
	candidates := gs.indexCandidates(tokens)
	if len(candidates) == 0 {
		candidates = make([]int, len(entries))
		for i := range entries {
			candidates[i] = i
		}
	}

	for _, idx := range candidates {
		if idx < 0 || idx >= len(entries) {
			continue
		}
		entry := &entries[idx]
		relPath := entry.relPath
		score, matched, details := gs.matchTokens(tokens, relPath, caseSensitive, matchAll)
		if !matched {
			continue
		}

		score += computeSegmentBoost(query, relPath, details)

		pathLength := details.TargetLength
		if pathLength == 0 {
			pathLength = utf8.RuneCountInString(relPath)
		}
		pathSegments := countPathSegments(relPath)

		if !collector.Needs(score, pathLength, details.Start, details.End, details.MatchCount, details.WordHits, pathSegments, entry.order, true) {
			continue
		}

		result := gs.makeIndexedResult(entry, score, pathLength, details.Start, details.End, details.MatchCount, details.WordHits, pathSegments, true, details.Spans)
		collector.Store(result)
	}

	return collector.Results()
}

func (gs *GlobalSearcher) collectAllIndexFrom(entries []indexedEntry) []GlobalSearchResult {
	if len(entries) == 0 {
		return nil
	}

	limit := len(entries)
	if limit > maxDisplayResults {
		limit = maxDisplayResults
	}

	results := make([]GlobalSearchResult, 0, limit)
	for i := 0; i < limit; i++ {
		entry := &entries[i]
		pathLength := utf8.RuneCountInString(entry.relPath)
		segments := countPathSegments(entry.relPath)
		results = append(results, gs.makeIndexedResult(entry, 1.0, pathLength, -1, -1, 0, 0, segments, false, nil))
	}
	return results
}

func (gs *GlobalSearcher) indexCandidates(tokens []queryToken) []int {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()

	total := len(gs.indexEntries)
	if total == 0 || len(tokens) == 0 || gs.indexRuneBuckets == nil {
		return makeSequentialIndexes(total)
	}

	fRune := firstRune(strings.ToLower(tokens[0].pattern))
	if fRune == 0 {
		return makeSequentialIndexes(total)
	}
	if bucket, ok := gs.indexRuneBuckets[fRune]; ok && len(bucket) > 0 {
		out := make([]int, len(bucket))
		copy(out, bucket)
		return out
	}
	return makeSequentialIndexes(total)
}

func (gs *GlobalSearcher) buildIndex(start time.Time) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := newProgressTracker(start, indexProgressInterval, gs.emitProgress)
	progressDebugf("buildIndex start root=%s", gs.rootPath)

	gs.indexMu.Lock()
	initialCap := intMin(gs.maxIndexResults, 1024)
	gs.indexEntries = make([]indexedEntry, 0, initialCap)
	gs.indexRuneBuckets = make(map[rune][]int)
	gs.indexReady = false
	gs.indexErr = nil
	gs.pendingBroadcast = 0
	gs.broadcastSnapshotLocked()
	gs.indexMu.Unlock()

	workerCount := runtime.NumCPU() - 1
	if workerCount < 2 {
		workerCount = 2
	}
	defaultMaxWorkers := 8
	maxWorkers := parseEnvInt(envIndexMaxWorkers, 0)
	if maxWorkers > 0 {
		if maxWorkers < 1 {
			maxWorkers = 1
		}
		if maxWorkers < workerCount {
			workerCount = maxWorkers
		}
	} else {
		workerCount = clampInt(workerCount, 2, defaultMaxWorkers)
	}

	dirBuffer := clampInt(workerCount*8, 32, 1024)
	fileBuffer := clampInt(workerCount*64, 512, 16384)

	dirJobs := make(chan string, dirBuffer)
	fileResults := make(chan indexFileRecord, fileBuffer)

	var workerWG sync.WaitGroup
	var pendingDirs atomic.Int64
	pendingDirs.Store(1)
	var closeDirJobsOnce sync.Once

	closeDirJobs := func() {
		closeDirJobsOnce.Do(func() {
			close(dirJobs)
		})
	}

	processDir := func(dir string) []string {
		relDir, relErr := filepath.Rel(gs.rootPath, dir)
		if relErr != nil || relDir == "" {
			relDir = "."
		}
		dirKey := normalizeDirKey(relDir)
		dirMatcher := gs.ignoreProvider.MatcherFor(dirKey)

		entriesDir, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}

		childDirs := make([]string, 0, len(entriesDir))
		for _, entry := range entriesDir {
			if ctx.Err() != nil {
				return childDirs
			}

			fullPath := filepath.Join(dir, entry.Name())
			relPath, relErr := filepath.Rel(gs.rootPath, fullPath)
			if relErr != nil {
				relPath = filepath.Join(relDir, entry.Name())
			}

			if skip, _ := gs.shouldSkip(relPath, entry, fullPath, dirMatcher); skip {
				continue
			}

			if entry.IsDir() {
				childDirs = append(childDirs, fullPath)
				childKey := normalizeDirKey(relPath)
				gs.ignoreProvider.MatcherFor(childKey)
				continue
			}

			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}

			select {
			case fileResults <- indexFileRecord{
				fullPath:    fullPath,
				relPath:     relPath,
				size:        info.Size(),
				modUnixNano: info.ModTime().UnixNano(),
				mode:        uint32(info.Mode()),
			}:
			case <-ctx.Done():
				return childDirs
			}
		}

		return childDirs
	}

	workerWG.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer workerWG.Done()
			stack := make([]string, 0, 8)
			for {
				var dir string
				var ok bool
				if len(stack) > 0 {
					dir = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				} else {
					select {
					case <-ctx.Done():
						closeDirJobs()
						return
					case dir, ok = <-dirJobs:
						if !ok {
							return
						}
					}
				}

				childDirs := processDir(dir)
				remaining := pendingDirs.Add(int64(len(childDirs)) - 1)
				if remaining == 0 {
					closeDirJobs()
				}

				for _, child := range childDirs {
					select {
					case <-ctx.Done():
						if pendingDirs.Add(-1) == 0 {
							closeDirJobs()
						}
					case dirJobs <- child:
					default:
						stack = append(stack, child)
					}
				}
			}
		}()
	}

	go func() {
		select {
		case dirJobs <- gs.rootPath:
		case <-ctx.Done():
			if pendingDirs.Add(-1) == 0 {
				closeDirJobs()
			}
		}
	}()

	go func() {
		workerWG.Wait()
		close(fileResults)
	}()

	totalFiles := 0
	nextOrder := 0

	for record := range fileResults {
		if ctx.Err() != nil {
			break
		}

		lower := strings.ToLower(record.relPath)

		entry := indexedEntry{
			fullPath:    record.fullPath,
			relPath:     record.relPath,
			lowerPath:   lower,
			size:        record.size,
			modUnixNano: record.modUnixNano,
			mode:        record.mode,
			order:       nextOrder,
		}
		nextOrder++

		gs.appendIndexedEntry(entry, false)

		totalFiles++
		tracker.update(totalFiles)
		if totalFiles >= gs.maxIndexResults {
			cancel()
		}
	}

	workerWG.Wait()
	tracker.flush(totalFiles)

	finished := time.Now()

	gs.indexMu.Lock()
	gs.indexReady = true
	gs.indexBuilding = false
	gs.indexErr = nil
	gs.indexTotalFiles = totalFiles
	gs.pendingBroadcast = 0
	gs.broadcastSnapshotLocked()
	gs.indexMu.Unlock()
	gs.cache.clear()
	gs.incrementIndexGeneration()

	gs.emitProgress(func(p *IndexTelemetry) {
		p.Building = false
		p.Ready = true
		p.Disabled = false
		p.FilesIndexed = totalFiles
		p.CompletedAt = finished
		p.Duration = finished.Sub(start)
		p.UpdatedAt = finished
		p.LastError = ""
	})
	progressDebugf("buildIndex ready total=%d duration=%s", totalFiles, finished.Sub(start))
}

func (gs *GlobalSearcher) makeIndexedResult(entry *indexedEntry, score float64, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments int, hasMatch bool, spans []MatchSpan) GlobalSearchResult {
	fullPath := entry.fullPath
	mode := fs.FileMode(entry.mode)
	fileName := filepath.Base(fullPath)
	dirPath := filepath.Dir(fullPath)

	return GlobalSearchResult{
		FilePath:     fullPath,
		FileName:     fileName,
		DirPath:      dirPath,
		Score:        score,
		PathLength:   pathLength,
		MatchStart:   matchStart,
		MatchEnd:     matchEnd,
		MatchCount:   matchCount,
		WordHits:     wordHits,
		PathSegments: pathSegments,
		InputOrder:   entry.order,
		HasMatch:     hasMatch,
		MatchSpans:   spans,
		FileEntry: fsutil.Entry{
			Name:      fileName,
			IsDir:     false,
			IsSymlink: (mode & os.ModeSymlink) != 0,
			Size:      entry.size,
			Modified:  time.Unix(0, entry.modUnixNano),
			Mode:      mode,
		},
	}
}

func (gs *GlobalSearcher) snapshotEntries(start, end int) []indexedEntry {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	total := len(gs.indexEntries)
	if end <= 0 || end > total {
		end = total
	}
	if start < 0 {
		start = 0
	}
	if start >= end {
		return nil
	}
	chunk := make([]indexedEntry, end-start)
	copy(chunk, gs.indexEntries[start:end])
	return chunk
}

func (gs *GlobalSearcher) appendIndexedEntry(entry indexedEntry, force bool) {
	gs.indexMu.Lock()
	idx := len(gs.indexEntries)
	gs.indexEntries = append(gs.indexEntries, entry)
	if gs.indexRuneBuckets != nil {
		for _, r := range runeKeysForPath(entry.lowerPath) {
			gs.indexRuneBuckets[r] = append(gs.indexRuneBuckets[r], idx)
		}
	}
	gs.pendingBroadcast++
	notify := force || gs.pendingBroadcast >= indexStreamBatchSize
	if notify {
		gs.pendingBroadcast = 0
		gs.broadcastSnapshotLocked()
	}
	gs.indexMu.Unlock()
}

func (gs *GlobalSearcher) indexSnapshot() (ready bool, count int, useIndex bool) {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	return gs.indexReady, len(gs.indexEntries), gs.indexReady && len(gs.indexEntries) > 0
}

func makeSequentialIndexes(total int) []int {
	if total <= 0 {
		return nil
	}
	indexes := make([]int, total)
	for i := 0; i < total; i++ {
		indexes[i] = i
	}
	return indexes
}

func runeKeysForPath(lower string) []rune {
	if lower == "" {
		return nil
	}
	seen := make(map[rune]struct{})
	keys := make([]rune, 0, 8)
	for _, r := range lower {
		if !isRuneIndexable(r) {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		keys = append(keys, r)
	}
	return keys
}

func isRuneIndexable(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	return r >= 'à' && r <= 'ž'
}

func firstRune(s string) rune {
	for _, r := range s {
		if isRuneIndexable(r) {
			return r
		}
	}
	return 0
}

func (gs *GlobalSearcher) emitProgress(mutator func(*IndexTelemetry)) {
	var snapshot IndexTelemetry
	var cb func(IndexTelemetry)

	gs.indexMu.Lock()
	mutator(&gs.progress)
	gs.progress.RootPath = gs.rootPath
	gs.progress.TotalFiles = gs.indexTotalFiles
	gs.progress.MaxIndexResults = gs.maxIndexResults
	gs.progress.UseIndex = true
	if gs.progress.UpdatedAt.IsZero() {
		gs.progress.UpdatedAt = time.Now()
	}
	snapshot = gs.progress
	cb = gs.progressCb
	gs.indexMu.Unlock()

	if progressDebugEnabled {
		progressDebugf(
			"telemetry snapshot build=%v ready=%v disabled=%v files=%d",
			snapshot.Building,
			snapshot.Ready,
			snapshot.Disabled,
			snapshot.FilesIndexed,
		)
	}

	if cb != nil {
		cb(snapshot)
	}
}

func (gs *GlobalSearcher) currentProgress() IndexTelemetry {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	return gs.progress
}
