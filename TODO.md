# TODO

## Preview Streaming

- [ ] **Streaming search inside pager**  
  Large text previews should support `/pattern` search without loading everything. Consider two-phase search: scan current chunk first, then stream subsequent chunks, highlighting matches as they load.

- [ ] **Window resize on Windows pager**  
  Unix pagers now redraw immediately when the terminal resizes (SIGWINCH + `select`). Windows still requires a keypress because we never consume `WINDOW_BUFFER_SIZE_EVENT`. Implement a Windows-specific key reader that uses `ReadConsoleInput`/`golang.org/x/sys/windows` to listen for both `KEY_EVENT` and `WINDOW_BUFFER_SIZE_EVENT`, toggles raw mode via `SetConsoleMode`, and pushes synthetic resize events into the pager loop so `p.updateSize()`/`render()` fire without waiting for input.

## Performance & Search

- [ ] **Token heuristics & order**  
  During AND queries we currently sort tokens only by length. Explore heuristics based on selectivity (rareness, occurrence counts) to run the most discriminating token first and avoid extra DP passes.

- [x] **Lower-case name cache**  
  Case-insensitive filtering/search constantly calls `strings.ToLower` per filename/path. Cache folded names in `AppState` (and refresh when directories reload) to reduce CPU/allocs on large trees.

- [x] **Result pooling**  
  `GlobalSearchResult`/`FileEntry` allocations still dominate AND benchmarks (~5 MB/op). Investigate pooling or avoiding `os.FileInfo` until a result is promoted by the collector.

## Testing & QA

- [ ] **Preview pager PTY tests**  
  Add end-to-end tests that run `PreviewPager.Run()` against a pseudo-terminal created with the standard library (`golang.org/x/term` + `os.Pipe`/`syscall`), drive key presses (e.g., `q`, `w`, `PgDn`), and assert that the pager restores cursor visibility/DECAWM and renders expected headers/status for text and binary files. This would catch regressje typu “ukryty kursor po wyjściu” i dokumentować wymianę sekwencji sterujących.
