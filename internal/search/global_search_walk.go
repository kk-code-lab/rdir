package search

import (
	"io/fs"
	"path/filepath"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

// shouldHideFromListingFn mirrors fs.ShouldHideFromListing for test overrides.
var shouldHideFromListingFn = fsutil.ShouldHideFromListing

func (gs *GlobalSearcher) shouldSkip(relPath string, d fs.DirEntry, absPath string, matcher *GitignoreMatcher) (skip bool, skipDir bool) {
	if relPath == "" {
		relPath = "."
	}

	if relPath == "." {
		return false, false
	}

	if d.IsDir() && d.Name() == ".git" {
		return true, true
	}

	if shouldHideFromListingFn(absPath, d.Name()) {
		return true, d.IsDir()
	}

	if gs.hideHidden {
		if fsutil.IsHidden(absPath, d.Name()) {
			return true, d.IsDir()
		}
	}

	var matchPath string
	if relPath == "." {
		matchPath = gs.rootPath
	} else {
		matchPath = filepath.Join(gs.rootPath, relPath)
	}

	if matcher != nil && matcher.MatchWithType(matchPath, d.IsDir()) {
		if d.IsDir() {
			return true, true
		}
		return true, false
	}

	return false, false
}

func joinRelPath(parent, child string) string {
	if parent == "." || parent == "" {
		if child == "" {
			return "."
		}
		return child
	}
	return filepath.Join(parent, child)
}
