package search

import (
	"context"
	"unicode/utf8"
)

const streamingEmitThreshold = 64

// SearchRecursiveAsync performs global search asynchronously by streaming index updates.
func (gs *GlobalSearcher) SearchRecursiveAsync(query string, caseSensitive bool, callback func(results []GlobalSearchResult, isDone bool, inProgress bool)) {
	gs.cancelOngoingSearch()

	if cached, ok := gs.lookupCache(query, caseSensitive); ok {
		go callback(cached, true, false)
		return
	}

	if ready, count, useIndex := gs.indexSnapshot(); ready && useIndex && count > 0 {
		go func() {
			results := gs.searchIndex(query, caseSensitive)
			gs.storeCache(query, caseSensitive, results)
			callback(results, true, false)
		}()
		return
	}

	tokens, matchAll := prepareQueryTokens(query, caseSensitive)
	gs.orderTokens(tokens)

	ctx, cancel := context.WithCancel(context.Background())
	token := gs.setCancel(cancel)

	gs.ensureIndexStream()

	go gs.streamFromIndex(ctx, cancel, token, query, caseSensitive, tokens, matchAll, callback)
}

func (gs *GlobalSearcher) streamFromIndex(ctx context.Context, cancel context.CancelFunc, token int, query string, caseSensitive bool, tokens []queryToken, matchAll bool, callback func([]GlobalSearchResult, bool, bool)) {
	defer gs.clearCancel(token)
	defer cancel()

	observer := gs.newIndexObserver()
	if observer == nil {
		callback(nil, true, false)
		return
	}
	defer observer.Close()

	acc := newAsyncAccumulator(256, callback, false)
	defer acc.Close()

	processed := 0
	hasResults := false
	pendingMatches := 0

	for {
		snap, ok := observer.Next(ctx)
		if !ok {
			return
		}
		if snap.Err != nil {
			break
		}

		if snap.Count > processed {
			added := gs.collectIndexRange(acc, processed, snap.Count, tokens, matchAll, query, caseSensitive)
			processed = snap.Count
			if added > 0 {
				hasResults = true
				pendingMatches += added
				if pendingMatches >= streamingEmitThreshold {
					acc.Flush(false)
					pendingMatches = 0
				}
			}
		}

		if snap.Ready {
			break
		}
	}

	acc.FlushRemaining()
	finalResults := acc.FinalResults()

	if hasResults && len(finalResults) >= mergeStatusMinimumResults {
		callback(finalResults, true, true)
	}
	gs.storeCache(query, caseSensitive, finalResults)
	callback(finalResults, true, false)
}

func (gs *GlobalSearcher) collectIndexRange(acc *asyncAccumulator, start, end int, tokens []queryToken, matchAll bool, query string, caseSensitive bool) int {
	entries := gs.snapshotEntries(start, end)
	if len(entries) == 0 {
		return 0
	}

	hasMatch := !matchAll
	spanMode := indexSpanMode
	added := 0
	for i := range entries {
		entry := &entries[i]
		relPath := entry.relPath
		score, matched, details := gs.matchTokens(tokens, relPath, caseSensitive, matchAll, spanMode)
		if !matched {
			releasePositions(details.Positions)
			continue
		}

		score += computeSegmentBoost(query, relPath, details)

		pathLength := details.TargetLength
		if pathLength == 0 {
			pathLength = utf8.RuneCountInString(relPath)
		}
		pathSegments := countPathSegments(relPath)

		finalDetails := gs.materializeIndexDetails(spanMode, tokens, relPath, caseSensitive, matchAll, details)
		result := gs.makeIndexedResult(entry, score, pathLength, finalDetails.Start, finalDetails.End, finalDetails.MatchCount, finalDetails.WordHits, pathSegments, hasMatch, finalDetails.Spans)
		acc.Add(result)
		added++
	}
	return added
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
