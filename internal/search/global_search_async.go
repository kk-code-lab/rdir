package search

import (
	"context"
	"errors"
	"io/fs"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

// SearchRecursiveAsync performs global search asynchronously in a goroutine.
func (gs *GlobalSearcher) SearchRecursiveAsync(query string, caseSensitive bool, callback func(results []GlobalSearchResult, isDone bool, inProgress bool)) {
	gs.cancelOngoingSearch()

	if gs.maybeUseIndex() {
		ctx, cancel := context.WithCancel(context.Background())
		token := gs.setCancel(cancel)
		go func(token int, ctx context.Context, cancel context.CancelFunc) {
			defer gs.clearCancel(token)
			defer cancel()

			results := gs.searchIndex(query, caseSensitive)

			if !gs.isTokenCurrent(token) {
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			callback(results, true, false)
		}(token, ctx, cancel)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	token := gs.setCancel(cancel)

	go func(cancel context.CancelFunc, token int) {
		defer gs.clearCancel(token)
		defer cancel()

		results := borrowResultBuffer(256)
		defer func() {
			releaseResultBuffer(results)
		}()

		var sortedSnapshot []GlobalSearchResult
		var mu sync.Mutex
		lastCallbackTime := time.Now()
		lastCallbackSize := 0
		filesScanned := 0

		orderCounter := 0

		err := gs.walkFilesBFS(ctx, func(path string, relPath string, d fs.DirEntry) error {
			filesScanned++
			gs.maybeKickoffIndex(filesScanned)

			score, matched, details := gs.matcher.MatchDetailed(query, relPath)
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
			hasMatch := len(query) > 0
			result := makeGlobalSearchResult(path, d, info, score, pathLength, matchStart, matchEnd, matchCount, wordHits, pathSegments, order, hasMatch)

			mu.Lock()
			if len(results) == cap(results) {
				results = growResultBuffer(results, len(results)+1)
			}
			results = append(results, result)
			mu.Unlock()

			if flushBatch := shouldFlushBatch(lastCallbackSize, len(results), lastCallbackTime); flushBatch {
				mu.Lock()
				currentSize := len(results)
				if currentSize > lastCallbackSize {
					newResults := borrowResultBuffer(currentSize - lastCallbackSize)
					newResults = append(newResults, results[lastCallbackSize:currentSize]...)

					mu.Unlock()

					sort.Slice(newResults, func(i, j int) bool {
						return compareResults(newResults[i], newResults[j]) < 0
					})

					prevSnapshot := sortedSnapshot
					sortedSnapshot = mergeResults(sortedSnapshot, newResults)
					releaseResultBuffer(newResults)
					releaseResultBuffer(prevSnapshot)

					displaySnapshot := sortedSnapshot
					if len(displaySnapshot) > maxDisplayResults {
						displaySnapshot = displaySnapshot[:maxDisplayResults]
					}

					callback(displaySnapshot, false, true)
					lastCallbackTime = time.Now()
					lastCallbackSize = currentSize
				} else {
					mu.Unlock()
				}
			}

			return nil
		})

		if errors.Is(err, context.Canceled) {
			return
		}

		gs.considerIndexBuildAfterWalk(filesScanned)

		mu.Lock()
		defer mu.Unlock()

		var finalResults []GlobalSearchResult
		if len(results) > lastCallbackSize {
			unsortedFinal := results[lastCallbackSize:]
			sort.Slice(unsortedFinal, func(i, j int) bool {
				return compareResults(unsortedFinal[i], unsortedFinal[j]) < 0
			})

			prevSnapshot := sortedSnapshot
			sortedSnapshot = mergeResults(sortedSnapshot, unsortedFinal)
			releaseResultBuffer(prevSnapshot)
			finalResults = sortedSnapshot
		} else {
			finalResults = sortedSnapshot
		}

		if len(finalResults) > maxDisplayResults {
			finalResults = finalResults[:maxDisplayResults]
		}

		if len(finalResults) >= mergeStatusMinimumResults {
			callback(finalResults, true, true)
		}
		callback(finalResults, true, false)
	}(cancel, token)
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
