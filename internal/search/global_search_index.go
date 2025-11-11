package search

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

type indexedEntry struct {
	fullPathOffset uint32
	fullPathLength uint32
	dirPathLength  uint32
	fileNameOffset uint32
	lowerOffset    uint32
	lowerLength    uint32
	size           int64
	modUnixNano    int64
	mode           uint32
	order          uint32
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
	if matchAll {
		return gs.collectAllIndex()
	}

	collector := newTopCollector(maxDisplayResults)

	for i := range gs.indexEntries {
		entry := &gs.indexEntries[i]
		textLower := gs.indexLowerPath(entry)
		if len(textLower) == 0 {
			continue
		}

		relPath := gs.indexRelativePath(entry)
		score, matched, details := gs.matchTokens(tokens, relPath, caseSensitive, matchAll)
		matchStart := details.Start
		matchEnd := details.End
		targetLen := details.TargetLength
		matchCount := details.MatchCount
		wordHits := details.WordHits
		if !matched {
			continue
		}

		score += computeSegmentBoost(query, relPath, details)

		pathLength := targetLen
		if pathLength == 0 {
			pathLength = utf8.RuneCountInString(relPath)
		}
		pathSegments := countPathSegments(relPath)

		order := int(entry.order)

		if !collector.Needs(score, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments, order, true) {
			continue
		}

		result := gs.makeIndexedResult(entry, score, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments, true, details.Spans)
		collector.Store(result)
	}

	return collector.Results()
}

func (gs *GlobalSearcher) collectAllIndex() []GlobalSearchResult {
	order := gs.indexDisplayOrder
	if len(order) == 0 {
		order = make([]int, len(gs.indexEntries))
		for i := range gs.indexEntries {
			order[i] = i
		}
	}

	limit := len(order)
	if limit > maxDisplayResults {
		limit = maxDisplayResults
	}

	results := make([]GlobalSearchResult, limit)
	for i := 0; i < limit; i++ {
		idx := order[i]
		if idx < 0 || idx >= len(gs.indexEntries) {
			continue
		}
		entry := &gs.indexEntries[idx]
		relPath := gs.indexRelativePath(entry)
		pathLength := utf8.RuneCountInString(relPath)
		segments := countPathSegments(relPath)
		results[i] = gs.makeIndexedResult(entry, 1.0, pathLength, -1, -1, 0, 0, segments, false, nil)
	}

	return results
}

func (gs *GlobalSearcher) maybeUseIndex() bool {
	gs.indexMu.Lock()
	useIndex := gs.useIndex
	ready := gs.indexReady && len(gs.indexEntries) > 0
	shouldStart := useIndex && !ready && !gs.indexBuilding && gs.indexBuildHint
	gs.indexMu.Unlock()

	if shouldStart {
		gs.startIndexBuild(time.Now())
	}

	return useIndex && ready
}

