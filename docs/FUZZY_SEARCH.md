# Fuzzy Search Guide

## Overview

`rdir` uses the same fuzzy engine for two entry points:

1. **Inline filter** (press `/`) – narrows the current directory listing while preserving its original ordering.
2. **Global search** (press `f`) – streams matches from the entire tree, optionally via an index, and sorts them by score.

Both flows share the `internal/search.FuzzyMatcher` and normalize scores to the `[0.0, 1.0]` range. Inline filtering treats the score as a visibility gate (no resorting). Global search still renders highest scores first.

---

## Query Lifecycle

### Tokenisation and Case Sensitivity
- Queries are split on whitespace in `prepareFilterTokens`. Every token must match.
- Case-insensitive matching is the default. If the user types any uppercase rune the reducer flips `FilterCaseSensitive` / `GlobalSearchCaseSensitive` to `true` via `updateCaseSensitivityOnAppend` and `queryHasUppercase`.
- Inline filter keeps `FilterSavedIndex` so ESC can restore the cursor when the user abandons the search.

### Filter recomputation
`AppState.recomputeFilter()`:
1. Reuses a single matcher instance stored on the state (`filterMatcher`).
2. Runs `matchFilterTokens` for each visible file, calling `MatchDetailedFromRunes` so tokens reuse pre-folded rune slices.
3. Accumulates `FilterMatches` (scores) and `FilteredIndices` (file positions) in the same order files appear in the directory listing.
4. Leaves the order untouched and invalidates `displayFilesCache` so the renderer reflects the filtered subset.
5. `retainSelectionAfterFilterChange` keeps the cursor near the previous file when the results shrink.

### Global search
- `GlobalSearcher.SearchRecursiveAsync` streams batch results and uses `mergeResults` to maintain a descending score order in O(n).
- Each `GlobalSearchResult` carries score, word-hit counts, path metadata, and span positions; `compareResults` uses those fields as tie-breakers.
- Async workflows honour hide-hidden settings and cancel in-flight walkers when the query changes.

---

## Matcher Internals (`internal/search/fuzzy.go`)

### Input preparation
- Accepts `pattern`/`text` strings or precomputed rune slices.
- Detects ASCII-only cases and converts to byte slices so tight contiguous matches can skip rune decoding.
- Provides `Match`, `MatchDetailed`, `MatchDetailedWithMode`, and `MatchDetailedFromRunes` to cover simple and advanced callers.

### Scoring Components
The scorer is tuned for dense, word-aligned matches:

| Component | Purpose | Default weight |
|-----------|---------|----------------|
| `charBonus` | Base reward per matched rune | `1.2` |
| `consecutiveBonus` | Rewards uninterrupted runs | `1.2` |
| `wordBoundaryBonus` | Boosts matches after `/`, `_`, or camelCase bumps | `0.6` |
| `substringBonus`, `prefixBonus`, `finalSegmentBonus` | Encourage whole-token, prefix, and suffix hits | `1.2`, `2.4`, `2.0` |
| `gapPenalty` | Penalises skipped runes | `0.18` per rune |
| `caseMismatchPenalty` | Soft penalty when only case differs | `0.1` |
| `startPenaltyFactor` / `crossSegmentPenalty` | Favour early, compact matches | `0.012`, `0.9` |
| `wordHitBonus` | Tie-breaker for matches at word starts | `3.2` |

Additional helpers:
- Boundary bitmasks quickly classify rune transitions (word vs strong boundaries).
- Contiguous substring detection runs first; if we find a literal substring the algorithm records `MatchDetails.Start/End` immediately.
- The final `score` is normalised against an upper bound, yielding a float in `[0,1]`.

### Multi-token aggregation
When the user types multiple terms, each token is scored independently and the arithmetic mean becomes the final score. Both the inline filter and global search reuse this logic so `foo bar` means “find entries containing `foo` and `bar` anywhere in the path” (case rules still depend on the query).

---

## Performance Notes

- ASCII fast path: ASCII-only patterns/texts skip rune allocation and use byte slices.
- SIMD-backed DP32 path: `internal/fuzzy_dp_ascii32.go` plus the NEON assembly helpers accelerate contiguous substring scoring. It is guarded by `RDIR_EXPERIMENTAL_ASCII_DP32=1` (or `ascii32Force` in tests) so new optimisations can be opt-in.
- Result pooling: `MatchMultipleInto` accepts a destination slice to avoid repeated allocations; global search batches also reuse pooled buffers (`borrowResultBuffer`).
- Benchmarks: `make bench-fuzzy` exercises scalar vs ASCII/DP32 paths; `make bench-fuzzy GLOBAL=1` (or directly running `internal/search/fuzzy_benchmark_test.go`) targets the heavier workloads.

---

## Testing

- `internal/search/fuzzy_test.go` – unit tests for scoring, boundaries, ASCII fast path, deterministic ordering.
- `internal/search/fuzzy_ascii32_integration_test.go` / `fuzzy_dp_ascii32_test.go` – ensure the experimental DP path stays within `1e-6` of the scalar implementation.
- `internal/state/fuzzy_integration_test.go` – reducer-level coverage for filter queries, cursor restoration, and hide-hidden interactions.
- `internal/search/global_search_sort_test.go` – regression tests for score/tie-breaking logic in the async global search.

Run them all with:
```bash
make test
# or focus on the matcher
go test ./internal/search -run Fuzzy -v
```

---

## Practical Tips

- Reuse `AppState.filterMatcher` instead of creating new matchers inside tight loops.
- Call `state.invalidateDisplayFilesCache()` every time you mutate filter/query/hidden-file fields so the renderer sees the updated slice.
- When adding new scoring knobs, keep the output within `[0,1]` to avoid destabilising comparisons with existing cached scores.
- Prefer `MatchDetailedFromRunes` when you already folded the pattern/text (e.g. multi-token filter) – it avoids duplicate allocations and keeps the filter responsive on very large directories.
