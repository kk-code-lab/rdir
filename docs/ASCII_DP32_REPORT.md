Handoff Summary: ASCII-DP32 (NEON) Integration in rdir

Current Progress and Key Decisions
- ARM64 NEON primitives (asm) implemented and tested:
  - copyRangeF32Asm: chunk-8 + tail using VLD1/VST1
  - setIdxRangeAsm: chunk-8 + tail using VST1
  - setRangeF32NegInfAsm: chunk-8 + tail with RODATA -Inf block
- Go wrappers added: asmCopyRangeF32, asmSetIdxRange, asmCopyPrefixRange, plus tiny one-element primitives (fuzzyWriteOneAsm, fuzzyWriteOneIdxAsm, fuzzyCopyOneF32Asm).
- Extensive tests and microbenchmarks:
  - ASM vs pure Go (8/64/1024): ~75–80% improvement for 64+ elements on Apple M1
  - DP ASCII32 forced vs scalar: Short cases faster, Medium/Long slower while migration is partial
- Experimental ASCII-DP32 path (internal/fuzzy_dp_ascii32.go):
  - Controlled via env flag RDIR_EXPERIMENTAL_ASCII_DP32=1 (or test override ascii32Force)
  - Computes init, first row, prefix window, and up to 64 rows in float32 on a band up to 2048 columns; captures backtrack
  - For envelope m ≤ 64 and n ≤ 2048 returns DP32 result directly (no scalar)
  - Outside envelope: exercises NEON but delegates to scalar to keep outputs identical
- Report added: ASCII_DP32_REPORT.md (status, plan, measurements)
- User preference: rigorous, fail-fast (no silent fallbacks), small measurable steps, add benchmarks and tests, maintain 1e-6 score tolerance when comparing float32 vs float64

Latest Progress (2025-11-06)
- Chunk-8 prefix window loop now in asm with reusable inline macro (PREFIX_LANE). Scalar behaviour unchanged, but duplication reduced and code is ready for NEON mask experiments.
- Verified benchmarks (Apple M1, Go 1.22) after refactor:
  - DP ASCII32 forced: Single 129±1 ns/op, Medium 1.43–1.49 µs/op, Long ~0.25–0.26 ms/op (still slower than scalar on Medium/Long, but no regression vs previous scaffolding).
  - Baseline scalar path (flag off): Single 129–130 ns/op, Medium ~1.44 µs/op, Long ~0.25 ms/op.
- Attempt to outsource per-lane work to a BL subroutine caused the chunk loop to spin forever (R3 never reached zero). Lesson: per-lane updates must stay inline until the NEON formulation is proven.
- Current experiments: prepared plan to introduce NEON masks (VCMGT/VBSL) inside the macro; not yet implemented, so behaviour and perf unchanged.
- Observed limitation: keeping the DP structure strictly column-by-column makes it difficult to gain large NEON speedups; meaningful SIMD gains will likely require a higher-level reformulation (prefix scan style or tiled DP blocks).

