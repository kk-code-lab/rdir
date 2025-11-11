package search

import (
	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

type FileEntry = fsutil.Entry

// GlobalSearchResult represents a single result from global search.
type GlobalSearchResult struct {
	FilePath     string
	FileName     string
	DirPath      string
	Score        float64
	PathLength   int
	MatchStart   int
	MatchEnd     int
	MatchCount   int
	WordHits     int
	PathSegments int
	InputOrder   int
	HasMatch     bool
	MatchSpans   []MatchSpan
	FileEntry    FileEntry
}
