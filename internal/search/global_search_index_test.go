package search

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGlobalSearcherBuildsIndexWhenThresholdMet(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "10")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)
	results := searcher.SearchRecursive("a", false)
	waitForIndexReady(t, searcher)

	ready, count, useIndex := searcher.indexSnapshot()
	if !ready {
		t.Fatalf("expected index to be ready, got false")
	}
	if count == 0 {
		t.Fatalf("expected index to have entries")
	}
	if !useIndex {
		t.Fatalf("expected searcher to keep using index")
	}
	if len(results) == 0 {
		t.Fatalf("expected results, got none")
	}
}

func TestGlobalSearcherDisablesIndexBelowThreshold(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "5") // require more files than we create
	t.Setenv(envMaxIndexResults, "10")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)
	_ = searcher.SearchRecursive("o", false)

	waitForIndexDisabled(t, searcher)

	ready, _, useIndex := searcher.indexSnapshot()
	if useIndex {
		t.Fatalf("expected index to be disabled for small directories")
	}
	if ready {
		t.Fatalf("expected index to remain unready")
	}
}

func TestGlobalSearcherAsyncUsesIndex(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "10")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gamma.txt"), []byte("g"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)

	// warm up index
	_ = searcher.SearchRecursive("g", false)
	waitForIndexReady(t, searcher)

	doneCh := make(chan struct{})
	searcher.SearchRecursiveAsync("g", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if isDone {
			if inProgress {
				t.Fatalf("expected inProgress=false for final callback when using index")
			}
			if len(results) == 0 {
				t.Fatalf("expected results from index search")
			}
			select {
			case <-doneCh:
			default:
				close(doneCh)
			}
		}
	})

	select {
	case <-doneCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected final callback to complete promptly when using index")
	}
}

func TestGlobalSearcherAsyncIndexCancellationSkipsCallback(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "1000000")

	root := t.TempDir()
	stubPath := filepath.Join(root, "stub.txt")
	if err := os.WriteFile(stubPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write stub file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)

	// Warm up index to trigger index building
	_ = searcher.SearchRecursive("stub", false)
	waitForIndexReady(t, searcher)

	const entryCount = 400000
	const relLower = "stub-file-name-that-is-deliberately-very-long-to-slow-down-matching-stub-file-name-that-is-deliberately-very-long-to-slow-down-matching.txt"
	entries := make([]indexedEntry, entryCount)
	stubResult := GlobalSearchResult{
		FilePath:  stubPath,
		FileName:  "stub.txt",
		DirPath:   root,
		Score:     0,
		FileEntry: FileEntry{Name: "stub.txt"},
	}

	lowerText := relLower
	fullPathText := stubPath
	fullPathArena := []byte(fullPathText)
	dirText := filepath.Dir(fullPathText)
	lowerArena := []byte(lowerText)
	fileNameOffset := strings.LastIndexByte(fullPathText, os.PathSeparator) + 1
	if fileNameOffset < 0 {
		fileNameOffset = 0
	}

	for i := range entries {
		entries[i] = indexedEntry{
			fullPathOffset: 0,
			fullPathLength: uint32(len(fullPathText)),
			dirPathLength:  uint32(len(dirText)),
			fileNameOffset: uint32(fileNameOffset),
			lowerOffset:    0,
			lowerLength:    uint32(len(lowerText)),
			size:           0,
			modUnixNano:    stubResult.FileEntry.Modified.UnixNano(),
			mode:           uint32(stubResult.FileEntry.Mode),
		}
	}

	searcher.indexMu.Lock()
	searcher.indexEntries = entries
	searcher.indexPathArena = fullPathArena
	searcher.indexLowerArena = lowerArena
	searcher.indexReady = true
	searcher.indexMu.Unlock()

	callbackTriggered := make(chan struct{}, 1)

	searcher.SearchRecursiveAsync("stub", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if !isDone {
			return
		}
		select {
		case callbackTriggered <- struct{}{}:
		default:
		}
	})

	time.Sleep(5 * time.Millisecond)
	searcher.cancelOngoingSearch()

	select {
	case <-callbackTriggered:
		t.Fatalf("expected no final callback after cancellation")
	case <-time.After(150 * time.Millisecond):
		// success: no callback observed
	}
}

