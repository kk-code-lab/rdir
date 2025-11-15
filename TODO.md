# TODO

## Platform Parity

- [x] **Shell detection defaults**  
  `detectShell` in `cmd/rdir/main.go` only inspects `$SHELL` and falls back to `bash`. On Windows or other shells where `$SHELL` is unset, `rdir --setup` never emits PowerShell/CMD snippets unless the user passes `--setup=...`. Teach detection to branch on `runtime.GOOS`, `COMSPEC`, or parent-process metadata so Windows shells receive the right integration automatically (still keep the explicit override flag).

- [x] **Pager command on Windows**  
  Pager detection now parses `$PAGER`, defaults to `more.com`/`cmd /C type` on Windows, and runs commands directly instead of shelling through `/bin/sh`, so the documented fallback finally works cross-platform.

- [x] **Clipboard integration**  
  Clipboard detection now returns concrete argument lists, preferring `clip.exe`/PowerShell on Windows and the usual UNIX tools elsewhere, so yank works cross-platform.

- [x] **Editor defaults on Windows**  
  Editor detection now falls back to platform-specific defaults (`code --wait`, Notepad++/Notepad on Windows, vim/nano otherwise) and opening an editor no longer depends on `/dev/tty` for Windows, so the `e` shortcut works across platforms.

- [x] **Hidden-file detection uses wrong paths**  
  Hidden-file checks now receive full paths everywhere, so Windows can consult `GetFileAttributes` correctly. Directory loading, parent sidebar, and global search all pass the absolute path, and tests cover mixed hidden/visible data.


## Preview Streaming

- [x] **Chunked text previews**  
  Fullscreen pager now streams multi-GB logs via `textPagerSource`, using `ReadAt` plus newline-aware chunking so responsiveness no longer depends on 64 KB caps.

- [x] **Shared wrap metadata**  
  Per-line offsets, rune counts, and display widths live in `PreviewData.TextLineMeta`, letting both the sidebar preview and pager reuse wrap math without re-reading files.

- [ ] **Streaming search inside pager**  
  Large text previews should support `/pattern` search without loading everything. Consider two-phase search: scan current chunk first, then stream subsequent chunks, highlighting matches as they load.

- [ ] **Window resize on Windows pager**  
  Unix pagers now redraw immediately when the terminal resizes (SIGWINCH + `select`). Windows still requires a keypress because we never consume `WINDOW_BUFFER_SIZE_EVENT`. Implement a Windows-specific key reader that uses `ReadConsoleInput`/`golang.org/x/sys/windows` to listen for both `KEY_EVENT` and `WINDOW_BUFFER_SIZE_EVENT`, toggles raw mode via `SetConsoleMode`, and pushes synthetic resize events into the pager loop so `p.updateSize()`/`render()` fire without waiting for input.

## Preview Formatters

- [x] **Markdown parser without new deps**  
  Internal parser replaces regex with block/inline tokenizers, AST, and fancy formatted output: headings, lists, code fences/indented blocks, blockquotes, HR, links/images, autolinks, strike, tables with box-drawing, and styled segments for preview/pager (full-width HR, fancy bullets). Limits and truncation behavior preserved; tests cover edge cases and table rendering.

## Performance & Search

- [ ] **Token heuristics & order**  
  During AND queries we currently sort tokens only by length. Explore heuristics based on selectivity (rareness, occurrence counts) to run the most discriminating token first and avoid extra DP passes.

- [x] **Substring pre-check reuse**  
  Inline filter and global search now share the same multi-token logic (no extra substring guard), so queries like `foo bar` behave identically everywhere.

- [ ] **Lower-case name cache**  
  Case-insensitive filtering/search constantly calls `strings.ToLower` per filename/path. Cache folded names in `AppState` (and refresh when directories reload) to reduce CPU/allocs on large trees.

- [ ] **Result pooling**  
  `GlobalSearchResult`/`FileEntry` allocations still dominate AND benchmarks (~5 MB/op). Investigate pooling or avoiding `os.FileInfo` until a result is promoted by the collector.

## Testing & QA

- [ ] **Preview pager PTY tests**  
  Add end-to-end tests that run `PreviewPager.Run()` against a pseudo-terminal created with the standard library (`golang.org/x/term` + `os.Pipe`/`syscall`), drive key presses (e.g., `q`, `w`, `PgDn`), and assert that the pager restores cursor visibility/DECAWM and renders expected headers/status for text and binary files. This would catch regressje typu “ukryty kursor po wyjściu” i dokumentować wymianę sekwencji sterujących.

- [x] **Terminal injection audit**  
  Walk every surface that prints user-controlled text (status bar, directory list, error banners, etc.) and ensure the same sanitization used in the pager header/dir entries is applied everywhere so ANSI escapes can’t leak to the terminal.
- [x] **External command safety review**  
  Inventory all `exec.Command` usages (editor, pager, clipboard, future hooks) to confirm no shell invocation paths exist, arguments remain separated, and non-zero exits propagate back to the UI/logs. Add regression tests for the most common flows while at it.
