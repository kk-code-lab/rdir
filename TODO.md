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

## Performance & Search

- [ ] **Token heuristics & order**  
  During AND queries we currently sort tokens only by length. Explore heuristics based on selectivity (rareness, occurrence counts) to run the most discriminating token first and avoid extra DP passes.

- [x] **Substring pre-check reuse**  
  Inline filter and global search now share the same multi-token logic (no extra substring guard), so queries like `foo bar` behave identically everywhere.

- [ ] **Lower-case name cache**  
  Case-insensitive filtering/search constantly calls `strings.ToLower` per filename/path. Cache folded names in `AppState` (and refresh when directories reload) to reduce CPU/allocs on large trees.

- [ ] **Result pooling**  
  `GlobalSearchResult`/`FileEntry` allocations still dominate AND benchmarks (~5 MB/op). Investigate pooling or avoiding `os.FileInfo` until a result is promoted by the collector.