func TestGlobalSearcherBuildIndexReportsProgress(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "1000")

	root := t.TempDir()
	const fileCount = 200
	for i := 0; i < fileCount; i++ {
		name := fmt.Sprintf("file-%d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
	}

	var (
		mu        sync.Mutex
		maxSeen   int
		callCount int
	)

	progressCb := func(tel IndexTelemetry) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		if tel.FilesIndexed > maxSeen {
			maxSeen = tel.FilesIndexed
		}
	}

	searcher := NewGlobalSearcher(root, false, progressCb)
	searcher.indexThreshold = 1
	searcher.maxIndexResults = fileCount

	searcher.buildIndex(time.Now())

	mu.Lock()
	defer mu.Unlock()
	if callCount == 0 {
		t.Fatalf("expected progress callback to run at least once")
	}
	if maxSeen < fileCount {
		t.Fatalf("expected progress callback to report at least %d indexed files, got %d", fileCount, maxSeen)
	}
}

func TestGlobalSearchIndexMatchesWalkOrdering(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "1000")

	root := t.TempDir()
	files := []string{
		"src/main.go",
		"src/server.go",
		"src/internal/app.go",
		"scripts/deploy.sh",
		"README.md",
		"docs/guide.md",
		"pkg/api/handler_test.go",
	}
	for _, name := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	searcher := NewGlobalSearcher(root, false, nil)

	// Warm cache and ensure index ready.
	_ = searcher.SearchRecursive("src", false)
	waitForIndexReady(t, searcher)

	walk := searcher.searchWalk("src", false)
	index := searcher.searchIndex("src", false)

	if len(walk) != len(index) {
		t.Fatalf("expected walk and index results to have equal length, walk=%d index=%d", len(walk), len(index))
	}

	substringIdx := func(text string) int {
		return strings.Index(strings.ToLower(text), "src")
	}

	for i := range walk {
		if walk[i].FilePath != index[i].FilePath {
			t.Fatalf("result %d mismatch: walk=%s index=%s", i, walk[i].FilePath, index[i].FilePath)
		}
		if math.Abs(walk[i].Score-index[i].Score) > 1e-9 {
			t.Fatalf("score mismatch for %s: walk=%.6f index=%.6f", walk[i].FilePath, walk[i].Score, index[i].Score)
		}
		// ensure substring bonus applied equally
		if substringIdx(walk[i].FilePath) != -1 {
			if walk[i].Score < 5.0 {
				t.Fatalf("expected substring bonus for %s, got %.6f", walk[i].FilePath, walk[i].Score)
			}
		}
	}
}

