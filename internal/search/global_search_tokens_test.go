package search

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestGlobalSearchTokensRequireAll(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "alpha beta.txt"))
	writeTestFile(t, filepath.Join(root, "beta alpha.txt"))
	writeTestFile(t, filepath.Join(root, "alpha.txt"))
	writeTestFile(t, filepath.Join(root, "beta.txt"))

	searcher := NewGlobalSearcher(root, false, nil)

	results := searcher.SearchRecursive("alpha beta", false)
	got := collectResultFiles(results)
	want := []string{"alpha beta.txt", "beta alpha.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected only files containing both tokens\nwant: %v\ngot:  %v", want, got)
	}

	// Trailing space should behave like single-token search
	resultsTrailing := searcher.SearchRecursive("alpha ", false)
	gotTrailing := collectResultFiles(resultsTrailing)
	resultsSingle := searcher.SearchRecursive("alpha", false)
	gotSingle := collectResultFiles(resultsSingle)
	if !reflect.DeepEqual(gotTrailing, gotSingle) {
		t.Fatalf("alpha<space> results should match alpha\nwith space: %v\nwithout:    %v", gotTrailing, gotSingle)
	}

	// Whitespace-only query should match everything (no filtering)
	resultsWhitespace := searcher.SearchRecursive("   ", false)
	gotWhitespace := collectResultFiles(resultsWhitespace)
	allFiles := []string{"alpha beta.txt", "beta alpha.txt", "alpha.txt", "beta.txt"}
	if len(gotWhitespace) < len(allFiles) {
		t.Fatalf("whitespace-only query should return all files, got %v", gotWhitespace)
	}
	for _, name := range allFiles {
		if !contains(gotWhitespace, name) {
			t.Fatalf("whitespace query missing %s in %v", name, gotWhitespace)
		}
	}
}

func TestGlobalSearchTokensApplyToIndexResults(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "alpha beta.txt"))
	writeTestFile(t, filepath.Join(root, "beta alpha.txt"))
	writeTestFile(t, filepath.Join(root, "alpha.txt"))

	searcher := NewGlobalSearcher(root, false, nil)
	searcher.buildIndex(time.Now())

	results := searcher.SearchRecursive("alpha beta", false)
	got := collectResultFiles(results)
	want := []string{"alpha beta.txt", "beta alpha.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("index search expected files with both tokens\nwant: %v\ngot:  %v", want, got)
	}
}

func collectResultFiles(results []GlobalSearchResult) []string {
	var names []string
	seen := map[string]bool{}
	for _, res := range results {
		if res.FileEntry.IsDir {
			continue
		}
		name := filepath.Base(res.FilePath)
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	// Keep deterministic order for comparisons
	sort.Strings(names)
	return names
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
