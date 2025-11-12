package search

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldSkipProtectedEntries(t *testing.T) {
	tmp := t.TempDir()
	searcher := NewGlobalSearcher(tmp, true, nil)

	prev := shouldHideFromListingFn
	shouldHideFromListingFn = func(fullPath, name string) bool {
		return filepath.Base(fullPath) == "Cookies" || name == "Cookies"
	}
	defer func() {
		shouldHideFromListingFn = prev
	}()

	entry := fakeDirEntry{name: "Cookies", dir: true}
	skip, skipDir := searcher.shouldSkip("Cookies", entry, filepath.Join(tmp, "Cookies"), nil)
	if !skip || !skipDir {
		t.Fatalf("expected Cookies to be skipped as a protected entry, got skip=%v skipDir=%v", skip, skipDir)
	}
}

type fakeDirEntry struct {
	name string
	dir  bool
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return f.dir }
func (f fakeDirEntry) Type() fs.FileMode          { return f.info().Mode() }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return f.info(), nil }

func (f fakeDirEntry) info() fs.FileInfo {
	return fakeFileInfo{name: f.name, dir: f.dir}
}

type fakeFileInfo struct {
	name string
	dir  bool
}

func (f fakeFileInfo) Name() string { return f.name }
func (f fakeFileInfo) Size() int64  { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir
	}
	return 0
}
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }
