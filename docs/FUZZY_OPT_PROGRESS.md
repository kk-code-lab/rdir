# Fuzzy Optimizations Progress

We track fuzzy/search performance work here so future iterations have a clear baseline.

## Baseline (2025-11-16, Apple M1, `go test ./internal/search -bench . -run '^$' -count=1`)

- `BenchmarkFuzzyGlobalSearcherANDWalk`: **71.9µs/op**, 442.9KB/op, 9 allocs/op
- `BenchmarkFuzzyGlobalSearcherANDIndex`: **5.07ms/op**, 4.47MB/op, 28,889 allocs/op

These runs include the token selectivity heuristic (smallest index rune bucket first) and the async benchmark guard.

## Direction

- Continue refining selectivity: consider bucket-size ratios or multi-rune fingerprints instead of min-single-rune only.
- Measure impact on both walk and index paths; keep AND benchmarks (`...ANDWalk`, `...ANDIndex`) as the primary bellwethers.
- Track allocation drops; AND paths are still allocation-heavy even after pooling work.
- Re-run this suite after each heuristic/tuning change and append results here with command + date.

## Update (2025-11-16, Apple M1, `go test ./internal/search -bench . -run '^$' -count=1`) – SpanNone default (positions/ full available via env)

- Token ordering uses bucket ratios + multi-rune fingerprint (2-rune min + best/median skew).
- Result caching avoids per-call result copying to cut allocs on AND paths.
- `BenchmarkFuzzyGlobalSearcherANDWalk`: **0.56µs/op**, 202B/op, 4 allocs/op
- `BenchmarkFuzzyGlobalSearcherANDIndex`: **5.03ms/op**, 4.47MB/op, 28,887 allocs/op

## Update (2025-11-16, Apple M1, `go test ./internal/search -bench . -run '^$' -count=1`)

- Added spanRequest to build spans lazily; index now defaults to `spanNone` to avoid span work on the first pass (set `RDIR_INDEX_LAZY_SPANS=positions|full` to change).
- `BenchmarkFuzzyGlobalSearcherANDWalk`: **0.55µs/op**, 202B/op, 4 allocs/op
- `BenchmarkFuzzyGlobalSearcherANDIndex`: **5.01ms/op**, 4.40MB/op, 26,640 allocs/op (spanPositions run; spanNone saves more allocs with a top-K rerun).
- Index lazy spans benchmark (`go test ./internal/search -bench IndexLazySpans -benchmem`):
  - `SpanFull`: **2.83ms/op**, 1.41MB/op, 39,053 allocs/op
  - `SpanPositions`: **2.59ms/op**, 0.90MB/op, 18,829 allocs/op
  - `SpanNoneRerun`: **2.27ms/op**, 0.64MB/op, 8,329 allocs/op

## Update (2025-11-16, Apple M1, `go test ./internal/search -bench IndexCandidates -run '^$' -count=1`)

- Added `BenchmarkIndexCandidatesAND` to contrast bitset-bucket filtering vs a sequential fallback on ~205K indexed files.
  - `BitsetBuckets`: **0.69ms/op**, 1.18KB/op, 11 allocs/op, ~5k candidates after intersect/precheck
  - `SequentialFallback`: **0.48ms/op**, 0B/op, 0 allocs/op
  - Reports `candidates_pre`/`candidates_post` to make filtering impact visible.

## Plan (2025-11-16, prioritised steps for >1M entries)

1. ~~Expand index candidate filtering: bitset/bigrams over the full path, intersect buckets for all tokens to slash N before hitting the matcher.~~
2. ~~Add cheap pre-checks before DP: `strings.Contains` on `lowerPath` for each token and an upper-bound score guard vs the heap minimum to skip costly matches.~~ (heap guard still pending)
3. Cache pre-folded []rune + boundary bits in the index and reuse them in `matchTokens`, avoiding per-hit `acquireRunes`/`boundaryBuffer`.
4. Parallelise candidate scoring (shards + worker pool + final top-K heap), with per-worker matchers/buffers to scale across cores.
5. After validation, turn SIMD DP on by default (NEON DP32) and add AVX2 for amd64 so ASCII-heavy paths run through faster DP.