func TestGlobalSearchIndexPrefersCompactMatch(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "1000000")

	root := t.TempDir()

	fillers := []string{
		"sandbox/localmirror/cortexbot/arch/em/EFM32_Gxxx_DK/bspdoc/html/dmd/_ssd2119_8h_source.html",
		"sandbox/localmirror/cortexbot/arch/em/EFM32_Gxxx_DK/bspdoc/html/dmd/_ssd2119_registers_8h_source.html",
		"sandbox/localmirror/cortexbot/arch/em/EFM32_Gxxx_DK/bspdoc/html/dmd/_ssd2119_spi_8c_source.html",
	}

	for i := 0; i < 200; i++ {
		name := fillers[i%len(fillers)]
		full := filepath.Join(root, fmt.Sprintf("%s_%03d", name, i))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir filler: %v", err)
		}
		if err := os.WriteFile(full, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write filler: %v", err)
		}
	}

	dhtPath := filepath.Join(root, "drivers/sensors/dht11/dht11_driver.c")
	if err := os.MkdirAll(filepath.Dir(dhtPath), 0o755); err != nil {
		t.Fatalf("mkdir dht: %v", err)
	}
	if err := os.WriteFile(dhtPath, []byte("sensor"), 0o644); err != nil {
		t.Fatalf("write dht: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)

	_ = searcher.SearchRecursive("dht11", false)
	waitForIndexReady(t, searcher)

	results := searcher.searchIndex("dht11", false)
	if len(results) == 0 {
		t.Fatalf("expected results for dht11 query")
	}
	if !strings.Contains(results[0].FilePath, "dht11") {
		t.Fatalf("expected best match to contain dht11, got %s", results[0].FilePath)
	}
}

func TestProgressTrackerMonotonicEmits(t *testing.T) {
	start := time.Unix(0, 0)
	interval := 5 * time.Millisecond
	var current = start
	emitted := make([]IndexTelemetry, 0, 4)
	tracker := newProgressTracker(start, interval, func(mut func(*IndexTelemetry)) {
		tel := IndexTelemetry{}
		mut(&tel)
		emitted = append(emitted, tel)
	}).withClock(func() time.Time { return current })

	advance := func(d time.Duration) {
		current = current.Add(d)
	}

	advance(interval)
	tracker.update(1)

	advance(interval / 2)
	tracker.update(2) // should not emit yet (interval not reached)

	advance(interval)
	tracker.update(3) // emits (time threshold met)

	tracker.flush(5) // final count

	if got := len(emitted); got != 4 {
		t.Fatalf("expected 4 emissions, got %d", got)
	}
	counts := []int{1, 2, 3, 5}
	for i, tel := range emitted {
		if tel.FilesIndexed != counts[i] {
			t.Fatalf("snapshot %d count = %d, want %d", i, tel.FilesIndexed, counts[i])
		}
		if !tel.Building && i < len(emitted)-1 {
			t.Fatalf("snapshot %d should still be building", i)
		}
	}

	final := emitted[len(emitted)-1]
	if !final.Building {
		t.Fatalf("final flush should be building (ready handled later), got building=%v", final.Building)
	}
}

func TestGlobalSearcherBuildIndexEmitsReadySnapshot(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "1000")

	root := t.TempDir()
	const fileCount = 5
	for i := 0; i < fileCount; i++ {
		name := fmt.Sprintf("file-%d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
	}

	var (
		mu        sync.Mutex
		snapshots []IndexTelemetry
	)

	searcher := NewGlobalSearcher(root, false, func(tel IndexTelemetry) {
		mu.Lock()
		snapshots = append(snapshots, tel)
		mu.Unlock()
	})
	searcher.indexThreshold = 1
	searcher.maxIndexResults = fileCount

	searcher.buildIndex(time.Now())

	mu.Lock()
	defer mu.Unlock()

	if len(snapshots) == 0 {
		t.Fatalf("expected at least one progress snapshot")
	}

	final := snapshots[len(snapshots)-1]
	if !final.Ready {
		t.Fatalf("expected final snapshot ready=true, got %#v", final)
	}
	if final.Building {
		t.Fatalf("expected final snapshot building=false, got %#v", final)
	}
	if final.FilesIndexed != fileCount {
		t.Fatalf("expected final snapshot to report %d files, got %d", fileCount, final.FilesIndexed)
	}

	buildingSeen := false
	for _, snap := range snapshots {
		if snap.Building {
			buildingSeen = true
			break
		}
	}
	if !buildingSeen {
		t.Fatalf("expected to observe at least one in-progress snapshot, snapshots=%#v", snapshots)
	}
}
func waitForIndexReady(t *testing.T, searcher *GlobalSearcher) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		ready, count, useIndex := searcher.indexSnapshot()
		if ready && useIndex && count > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("index not ready: ready=%v useIndex=%v count=%d", ready, useIndex, count)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForIndexDisabled(t *testing.T, searcher *GlobalSearcher) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		ready, _, useIndex := searcher.indexSnapshot()
		if !useIndex {
			// Once disabled, we expect ready=false
			if ready {
				t.Fatalf("expected ready=false when index disabled")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("index not disabled in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
