package state

import (
	"strings"
	"unicode"

	search "github.com/kk-code-lab/rdir/internal/search"
)

func (s *AppState) recomputeFilter() {
	if !s.FilterActive {
		s.FilteredIndices = nil
		s.FilterMatches = nil
		s.invalidateDisplayFilesCache()
		return
	}

	tokens := prepareFilterTokens(s.FilterQuery, s.FilterCaseSensitive)
	if len(tokens) == 0 {
		indices := s.FilteredIndices[:0]
		if cap(indices) < len(s.Files) {
			indices = make([]int, 0, len(s.Files))
		}
		for i := range s.Files {
			indices = append(indices, i)
		}
		s.FilteredIndices = indices
		s.FilterMatches = nil
		s.invalidateDisplayFilesCache()
		return
	}

	if s.filterMatcher == nil {
		s.filterMatcher = search.NewFuzzyMatcher()
	}

	s.ensureLowerNames()

	matches := s.FilterMatches[:0]
	indices := s.FilteredIndices[:0]
	for idx, file := range s.Files {
		lowerName := ""
		if idx < len(s.fileLowerNames) {
			lowerName = s.fileLowerNames[idx]
		}
		score, matched := matchFilterTokens(file.Name, lowerName, tokens, s.FilterCaseSensitive, s.filterMatcher)
		if matched {
			matches = append(matches, FuzzyMatch{FileIndex: idx, Score: score})
			indices = append(indices, idx)
		}
	}

	s.FilterMatches = matches
	s.FilteredIndices = indices
	s.invalidateDisplayFilesCache()
}

func (s *AppState) retainSelectionAfterFilterChange(prevSelectedIndex, prevDisplayIdx int) {
	displayFiles := s.getDisplayFiles()
	if len(displayFiles) == 0 {
		s.SelectedIndex = -1
		return
	}

	if prevSelectedIndex >= 0 {
		if idx := s.displayIndexForFile(prevSelectedIndex); idx >= 0 {
			s.setDisplaySelectedIndex(idx)
			return
		}
	}

	if prevDisplayIdx >= 0 {
		if prevDisplayIdx >= len(displayFiles) {
			prevDisplayIdx = len(displayFiles) - 1
		}
		s.setDisplaySelectedIndex(prevDisplayIdx)
		return
	}

	s.setDisplaySelectedIndex(0)
}

func (s *AppState) displayIndexForFile(fileIdx int) int {
	if fileIdx < 0 || fileIdx >= len(s.Files) {
		return -1
	}

	originalSelected := s.SelectedIndex
	s.SelectedIndex = fileIdx
	displayIdx := s.getDisplaySelectedIndex()
	s.SelectedIndex = originalSelected
	return displayIdx
}

func (s *AppState) clearFilter() {
	s.FilterActive = false
	s.FilterQuery = ""
	s.FilterCaseSensitive = false
	s.FilteredIndices = nil
	s.FilterMatches = nil
	s.ScrollOffset = 0
	if len(s.Files) > 0 {
		s.updateScrollVisibility()
	}
	s.invalidateDisplayFilesCache()
}

func splitFilterTokens(query string) []string {
	var tokens []string
	start := -1
	for idx, r := range query {
		if unicode.IsSpace(r) {
			if start >= 0 {
				tokens = append(tokens, query[start:idx])
				start = -1
			}
			continue
		}
		if start == -1 {
			start = idx
		}
	}
	if start >= 0 {
		tokens = append(tokens, query[start:])
	}
	return tokens
}

func prepareFilterTokens(query string, caseSensitive bool) []filterToken {
	rawTokens := splitFilterTokens(query)
	if len(rawTokens) == 0 {
		return nil
	}

	tokens := make([]filterToken, 0, len(rawTokens))
	for _, token := range rawTokens {
		if token == "" {
			continue
		}
		lower := strings.ToLower(token)
		pattern := token
		if !caseSensitive {
			pattern = lower
		}
		tokens = append(tokens, filterToken{
			raw:     token,
			folded:  lower,
			pattern: pattern,
			runes:   []rune(pattern),
		})
	}

	return tokens
}

func matchFilterTokens(name, lowerName string, tokens []filterToken, caseSensitive bool, matcher *search.FuzzyMatcher) (float64, bool) {
	if len(tokens) == 0 {
		return 0, false
	}

	var target string
	if caseSensitive {
		target = name
	} else {
		if lowerName != "" {
			target = lowerName
		} else {
			target = strings.ToLower(name)
		}
	}

	totalScore := 0.0
	for _, token := range tokens {
		score, matched := matcher.Match(token.pattern, target)
		if !matched {
			return 0, false
		}
		totalScore += score
	}
	return totalScore / float64(len(tokens)), true
}

func countFilterTokens(query string) int {
	return len(splitFilterTokens(query))
}
