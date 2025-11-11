package search

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSearchRecursiveAsyncFindsMatches(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(filePath, []byte("package main"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	searcher := NewGlobalSearcher(root, false, nil)
	tokens, matchAll := prepareQueryTokens("go", false)

	visitCount := 0
	matchFromWalk := 0
	_ = searcher.walkFilesBFS(context.Background(), func(path string, relPath string, d fs.DirEntry) error {
		visitCount++
		_, matched, _ := searcher.matchTokens(tokens, relPath, false, matchAll)
		if matched {
			matchFromWalk++
		}
		return nil
	})
	if visitCount == 0 {
		t.Fatalf("expected walk to visit files")
	}
	if matchFromWalk == 0 {
		t.Fatalf("expected walk to find matches")
	}

	if _, matched, _ := searcher.matchTokens(tokens, "main.go", false, matchAll); !matched {
		t.Fatalf("expected match for main.go")
	}

	ctx, cancel := context.WithCancel(context.Background())
	token := searcher.setCancel(cancel)
	var directResults []GlobalSearchResult
	directDone := make(chan struct{})
	go searcher.runWalkerAsync(ctx, cancel, token, "go", false, tokens, matchAll, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if !isDone {
			return
		}
		directResults = append([]GlobalSearchResult(nil), results...)
		select {
		case <-directDone:
		default:
			close(directDone)
		}
	})

	select {
	case <-directDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for direct walker results")
	}

	if len(directResults) == 0 {
		t.Fatalf("direct walker produced no results")
	}

	var mu sync.Mutex
	var finalResults []GlobalSearchResult
	done := make(chan struct{})

	searcher.SearchRecursiveAsync("go", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
		if !isDone {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		finalResults = append([]GlobalSearchResult(nil), results...)
		select {
		case <-done:
		default:
			close(done)
		}
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for async search")
	}

	if len(finalResults) == 0 {
		t.Fatalf("expected async results, got none")
	}

	if finalResults[0].FilePath != filePath {
		t.Fatalf("unexpected result: %+v", finalResults[0])
	}
}