func (gs *GlobalSearcher) startIndexBuild(start time.Time) {
	gs.indexMu.Lock()
	if !gs.useIndex || gs.indexReady || gs.indexBuilding {
		gs.indexMu.Unlock()
		return
	}
	gs.indexBuilding = true
	gs.indexMu.Unlock()

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

func (gs *GlobalSearcher) considerIndexBuildAfterWalk(filesScanned int) {
	if filesScanned < gs.indexThreshold {
		gs.indexMu.Lock()
		gs.useIndex = false
		gs.indexBuildHint = false
		gs.indexMu.Unlock()
		return
	}

	gs.maybeKickoffIndex(filesScanned)
}

func (gs *GlobalSearcher) maybeKickoffIndex(filesScanned int) {
	if filesScanned < gs.indexThreshold {
		return
	}

	gs.indexMu.Lock()
	if !gs.indexBuildHint {
		gs.indexBuildHint = true
	}
	shouldStart := gs.useIndex && !gs.indexReady && !gs.indexBuilding
	gs.indexMu.Unlock()

	if shouldStart {
		gs.startIndexBuild(time.Now())
	}
}

func (gs *GlobalSearcher) buildIndex(start time.Time) {
	entries := make([]indexedEntry, 0, intMin(gs.indexThreshold, gs.maxIndexResults))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := newProgressTracker(start, indexProgressInterval, gs.emitProgress)
	progressDebugf("buildIndex start root=%s threshold=%d", gs.rootPath, gs.indexThreshold)

	workerCount := runtime.NumCPU() - 1
	if workerCount < 2 {
		workerCount = 2
	}
	defaultMaxWorkers := 8
	if runtime.GOOS == "windows" {
		defaultMaxWorkers = 16
	}
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
	pendingDirs.Store(1) // account for root path
	var closeDirJobsOnce sync.Once

	closeDirJobs := func() {
		closeDirJobsOnce.Do(func() {
			progressDebugf("closing dirJobs")
			close(dirJobs)
		})
	}

	processDir := func(dir string) []string {
		progressDebugf("processDir start dir=%s", dir)
		relDir, relErr := filepath.Rel(gs.rootPath, dir)
		if relErr != nil {
			relDir = "."
		}
		if relDir == "" {
			relDir = "."
		}
		dirKey := normalizeDirKey(relDir)
		dirMatcher := gs.ignoreProvider.MatcherFor(dirKey)

		entriesDir, err := os.ReadDir(dir)
		if err != nil {
			progressDebugf("processDir read error dir=%s err=%v", dir, err)
			return nil
		}
		progressDebugf("processDir entries dir=%s count=%d", dir, len(entriesDir))

		childDirs := make([]string, 0, len(entriesDir))

		for _, entry := range entriesDir {
			if ctx.Err() != nil {
				progressDebugf("processDir cancelled dir=%s", dir)
				return childDirs
			}

			fullPath := filepath.Join(dir, entry.Name())
			relPath, relErr := filepath.Rel(gs.rootPath, fullPath)
			if relErr != nil {
				relPath = filepath.Join(relDir, entry.Name())
			}

			if skip, skipDir := gs.shouldSkip(relPath, entry, fullPath, dirMatcher); skip {
				if skipDir {
					progressDebugf("skip dir=%s rel=%s", fullPath, relPath)
					continue
				}
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
				progressDebugf("info error path=%s err=%v", fullPath, infoErr)
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
				if progressDebugVerbose {
					progressDebugf("file queued path=%s", relPath)
				}
			case <-ctx.Done():
				progressDebugf("file enqueue cancelled path=%s", fullPath)
				return childDirs
			}
		}

		progressDebugf("processDir done dir=%s children=%d", dir, len(childDirs))
		return childDirs
	}

	workerWG.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func(id int) {
			defer workerWG.Done()
			progressDebugf("worker %d start", id)
			stack := make([]string, 0, 8)
			for {
				var dir string
				var ok bool
				if len(stack) > 0 {
					dir = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					progressDebugf("worker %d processing inline dir=%s stack=%d", id, dir, len(stack))
				} else {
					select {
					case <-ctx.Done():
						progressDebugf("worker %d exit ctx stack=%d", id, len(stack))
						closeDirJobs()
						return
					case dir, ok = <-dirJobs:
						if !ok {
							progressDebugf("worker %d exit dirJobs closed stack=%d", id, len(stack))
							return
						}
						progressDebugf("worker %d processing dir=%s stack=%d", id, dir, len(stack))
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
						progressDebugf("worker %d enqueue cancelled dir=%s", id, child)
						if pendingDirs.Add(-1) == 0 {
							closeDirJobs()
						}
					case dirJobs <- child:
						progressDebugf("worker %d enqueue child dir=%s", id, child)
					default:
						stack = append(stack, child)
						progressDebugf("worker %d stack child dir=%s stack=%d", id, child, len(stack))
					}
				}
			}
		}(i)
	}

	go func() {
		select {
		case dirJobs <- gs.rootPath:
			progressDebugf("root enqueue %s", gs.rootPath)
		case <-ctx.Done():
			if pendingDirs.Add(-1) == 0 {
				closeDirJobs()
			}
		}
	}()

	go func() {
		workerWG.Wait()
		progressDebugf("workerWG complete; closing fileResults")
		close(fileResults)
	}()

	fullPathArena := make([]byte, 0, 256*1024)
	lowerArena := make([]byte, 0, 256*1024)
	appendString := func(arena *[]byte, value string) (uint32, uint32, bool) {
		length := len(value)
		if length == 0 {
			offset := len(*arena)
			return uint32(offset), 0, true
		}
		if int64(len(*arena)+length) > maxIndexArenaSize {
			return 0, 0, false
		}
		offset := len(*arena)
		*arena = append(*arena, value...)
		return uint32(offset), uint32(length), true
	}

	totalFiles := 0
	var nextOrder uint32
	arenaOverflow := false
	for record := range fileResults {
		if arenaOverflow {
			continue
		}

		fullOffset, fullLength, ok := appendString(&fullPathArena, record.fullPath)
		if !ok {
			arenaOverflow = true
			progressDebugf("full path arena overflow len=%d", len(fullPathArena))
			cancel()
			continue
		}

		dir := filepath.Dir(record.fullPath)
		dirLength := len(dir)

		lower := strings.ToLower(record.relPath)
		lowerOffset, lowerLength, ok := appendString(&lowerArena, lower)
		if !ok {
			arenaOverflow = true
			progressDebugf("lower arena overflow len=%d", len(lowerArena))
			cancel()
			continue
		}

		sepIndex := strings.LastIndexByte(record.fullPath, os.PathSeparator)
		if sepIndex == -1 && os.PathSeparator != '/' {
			sepIndex = strings.LastIndexByte(record.fullPath, '/')
		}
		fileNameOffset := sepIndex + 1
		if fileNameOffset < 0 {
			fileNameOffset = 0
		}
		if fileNameOffset > len(record.fullPath) {
			fileNameOffset = len(record.fullPath)
		}

		entryOrder := nextOrder
		nextOrder++

		entries = append(entries, indexedEntry{
			fullPathOffset: fullOffset,
			fullPathLength: fullLength,
			dirPathLength:  uint32(dirLength),
			fileNameOffset: uint32(fileNameOffset),
			lowerOffset:    lowerOffset,
			lowerLength:    lowerLength,
			size:           record.size,
			modUnixNano:    record.modUnixNano,
			mode:           record.mode,
			order:          entryOrder,
		})

		totalFiles++
		tracker.update(totalFiles)
		if progressDebugVerbose {
			progressDebugf("indexed count=%d path=%s", totalFiles, record.relPath)
		}
		if totalFiles >= gs.maxIndexResults {
			progressDebugf("maxIndexResults reached count=%d", totalFiles)
			cancel()
		}
	}

	progressDebugf("fileResults drained count=%d", totalFiles)
	workerWG.Wait()
	progressDebugf("workerWG.Wait returned after drain")

	tracker.flush(totalFiles)
	progressDebugf("buildIndex drain complete total=%d entries=%d", totalFiles, len(entries))
	progressDebugf("drain complete total=%d", totalFiles)

	sort.Slice(entries, func(i, j int) bool {
		leftLower := arenaString(lowerArena, entries[i].lowerOffset, entries[i].lowerLength)
		rightLower := arenaString(lowerArena, entries[j].lowerOffset, entries[j].lowerLength)
		if leftLower != rightLower {
			return leftLower < rightLower
		}
		leftFull := arenaString(fullPathArena, entries[i].fullPathOffset, entries[i].fullPathLength)
		rightFull := arenaString(fullPathArena, entries[j].fullPathOffset, entries[j].fullPathLength)
		return leftFull < rightFull
	})

	displayOrder := make([]int, len(entries))
	for i := range entries {
		displayOrder[i] = i
	}
	sort.SliceStable(displayOrder, func(i, j int) bool {
		return entries[displayOrder[i]].order < entries[displayOrder[j]].order
	})

	finished := time.Now()

	gs.indexMu.Lock()
	finalCount := totalFiles
	progressDebugf(
		"buildIndex finalize total=%d threshold=%d ready=%v useIndex=%v entries=%d",
		finalCount,
		gs.indexThreshold,
		gs.indexReady,
		gs.useIndex,
		len(gs.indexEntries),
	)
	progressDebugf("buildIndex finalize total=%d threshold=%d", finalCount, gs.indexThreshold)
	lastErrMsg := ""
	if arenaOverflow {
		lastErrMsg = "index string arena exceeded 4GB; using fallback walk search"
	}
	disableIndex := finalCount < gs.indexThreshold || arenaOverflow
	if disableIndex {
		if arenaOverflow {
			gs.indexErr = errors.New(lastErrMsg)
		} else {
			gs.indexErr = nil
		}
		gs.useIndex = false
		gs.indexEntries = nil
		gs.indexPathArena = nil
		gs.indexLowerArena = nil
		gs.indexDisplayOrder = nil
		gs.indexReady = false
		gs.indexBuilding = false
		gs.indexMu.Unlock()

		gs.emitProgress(func(p *IndexTelemetry) {
			p.Building = false
			p.Ready = false
			p.Disabled = true
			p.FilesIndexed = finalCount
			p.CompletedAt = finished
			p.Duration = finished.Sub(start)
			p.UpdatedAt = finished
			p.LastError = lastErrMsg
		})
		progressDebugf("buildIndex disabled total=%d", finalCount)
		return
	}

	gs.indexEntries = entries
	gs.indexPathArena = fullPathArena
	gs.indexLowerArena = lowerArena
	gs.indexDisplayOrder = displayOrder
	gs.indexReady = true
	gs.indexErr = nil
	gs.indexBuilding = false
	progressDebugf(
		"buildIndex commit ready entries=%d useIndex=%v finalCount=%d",
		len(gs.indexEntries),
		gs.useIndex,
		finalCount,
	)
	gs.indexMu.Unlock()

	gs.emitProgress(func(p *IndexTelemetry) {
		p.Building = false
		p.Ready = true
		p.Disabled = false
		p.FilesIndexed = finalCount
		p.CompletedAt = finished
		p.Duration = finished.Sub(start)
		p.UpdatedAt = finished
		p.LastError = ""
	})
	progressDebugf("buildIndex ready total=%d duration=%s", finalCount, finished.Sub(start))
}

