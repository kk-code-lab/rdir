package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalSearcherHonorsIgnoreRulesWalk(t *testing.T) {
	t.Setenv(envDisableIndex, "1")

	root, wantPresent, wantAbsent := createGlobalSearchIgnoreFixture(t)

	searcher := NewGlobalSearcher(root, false, nil)
	results := searcher.SearchRecursive("", false)

	paths := collectRelativePathSet(root, results)

	for _, expected := range wantPresent {
		if !paths.contains(expected) {
			t.Fatalf("expected %q to be present in walk results; got %#v", expected, paths.items())
		}
	}

	for _, unexpected := range wantAbsent {
		if paths.contains(unexpected) {
			t.Fatalf("did not expect %q in walk results; got %#v", unexpected, paths.items())
		}
	}
}

func TestGlobalSearcherHonorsIgnoreRulesIndex(t *testing.T) {
	t.Setenv(envDisableIndex, "")
	t.Setenv(envIndexThreshold, "1")
	t.Setenv(envMaxIndexResults, "100")

	root, wantPresent, wantAbsent := createGlobalSearchIgnoreFixture(t)

	searcher := NewGlobalSearcher(root, false, nil)

	// Kick off index build
	_ = searcher.SearchRecursive("init", false)
	waitForIndexReady(t, searcher)

	results := searcher.SearchRecursive("", false)
	paths := collectRelativePathSet(root, results)

	for _, expected := range wantPresent {
		if !paths.contains(expected) {
			t.Fatalf("expected %q to be present in indexed results; got %#v", expected, paths.items())
		}
	}

	for _, unexpected := range wantAbsent {
		if paths.contains(unexpected) {
			t.Fatalf("did not expect %q in indexed results; got %#v", unexpected, paths.items())
		}
	}
}

func createGlobalSearchIgnoreFixture(t *testing.T) (root string, include []string, exclude []string) {
	t.Helper()

	root = t.TempDir()

	writeFile := func(relPath, data string) {
		t.Helper()
		full := filepath.Join(root, relPath)
		dir := filepath.Dir(full)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	writeFile(".gitignore", "ignored_git.txt\nignored_dir/\nignored_folder/\n")
	writeFile(".ignore", "ignored_ignore.txt\n")
	writeFile(".rdirignore", "ignored_rdir.txt\n!ignored_git.txt\n!ignored_dir/\n")
	writeFile("keep_root.txt", "root")
	writeFile("ignored_git.txt", "ignored")
	writeFile("ignored_ignore.txt", "ignored")
	writeFile("ignored_rdir.txt", "ignored")
	writeFile("excluded_info.txt", "ignored")

	writeFile("ignored_dir/safe.txt", "safe")
	writeFile("ignored_folder/skip.txt", "skip")

	if err := os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755); err != nil {
		t.Fatalf("mkdir git info: %v", err)
	}
	writeFile(filepath.Join(".git", "info", "exclude"), "excluded_info.txt\n")

	writeFile(filepath.Join("nested", ".gitignore"), "*.log\n")
	writeFile(filepath.Join("nested", ".ignore"), "*.tmp\n")
	writeFile(filepath.Join("nested", "keep.md"), "keep")
	writeFile(filepath.Join("nested", "ignoreme.log"), "ignore")
	writeFile(filepath.Join("nested", "temp.tmp"), "ignore")
	writeFile(filepath.Join("nested", "sub", "keep.txt"), "keep")
	writeFile(filepath.Join("nested", "sub", "child.tmp"), "ignore")

	include = []string{
		"keep_root.txt",
		"ignored_git.txt",
		"ignored_dir/safe.txt",
		"nested/keep.md",
		"nested/sub/keep.txt",
	}

	exclude = []string{
		"ignored_ignore.txt",
		"ignored_rdir.txt",
		"excluded_info.txt",
		"ignored_folder/skip.txt",
		"nested/ignoreme.log",
		"nested/temp.tmp",
		"nested/sub/child.tmp",
	}

	return root, include, exclude
}

type pathSet map[string]struct{}

func collectRelativePathSet(root string, results []GlobalSearchResult) pathSet {
	set := make(pathSet, len(results))
	for _, res := range results {
		rel, err := filepath.Rel(root, res.FilePath)
		if err != nil {
			rel = res.FileName
		}
		rel = filepath.ToSlash(rel)
		set[rel] = struct{}{}
	}
	return set
}

func (ps pathSet) contains(path string) bool {
	path = filepath.ToSlash(path)
	_, ok := ps[path]
	return ok
}

func (ps pathSet) items() []string {
	paths := make([]string, 0, len(ps))
	for p := range ps {
		paths = append(paths, p)
	}
	return paths
}
