package search

import (
	"math"
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
	textRunes, textBuf := acquireRunes(relPath, fold)
	defer releaseRunes(textBuf)

	totalScore := 0.0
	agg := MatchDetails{
		Start:        math.MaxInt32,
		End:          -1,
		TargetLength: len(textRunes),
	}

	for _, token := range tokens {
		score, matched, details := gs.matcher.MatchDetailedFromRunes(token.pattern, token.runes, relPath, textRunes)
		if !matched {
			return 0, false, MatchDetails{}
		}
		totalScore += score

		if details.MatchCount > 0 {
			agg.MatchCount += details.MatchCount
		}
		if details.WordHits > 0 {
			agg.WordHits += details.WordHits
		}
		if spans := extractTokenSpans(token.runes, textRunes); len(spans) > 0 {
			agg.Spans = append(agg.Spans, spans...)
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
	}
	if agg.TargetLength == 0 {
		agg.TargetLength = utf8.RuneCountInString(relPath)
	}

	return totalScore / float64(len(tokens)), true, agg
}

func prepareQueryTokens(query string, caseSensitive bool) ([]queryToken, bool) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, true
	}
	rawTokens := splitQueryTokens(query)
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

func extractTokenSpans(pattern []rune, text []rune) []MatchSpan {
	if len(pattern) == 0 || len(text) == 0 {
		return nil
	}

	positions := make([]int, 0, len(pattern))
	targetIdx := 0
	for idx, ru := range text {
		if targetIdx >= len(pattern) {
			break
		}
		if ru != pattern[targetIdx] {
			continue
		}
		positions = append(positions, idx)
		targetIdx++
		if targetIdx == len(pattern) {
			break
		}
	}

	if len(positions) == 0 {
		return nil
	}

	spans := make([]MatchSpan, 0, len(positions))
	spanStart := positions[0]
	prev := positions[0]
	for i := 1; i < len(positions); i++ {
		if positions[i] == prev+1 {
			prev = positions[i]
			continue
		}
		spans = append(spans, MatchSpan{Start: spanStart, End: prev})
		spanStart = positions[i]
		prev = positions[i]
	}
	spans = append(spans, MatchSpan{Start: spanStart, End: prev})
	return spans
}