Important Context & Constraints
- Do not change default behavior unless ASCII-DP32 flag is enabled; default path remains scalar/ASM ASCII
- Tests must remain green; use 1e-6 tolerance for score deltas in float32 vs float64 comparisons
- Keep integration incremental; measure impact via benchmarks after each step
- Platform: focus on ARM64 (asm guarded by //go:build arm64 && !purego)

What Remains (Next Steps)
- ASCII-DP core migration (float32 path):
  - Lift remaining scalar sections once envelope (m ≤ 64, n ≤ 2048) is stable.
  - Gradually extend envelope size after perf gains are confirmed.
- Prefix window NEON work:
  1. Extend `PREFIX_LANE` to operate on 4-column sub-blocks (mask-friendly).
  2. Introduce `VCMGT`/`VBSL`-based masks to apply gap decay and candidate selection per chunk, but keep scalar finalisation until correctness is proven.
  3. Only after benchmarks show wins, translate argmax/bestIdx updates to NEON.
- Argmax on dpPrev32: reuse macro approach, explore chunked mask scans.
- Expand ASCII-DP32 envelope stepwise once tests/benchmarks confirm improvements.
- Keep running `TestASCII32EqualsScalarWhenForced`, `RDIR_EXPERIMENTAL_ASCII_DP32=1 make bench-fuzzy`, and `make bench-fuzzy` after every change.
- Investigate higher-level SIMD-friendly formulations:
  - Prefix-scan (Blelloch-style) decomposition: compute block-local prefix maxima in SIMD, then propagate block summaries to next blocks.
  - Tiled DP blocks (rectangle of 8×8 columns): use NEON to update entire tiles before reconciling edges, reducing per-column dependency pressure.

Critical Data, Examples, References
- Files to continue work:
  - internal/fuzzy_dp_ascii32.go (main ASCII-DP32 implementation)
  - internal/fuzzy_dp_ascii_arm64.s/.go (NEON primitives)
  - internal/search/fuzzy.go (flags: RDIR_EXPERIMENTAL_ASCII_DP32; ascii32Force in tests)
  - internal/search/fuzzy_ascii32_integration_test.go (equivalence tests, 1e-6 tolerance)
  - internal/search/asm_benchmark_test.go (benchmarks: DPAscii32 vs DPScalar; primitives ASM vs Go)
  - ASCII_DP32_REPORT.md (status/plan)
- Current envelope (returns DP32 result without scalar):
  - m ≤ 64, n ≤ 2048
- Benchmarks (Apple M1):
  - Primitives: setIdxRange/copyRangeF32 ~75–80% faster than Go for 64+ elements
  - DPASCII32 forced:
    - Short ~170 ns/op vs scalar ~183 ns/op (faster)
    - Medium ~2565 ns/op vs scalar ~2171 ns/op
    - Long ~11900 ns/op vs scalar ~7890 ns/op
- Improvement already visible for small cases; larger cases need more migration/wektoryzacja to win
- Prefix window NEON attempt (2025-11-06):
  - Tried translating the scalar recurrence directly to arm64 asm/NEON.
  - Hard dependency chain (each column depends on previous best) kept collapsing to scalar code; no safe vector-friendly formulation surfaced.
  - Assembly experiment collided with ABI details (pointer writes, register preservation) and was dropped before landing any changes.
  - Recommendation: prototype chunked Go version with explicit state passing, confirm exact dependency graph, then decide if NEON is viable; avoid touching asm until a clear, dependency-free unroll exists.

Execution Tips
- Maintain fail-fast tests; no silent fallbacks masking asm issues
- After each change: run TestASCII32EqualsScalarWhenForced and DP benchmarks (both ASCII32 forced and scalar)
- If needing broader vectorization: start with chunk-8 unroll in Go, then translate to NEON

End Goal
- Full ASCII-DP32 (float32 + NEON) coverage for typical DP scenarios (Medium/Long), yielding consistent improvements in ns/op compared to scalar, with default behavior switchable via flag (and eventually enabled by default once stable).

Latest Progress (2025-11-07)
- Rebuilt the ASCII32 chunk pipeline so matches now carry precomputed prefix/prev candidates; the NEON chunker can operate allocation-free and falls back to scalar only for the tail.
- Added env toggles (`RDIR_DISABLE_ASCII32_CHUNK_ASM`, `RDIR_VERIFY_ASCII32_CHUNK_ASM`, `RDIR_DEBUG_ASCII32`) plus extra scratch buffers to debug/validate asm results lane-by-lane.
- Implemented `ascii32ProcessChunkAsm` using raw opcodes for `dup`, `fmax`, and `fcmgt`; the vector section handles multiples of 4 columns while the Go tail retains correctness.
- Current benchmarks (Apple M1, ASCII32 forced): Short 266 ns, Medium 2.45 µs, Long 7.16 µs vs scalar Long 7.99 µs (~10% gain with SIMD-only chunk).

Next Steps (2025-11-xx)
- Extend NEON coverage to 8-lane blocks (or handle tails in asm) so we can vectorize entire chunks without scalar fallbacks.
- Restore an asm prefix window once column offsets are correctly tracked, reducing per-row overhead.
- Evaluate auto-vectorized candidate prep (pre-sum char/word bonuses in asm) to lower Go-side work before hitting NEON.
- When Go's assembler grows the missing mnemonics, replace raw opcodes with readable intrinsics to ease future maintenance.

Latest Progress (2025-11-08)
- Replaced the fragile macro-driven prefix window with a scalar arm64 asm loop that accepts `(start, prevVal)` seeds; it now mirrors `scalarPrefixRef` for any window slice, covers offsets correctly, and is guarded by new verification helpers plus offset-aware tests (full-range + sub-range cases).
- Tightened dp chunk handling: `ascii32ProcessChunk` once again limits asm usage to multiples of four lanes and routes any remainder through the proven scalar helper while we redesign the tail path; added optional debug hooks to inspect lane outputs.
- Discovered the chunk NEON tail still emits invalid scores for <4 matches; until the fix lands we gate the vector path behind a new opt-in (`RDIR_ENABLE_ASCII32_CHUNK_ASM=1`) so correctness stays paramount without breaking the existing benchmarking knobs.
- `make test` (and the focused asm prefix tests) run clean with chunk asm disabled by default, giving a safe baseline to iterate on the revamped prefix + future chunk work.
- Corrected the ABI offset for the `negInf` argument in `ascii32ProcessChunkAsm`, so manual chunk verification now feeds the real `-Inf` sentinel into the scalar tail when diagnosing asm output.
- Added `ascii32ChooseBest`/`ascii32ApplyThreshold` helpers plus `TestAscii32ChooseBest`; chunk asm now returns just a lane mask + score and the Go side reuses the same helper for final selection, keeping scalar/asm logic in sync.
- Latest NEON attempt exposed that `VBSL`/`VBIF` mutate their destination registers; without duplicating masks or keeping pristine copies of prefix/prev candidates, later lanes inherited garbage. Keep extra copies before mixing, or stage the logic in Go first before porting.
- Added `TestAscii32ProcessChunkAsmMatchesScalar` plus chunk-mask debug logs; they quickly flagged regressions while the asm path remains opt-in (`RDIR_ENABLE_ASCII32_CHUNK_ASM`).

Latest Progress (2025-11-09)
- Hardened lane diagnostics with `TestAscii32ProcessChunkAsmMaskAndThreshold`; it covers prev-only winners, invalid indices, and threshold-clamped lanes so mask bugs surface immediately.
- Verified `RDIR_ENABLE_ASCII32_CHUNK_ASM=1 ascii32VerifyChunkASM=1 make test` to ensure the NEON chunk path plus the scalar tail stay bit-for-bit with the reference helpers.
- Bench snapshots (Apple M1, Go 1.22):
  - Chunk asm off: Single 130.8 ns/op, Multiple 1.52 µs/op, Large 264 µs/op.
  - Chunk asm on: Single 131.3 ns/op, Multiple 1.52 µs/op, Large 266 µs/op.
- Interpretation: current NEON chunk path is effectively perf-neutral; the next iteration should focus on removing the Go-side threshold post-processing and covering the tail inside asm so we realize measurable wins.
- Historical note: the temporary `ascii32VerifyChunkASM` env flag (now removed) helped provide lane-by-lane tracing while the experiment was active.

Latest Progress (2025-11-10)
- Retired the `ascii32ProcessChunkAsm` experiment and removed the opt-in flag (`RDIR_ENABLE_ASCII32_CHUNK_ASM`) after repeated benchmarks showed no measurable gain; chunk processing is back to the proven scalar helper.
- Deleted the asm-only tests (`TestAscii32ProcessChunkAsmMatchesScalar`, `TestAscii32ProcessChunkAsmMaskAndThreshold`) and the scratch asm harness, keeping only the reusable helpers that still matter (prefix/set/copy).
- Lessons learned: without moving the thresholding/backtrack writes entirely into NEON, the Go-side overhead eclipses any wins from vectorizing candidate comparison; future SIMD work should target self-contained primitives (prefix/copy) or redesign the DP layout instead of partial lane selection.
