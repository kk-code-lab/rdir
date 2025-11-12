package search

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

// shouldHideFromListingFn mirrors fs.ShouldHideFromListing for test overrides.
var shouldHideFromListingFn = fsutil.ShouldHideFromListing

func (gs *GlobalSearcher) searchWalk(query string, caseSensitive bool) []GlobalSearchResult {
	tokens, matchAll := prepareQueryTokens(query, caseSensitive)
	hasExplicitQuery := !matchAll

	collector := newTopCollector(maxDisplayResults)
	orderCounter := 0
	filesScanned := 0

	_ = gs.walkFilesBFS(context.Background(), func(path string, relPath string, d fs.DirEntry) error {
		filesScanned++
		gs.maybeKickoffIndex(filesScanned)

		score, matched, details := gs.matchTokens(tokens, relPath, caseSensitive, matchAll)
		if !matched {
			return nil
		}

		score += computeSegmentBoost(query, relPath, details)

		pathLength := details.TargetLength
		if pathLength == 0 {
			pathLength = utf8.RuneCountInString(relPath)
		}
		pathSegments := countPathSegments(relPath)

		order := orderCounter
		orderCounter++

		hasMatch := hasExplicitQuery

		if !collector.Needs(score, pathLength, details.Start, details.End, details.MatchCount, details.WordHits, pathSegments, order, hasMatch) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		collector.Store(makeGlobalSearchResult(path, d, info, score, pathLength, details.Start, details.End, details.MatchCount, details.WordHits, pathSegments, order, hasMatch, details.Spans))

		return nil
	})

	gs.considerIndexBuildAfterWalk(filesScanned)

	return collector.Results()
}

func makeGlobalSearchResult(path string, d fs.DirEntry, info fs.FileInfo, score float64, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments, order int, hasMatch bool, spans []MatchSpan) GlobalSearchResult {
	return GlobalSearchResult{
		FilePath:     path,
		FileName:     d.Name(),
		DirPath:      filepath.Dir(path),
		Score:        score,
		PathLength:   pathLength,
		MatchStart:   matchStart,
		MatchEnd:     matchEnd,
		MatchCount:   matchCount,
		WordHits:     wordHits,
		PathSegments: pathSegments,
		InputOrder:   order,
		HasMatch:     hasMatch,
		MatchSpans:   cloneMatchSpans(spans),
		FileEntry: fsutil.Entry{
			Name:      d.Name(),
			IsDir:     d.IsDir(),
			IsSymlink: (info.Mode() & os.ModeSymlink) != 0,
			Size:      info.Size(),
			Modified:  info.ModTime(),
			Mode:      info.Mode(),
		},
	}
}

func cloneMatchSpans(spans []MatchSpan) []MatchSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]MatchSpan, len(spans))
	copy(out, spans)
	return out
}

func (gs *GlobalSearcher) walkFilesBFS(ctx context.Context, handle func(fullPath, relPath string, d fs.DirEntry) error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	type dirNode struct {
		absPath string
		relPath string
		matcher *GitignoreMatcher
	}

	rootMatcher := gs.ignoreProvider.MatcherFor(normalizeDirKey("."))
	queue := []dirNode{{
		absPath: gs.rootPath,
		relPath: ".",
		matcher: rootMatcher,
	}}

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}

		node := queue[0]
		queue = queue[1:]

		entries, err := os.ReadDir(node.absPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}

			rel := joinRelPath(node.relPath, entry.Name())
			fullPath := filepath.Join(node.absPath, entry.Name())

			if skip, _ := gs.shouldSkip(rel, entry, fullPath, node.matcher); skip {
				continue
			}

			if entry.IsDir() {
				dirKey := normalizeDirKey(rel)
				dirMatcher := gs.ignoreProvider.MatcherFor(dirKey)
				queue = append(queue, dirNode{
					absPath: fullPath,
					relPath: rel,
					matcher: dirMatcher,
				})
				continue
			}

			if err := handle(fullPath, rel, entry); err != nil {
				return err
			}
		}
	}

	return nil
}

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
