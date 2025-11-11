package search

import (
	"context"
	"errors"
	"io/fs"
	"unicode/utf8"
)

// SearchRecursiveAsync performs global search asynchronously in a goroutine.
func (gs *GlobalSearcher) SearchRecursiveAsync(query string, caseSensitive bool, callback func(results []GlobalSearchResult, isDone bool, inProgress bool)) {
	gs.cancelOngoingSearch()
	tokens, matchAll := prepareQueryTokens(query, caseSensitive)

	ctx, cancel := context.WithCancel(context.Background())
	token := gs.setCancel(cancel)

	startWalker := func() {
		go gs.runWalkerAsync(ctx, cancel, token, query, caseSensitive, tokens, matchAll, callback)
	}

	if gs.maybeUseIndex() {
		go func() {
			results := gs.searchIndex(query, caseSensitive)
			if len(results) > 0 {
				if gs.isTokenCurrent(token) {
					callback(results, true, false)
				}
				cancel()
				gs.clearCancel(token)
				return
			}
			if !gs.isTokenCurrent(token) {
				cancel()
				gs.clearCancel(token)
				return
			}
			startWalker()
		}()
		return
	}

	startWalker()
}

func (gs *GlobalSearcher) runWalkerAsync(ctx context.Context, cancel context.CancelFunc, token int, query string, caseSensitive bool, tokens []queryToken, matchAll bool, callback func([]GlobalSearchResult, bool, bool)) {
	defer gs.clearCancel(token)
	defer cancel()

	acc := newAsyncAccumulator(256, callback)
	defer acc.Close()

	filesScanned := 0
	orderCounter := 0

	err := gs.walkFilesBFS(ctx, func(path string, relPath string, d fs.DirEntry) error {
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
		matchStart := details.Start
		matchEnd := details.End
		matchCount := details.MatchCount
		wordHits := details.WordHits
		pathSegments := countPathSegments(relPath)

		order := orderCounter
		orderCounter++

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		hasMatch := !matchAll
		result := makeGlobalSearchResult(path, d, info, score, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments, order, hasMatch, details.Spans)

		acc.Add(result)
		acc.Flush(false)

		return nil
	})

	if errors.Is(err, context.Canceled) {
		return
	}

	gs.considerIndexBuildAfterWalk(filesScanned)

	acc.FlushRemaining()
	finalResults := acc.FinalResults()

	if len(finalResults) >= mergeStatusMinimumResults {
		callback(finalResults, true, true)
	}
	callback(finalResults, true, false)
}

func (gs *GlobalSearcher) cancelOngoingSearch() {
	gs.cancelMu.Lock()
	defer gs.cancelMu.Unlock()
	if gs.cancel != nil {
		gs.cancel()
		gs.cancel = nil
		gs.token++
	}
}

func (gs *GlobalSearcher) setCancel(cancel context.CancelFunc) int {
	gs.cancelMu.Lock()
	gs.token++
	token := gs.token
	gs.cancel = cancel
	gs.cancelMu.Unlock()
	return token
}

func (gs *GlobalSearcher) clearCancel(token int) {
	gs.cancelMu.Lock()
	if gs.token == token {
		gs.cancel = nil
	}
	gs.cancelMu.Unlock()
}

func (gs *GlobalSearcher) isTokenCurrent(token int) bool {
	gs.cancelMu.Lock()
	defer gs.cancelMu.Unlock()
	return gs.token == token
}
