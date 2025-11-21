# TODO

## Preview Streaming

- [x] **Streaming search inside pager**
  Large text previews should support `/pattern` search without loading everything. We’ll do literal-only search with SIMD fast paths (AVX2/AVX-512 on amd64, NEON on arm64) and chunked streaming fallback.  
  Plan:
  - [x] Input/UI: add `/` to enter search mode, show query + hit index in status bar, `n/N` (or arrows) to jump next/prev, `Esc` to exit.
  - [ ] Matcher: streaming literal search is in place; add SIMD fast paths (`memmem`) with chunk overlap and keep line/col offsets for hits (pending).
  - [x] Highlighting: store hit spans per line; on render, map spans to wrapped rows and inject ANSI; binary mode skips search.
  - [x] Navigation: maintain hit list + cursor; jumps update `PreviewScrollOffset`/`PreviewWrapOffset` to center the hit.
  - [ ] Performance/limits: cap buffered hits (e.g., 10k) and buffered lines (e.g., 20k); debounce query changes; fallback to pure Go `bytes.Index` when SIMD unavailable. (Optional tuning remains.)

- [x] **UTF-16 pager streaming**
  `textPagerSource` now streams UTF-16 LE/BE previews with shared decoder state, byte-offset aware line metadata, and copy-all support that loads the full file without re-decoding the head buffer.

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

- [ ] **Optional: auto-jump search anchor**  
  If we enable live jumping while typing, anchor to the view center on entering `/`, auto-center the first hit at/after that anchor as the query updates (debounced), and reset the anchor after manual jumps (`n/N/Enter`). Clearing (←) or `Esc` should restore the anchored view and clear results. Gate behind a preference (e.g., `PreviewSearchAutoJump`) to avoid surprising motion by default.

## Binary Pager Improvements

- [x] **Block jumps + offset status**  
  Add `[ / ]` for ±4 KB and `{ / }` for ±64 KB jumps in `PreviewPager` for binaries; status shows current offset and file %. Pager-only; no preview panel change.

- [ ] **Binary search**  
  Remove the `/` blockade in binary mode (`pager.go:1162-1168`) after adding simple byte/ASCII search on `binarySource`. Support `n/N`, hit highlighting, respect `searchMaxHits/Lines`. Preview panel remains without search.

- [ ] **Adaptive hexdump width (pager)**  
  Let `binaryPagerSource` emit 8/16/24 bytes per row based on pager width; size offset/hex/ASCII columns accordingly. Inline preview stays fixed at 16 B to remain lightweight.

- [ ] **Dual-column hex + text**  
  Add a pager mode where each line shows hex plus parallel ASCII/UTF-8 decoding with non-printables masked; simple key toggle. Preview panel unchanged.

- [ ] **Export visible block**  
  Extend existing `copyVisibleToClipboard` so binary mode can copy the current view as hex+ASCII (e.g., the ~256 B on screen) and optionally save to `temp/`. Pager-only.

- [ ] **Lightweight format detection**  
  Detect signatures (PNG/ZIP/ELF/Mach-O/PDF) when loading binaries; show a terse header in preview (e.g., type + section sizes) and richer detail in pager. Minimal I/O: first few hundred bytes, no full parse.  
