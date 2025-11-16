package search

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type queryToken struct {
	raw     string
	folded  string
	pattern string
	runes   []rune
}

func (gs *GlobalSearcher) matchTokens(tokens []queryToken, relPath string, caseSensitive bool, matchAll bool) (float64, bool, MatchDetails) {
	if matchAll {
		return 1.0, true, MatchDetails{
			Start:        -1,
			End:          -1,
			TargetLength: utf8.RuneCountInString(relPath),
		}
	}

	if len(tokens) == 0 {
		return 0, false, MatchDetails{}
	}

	fold := !caseSensitive
	pathRunes, pathBuf := acquireRunes(relPath, fold)
	defer releaseRunes(pathBuf)

	pathScore, pathDetails, ok := gs.aggregateTokenMatches(tokens, relPath, pathRunes)
	if !ok {
		return 0, false, MatchDetails{}
	}

	bestScore := pathScore
	bestDetails := pathDetails

	filename := filepath.Base(relPath)
	if filename == "" {
		filename = relPath
	}
	if filename != "" {
		fileRunes, fileBuf := acquireRunes(filename, fold)
		fileOffset := len(pathRunes) - len(fileRunes)
		if fileOffset < 0 {
			fileOffset = 0
		}
		fileScore, fileDetails, fileOK := gs.aggregateTokenMatches(tokens, filename, fileRunes)
		releaseRunes(fileBuf)

		if fileOK && fileScore > bestScore {
			bestScore = fileScore
			if fileDetails.Start >= 0 {
				fileDetails.Start += fileOffset
			}
			if fileDetails.End >= 0 {
				fileDetails.End += fileOffset
			}
			for i := range fileDetails.Spans {
				fileDetails.Spans[i].Start += fileOffset
				fileDetails.Spans[i].End += fileOffset
			}
			bestDetails = fileDetails
		}
	}

	if bestDetails.TargetLength == 0 {
		bestDetails.TargetLength = len(pathRunes)
	}

	return bestScore / float64(len(tokens)), true, bestDetails
}

func prepareQueryTokens(query string, caseSensitive bool) ([]queryToken, bool) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, true
	}
	rawTokens := splitQueryTokens(trimmed)
	if len(rawTokens) == 0 {
		return nil, true
	}

	fold := !caseSensitive
	tokens := make([]queryToken, 0, len(rawTokens))
	for _, t := range rawTokens {
		if t == "" {
			continue
		}
		folded := strings.ToLower(t)
		pattern := t
		if fold {
			pattern = folded
		}
		tokens = append(tokens, queryToken{
			raw:     t,
			folded:  folded,
			pattern: pattern,
			runes:   []rune(pattern),
		})
	}

	if len(tokens) == 0 {
		return nil, true
	}

	sort.SliceStable(tokens, func(i, j int) bool {
		return len(tokens[i].runes) > len(tokens[j].runes)
	})

	return tokens, false
}

// orderTokens reorders tokens to run the most selective ones first.
// With an index in place we approximate selectivity using rune bucket sizes,
// falling back to length-based ordering when index stats are unavailable.
func (gs *GlobalSearcher) orderTokens(tokens []queryToken) {
	if len(tokens) < 2 {
		return
	}

	// Collect the unique indexable runes present in all tokens so we can
	// snapshot only the bucket sizes we need while holding the mutex.
	runeSet := make(map[rune]struct{})
	for _, token := range tokens {
		source := token.folded
		if source == "" {
			source = token.pattern
		}
		for _, r := range source {
			if !isRuneIndexable(r) {
				continue
			}
			runeSet[r] = struct{}{}
		}
	}

	if len(runeSet) == 0 {
		// Nothing indexable to rank by; keep existing order.
		return
	}

	bucketSizes := make(map[rune]int, len(runeSet))
	totalEntries := 0

	gs.indexMu.Lock()
	totalEntries = len(gs.indexEntries)
	if gs.indexRuneBuckets != nil {
		for r := range runeSet {
			if bucket, ok := gs.indexRuneBuckets[r]; ok {
				bucketSizes[r] = len(bucket)
			}
		}
	}
	gs.indexMu.Unlock()

	if totalEntries == 0 || len(bucketSizes) == 0 {
		// No index data yet; keep length ordering to avoid churn.
		return
	}

	type tokenOrderScore struct {
		selectivity float64
		bestRatio   float64
		length      int
	}

	totalEntriesFloat := float64(totalEntries)
	scores := make([]tokenOrderScore, len(tokens))
	for i, token := range tokens {
		length := len(token.runes)
		best := totalEntries
		second := totalEntries
		seen := make(map[rune]struct{})
		source := token.folded
		if source == "" {
			source = token.pattern
		}
		for _, r := range source {
			if !isRuneIndexable(r) {
				continue
			}
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			if size, ok := bucketSizes[r]; ok {
				if size < best {
					second = best
					best = size
				} else if size < second {
					second = size
				}
			} else if best > 0 {
				second = best
				best = 0
			}
		}
		bestRatio := float64(best) / totalEntriesFloat
		selectivity := bestRatio
		if len(seen) >= 2 {
			pairRatio := (float64(best) * float64(second)) / (totalEntriesFloat * totalEntriesFloat)
			if pairRatio < selectivity {
				selectivity = pairRatio
			}
		}
		scores[i] = tokenOrderScore{
			selectivity: selectivity,
			bestRatio:   bestRatio,
			length:      length,
		}
	}

	sort.SliceStable(tokens, func(i, j int) bool {
		if scores[i].selectivity != scores[j].selectivity {
			return scores[i].selectivity < scores[j].selectivity
		}
		if scores[i].bestRatio != scores[j].bestRatio {
			return scores[i].bestRatio < scores[j].bestRatio
		}
		if scores[i].length != scores[j].length {
			return scores[i].length > scores[j].length
		}
		return tokens[i].raw < tokens[j].raw
	})
}

func splitQueryTokens(query string) []string {
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

func (gs *GlobalSearcher) aggregateTokenMatches(tokens []queryToken, text string, textRunes []rune) (float64, MatchDetails, bool) {
	totalScore := 0.0
	agg := MatchDetails{
		Start:        math.MaxInt32,
		End:          -1,
		TargetLength: len(textRunes),
	}

	for _, token := range tokens {
		score, matched, details := gs.matcher.MatchDetailedFromRunes(token.pattern, token.runes, text, textRunes)
		if !matched {
			return 0, MatchDetails{}, false
		}
		totalScore += score

		if details.MatchCount > 0 {
			agg.MatchCount += details.MatchCount
		}
		if details.WordHits > 0 {
			agg.WordHits += details.WordHits
		}
		if len(details.Spans) > 0 {
			agg.Spans = append(agg.Spans, details.Spans...)
		}
		if details.Start >= 0 && details.Start < agg.Start {
			agg.Start = details.Start
		}
		if details.End > agg.End {
			agg.End = details.End
		}
	}

	if agg.Start == math.MaxInt32 {
		agg.Start = -1
	}
	if len(agg.Spans) > 1 {
		sort.Slice(agg.Spans, func(i, j int) bool {
			if agg.Spans[i].Start == agg.Spans[j].Start {
				return agg.Spans[i].End < agg.Spans[j].End
			}
			return agg.Spans[i].Start < agg.Spans[j].Start
		})
		agg.Spans = MergeMatchSpans(agg.Spans)
	}
	if agg.TargetLength == 0 {
		agg.TargetLength = utf8.RuneCountInString(text)
	}

	return totalScore, agg, true
}