func (gs *GlobalSearcher) makeIndexedResult(entry *indexedEntry, score float64, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments int, hasMatch bool, spans []MatchSpan) GlobalSearchResult {
	fullPath := gs.indexFullPath(entry)
	mode := fs.FileMode(entry.mode)

	fileNameStart := int(entry.fileNameOffset)
	if fileNameStart < 0 || fileNameStart > len(fullPath) {
		fileNameStart = len(fullPath)
	}
	fileName := fullPath[fileNameStart:]
	dirLen := int(entry.dirPathLength)
	if dirLen < 0 {
		dirLen = 0
	}
	if dirLen > len(fullPath) {
		dirLen = len(fullPath)
	}
	dirPath := fullPath[:dirLen]

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
		InputOrder:   int(entry.order),
		HasMatch:     hasMatch,
		MatchSpans:   cloneMatchSpans(spans),
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

func (gs *GlobalSearcher) indexRelativePath(entry *indexedEntry) string {
	full := gs.indexFullPath(entry)
	rel, err := filepath.Rel(gs.rootPath, full)
	if err != nil || rel == "" {
		return full
	}
	return rel
}

func arenaString(arena []byte, offset, length uint32) string {
	if length == 0 || len(arena) == 0 {
		return ""
	}
	start := int(offset)
	end := start + int(length)
	if start < 0 || end > len(arena) {
		return ""
	}
	return unsafe.String(&arena[start], int(length))
}

func (gs *GlobalSearcher) indexFullPath(entry *indexedEntry) string {
	return arenaString(gs.indexPathArena, entry.fullPathOffset, entry.fullPathLength)
}

func (gs *GlobalSearcher) indexLowerPath(entry *indexedEntry) string {
	return arenaString(gs.indexLowerArena, entry.lowerOffset, entry.lowerLength)
}

func (gs *GlobalSearcher) emitProgress(mutator func(*IndexTelemetry)) {
	var snapshot IndexTelemetry
	var cb func(IndexTelemetry)

	gs.indexMu.Lock()
	mutator(&gs.progress)
	gs.progress.RootPath = gs.rootPath
	gs.progress.Threshold = gs.indexThreshold
	gs.progress.MaxIndexResults = gs.maxIndexResults
	gs.progress.UseIndex = gs.useIndex
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

func (gs *GlobalSearcher) indexSnapshot() (ready bool, count int, useIndex bool) {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	return gs.indexReady, len(gs.indexEntries), gs.useIndex
}

func (gs *GlobalSearcher) currentProgress() IndexTelemetry {
	gs.indexMu.Lock()
	defer gs.indexMu.Unlock()
	return gs.progress
}
