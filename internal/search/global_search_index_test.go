package search

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGlobalSearcherBuildsIndex(t *testing.T) {
	root := t.TempDir()
	files := []string{"alpha.txt", "beta.txt", "gamma.txt"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	searcher := NewGlobalSearcher(root, false, nil)
	searcher.buildIndex(time.Now())
	results := searcher.SearchRecursive("a", false)
	if len(results) == 0 {
		t.Fatalf("expected results from indexed search")
	}

	ready, count, useIndex := searcher.indexSnapshot()
	if !ready || !useIndex {
		t.Fatalf("expected index ready after initial search, got ready=%v use=%v", ready, useIndex)
	}
	if count != len(files) {
		t.Fatalf("expected %d entries, got %d", len(files), count)
	}
}

func TestGlobalSearcherAsyncUsesIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gamma.txt"), []byte("g"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)
	searcher.buildIndex(time.Now())

	doneCh := make(chan struct{})
	searcher.SearchRecursiveAsync("gamma", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if !isDone || inProgress {
			return
		}
		if len(results) == 0 {
			t.Fatalf("expected indexed results")
		}
		select {
		case <-doneCh:
		default:
			close(doneCh)
		}
	})

	select {
	case <-doneCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected async search to finish promptly once index ready")
	}
}

func TestGlobalSearcherBuildIndexReportsProgress(t *testing.T) {
	root := t.TempDir()
	const fileCount = 50
	for i := 0; i < fileCount; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%d.txt", i)), []byte("x"), 0o644); err != nil {
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

func TestProgressTrackerEmitsBatchSnapshots(t *testing.T) {
	interval := 100 * time.Millisecond
	current := time.Now()
	emitted := make([]IndexTelemetry, 0, 4)
	tracker := newProgressTracker(current, interval, func(mutator func(*IndexTelemetry)) {
		var tel IndexTelemetry
		mutator(&tel)
		emitted = append(emitted, tel)
	}).withClock(func() time.Time { return current })

	advance := func(d time.Duration) {
		current = current.Add(d)
	}

	advance(interval)
	tracker.update(1)

	advance(interval / 2)
	tracker.update(2)

	advance(interval)
	tracker.update(3)

	tracker.flush(5)

	if len(emitted) != 4 {
		t.Fatalf("expected 4 emissions, got %d", len(emitted))
	}
}

func TestGlobalSearcherBuildIndexEmitsReadySnapshot(t *testing.T) {
	root := t.TempDir()
	const fileCount = 5
	for i := 0; i < fileCount; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%d.txt", i)), []byte("x"), 0o644); err != nil {
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
	searcher.maxIndexResults = fileCount

	searcher.buildIndex(time.Now())

	mu.Lock()
	defer mu.Unlock()

	if len(snapshots) == 0 {
		t.Fatalf("expected at least one progress snapshot")
	}

	final := snapshots[len(snapshots)-1]
	if !final.Ready || final.Building {
		t.Fatalf("expected final snapshot ready=true, building=false; got %#v", final)
	}
	if final.FilesIndexed != fileCount {
		t.Fatalf("expected final snapshot to report %d files, got %d", fileCount, final.FilesIndexed)
	}
}

func containsName(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func TestGlobalSearcherPrefixIndexNarrowsCandidates(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"alpha.txt",
		"alpine.log",
		"beta.txt",
		"gamma.txt",
	}
	for _, name := range files {
		full := filepath.Join(root, name)
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	searcher := NewGlobalSearcher(root, false, nil)
	searcher.buildIndex(time.Now())

	results := searcher.searchIndex("alp", false)
	if len(results) != 2 {
		t.Fatalf("expected prefix search to hit 2 files, got %d", len(results))
	}
	names := []string{results[0].FileName, results[1].FileName}
	if !containsName(names, "alpha.txt") || !containsName(names, "alpine.log") {
		t.Fatalf("expected alpha/alpine in results, got %#v", names)
	}
}

func TestIndexCandidatesRequireAllTokenRunes(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"foo.txt",
		"bar.txt",
		"foo-bar.txt",
	}
	for _, name := range files {
		full := filepath.Join(root, name)
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	searcher := NewGlobalSearcher(root, false, nil)
	searcher.buildIndex(time.Now())
	entries := searcher.snapshotEntries(0, -1)

	tokens, _ := prepareQueryTokens("foo bar", false)
	searcher.orderTokens(tokens)

	candidates := searcher.indexCandidates(tokens, entries)

	got := make([]string, 0, len(candidates))
	for _, idx := range candidates {
		if idx >= 0 && idx < len(entries) {
			got = append(got, entries[idx].relPath)
		}
	}

	if len(got) != 1 || got[0] != "foo-bar.txt" {
		t.Fatalf("expected foo-bar only, got %#v", got)
	}
}
