package search

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSearchRecursiveAsyncStreamsResults(t *testing.T) {
	root := t.TempDir()
	totalFiles := 200
	for i := 0; i < totalFiles; i++ {
		name := fmt.Sprintf("file-%03d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("data"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	searcher := NewGlobalSearcher(root, false, nil)

	done := make(chan struct{})
	var mu sync.Mutex
	progressSeen := false
	var final []GlobalSearchResult

	searcher.SearchRecursiveAsync("file", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		mu.Lock()
		defer mu.Unlock()
		if inProgress {
			progressSeen = true
		}
		if isDone && !inProgress {
			final = append([]GlobalSearchResult(nil), results...)
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for async search")
	}

	mu.Lock()
	defer mu.Unlock()

	if !progressSeen {
		t.Fatalf("expected to see streaming progress before completion")
	}
	if len(final) != totalFiles {
		t.Fatalf("expected %d results, got %d", totalFiles, len(final))
	}
	if final[0].FileName == "" {
		t.Fatalf("expected filenames in results, got %#v", final[0])
	}
}

func TestSearchRecursiveAsyncReadyIndexSingleCallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)
	searcher.buildIndex(time.Now())

	var mu sync.Mutex
	callbacks := 0
	done := make(chan struct{})

	searcher.SearchRecursiveAsync("main", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		mu.Lock()
		callbacks++
		mu.Unlock()
		if !isDone {
			t.Fatalf("expected only final callback when index ready")
		}
		if inProgress {
			t.Fatalf("expected final callback to report inProgress=false")
		}
		if len(results) == 0 {
			t.Fatalf("expected indexed results")
		}
		select {
		case <-done:
		default:
			close(done)
		}
	})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for indexed async search")
	}

	mu.Lock()
	defer mu.Unlock()
	if callbacks != 1 {
		t.Fatalf("expected exactly one callback, got %d", callbacks)
	}
}
