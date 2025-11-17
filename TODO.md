# TODO

## Preview Streaming

- [ ] **Streaming search inside pager**  
  Large text previews should support `/pattern` search without loading everything. We’ll do literal-only search with SIMD fast paths (AVX2/AVX-512 on amd64, NEON on arm64) and chunked streaming fallback.  
  Plan:
  1) Input/UI: add `/` to enter search mode, show query + hit index in status bar, `n/N` (or arrows) to jump next/prev, `Esc` to exit.  
  2) Matcher: implement chunked literal search (`memmem`) with arch-specific SIMD helpers; overlap chunk by `len(pat)-1`; keep line/col offsets for hits.  
  3) Highlighting: store hit spans per line; on render, map spans to wrapped rows and inject ANSI; binary mode skips search.  
  4) Navigation: maintain hit list + cursor; jumps update `PreviewScrollOffset`/`PreviewWrapOffset` to center the hit.  
  5) Performance/limits: cap buffered hits (e.g., 10k) and buffered lines (e.g., 20k); debounce query changes; fallback to pure Go `bytes.Index` when SIMD unavailable.

- [ ] **Window resize on Windows pager**  
  Unix pagers now redraw immediately when the terminal resizes (SIGWINCH + `select`). Windows still requires a keypress because we never consume `WINDOW_BUFFER_SIZE_EVENT`. Implement a Windows-specific key reader that uses `ReadConsoleInput`/`golang.org/x/sys/windows` to listen for both `KEY_EVENT` and `WINDOW_BUFFER_SIZE_EVENT`, toggles raw mode via `SetConsoleMode`, and pushes synthetic resize events into the pager loop so `p.updateSize()`/`render()` fire without waiting for input.

## Performance & Search

- [x] **Token heuristics & order**  
  During AND queries we currently sort tokens only by length. Explore heuristics based on selectivity (rareness, occurrence counts) to run the most discriminating token first and avoid extra DP passes.

- [x] **Lower-case name cache**  
  Case-insensitive filtering/search constantly calls `strings.ToLower` per filename/path. Cache folded names in `AppState` (and refresh when directories reload) to reduce CPU/allocs on large trees.

- [x] **Result pooling**  
  `GlobalSearchResult`/`FileEntry` allocations still dominate AND benchmarks (~5 MB/op). Investigate pooling or avoiding `os.FileInfo` until a result is promoted by the collector.

## Testing & QA

- [ ] **Preview pager PTY tests**  
  Add end-to-end tests that run `PreviewPager.Run()` against a pseudo-terminal created with the standard library (`golang.org/x/term` + `os.Pipe`/`syscall`), drive key presses (e.g., `q`, `w`, `PgDn`), and assert that the pager restores cursor visibility/DECAWM and renders expected headers/status for text and binary files. This would catch regressje typu “ukryty kursor po wyjściu” i dokumentować wymianę sekwencji sterujących.
