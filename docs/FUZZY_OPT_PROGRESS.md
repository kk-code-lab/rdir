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

## Update (2025-11-16, Apple M1, `go test ./internal/search -bench . -run '^$' -count=1`)

- Token ordering uses bucket ratios + two-rune fingerprint selectivity.
- `BenchmarkFuzzyGlobalSearcherANDWalk`: **70.9µs/op**, 442.9KB/op, 9 allocs/op
- `BenchmarkFuzzyGlobalSearcherANDIndex`: **4.93ms/op**, 4.47MB/op, 28,885 allocs/op
