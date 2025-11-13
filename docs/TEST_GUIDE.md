# Testing Guide – rdir

## Overview

`rdir` follows a Redux-like architecture: reducers own all business logic, while the UI simply renders state. That separation keeps tests fast, deterministic, and runnable without tcell, terminal mocks, or shell glue. This guide explains where the suites live, how to run them, and what to keep in mind when adding coverage.

What we verify:
- State mutations (`internal/state`)
- Filesystem/preview helpers (real directories, text/binary heuristics)
- Fuzzy and global search (`internal/search`)
- UI helpers such as footer text and renderer formatting

---

## Running Tests

Prefer the `Makefile` targets—they bake in the right package set and flags:

```bash
make test            # go test ./internal/...
make test-coverage   # coverage report
make test-race       # concurrency / async search work

# Ad-hoc examples
go test ./internal/state -run TestNavigateDown -v
go test ./internal/search -bench Fuzzy
```

`make test` typically finishes in under a second. If it suddenly gets slow, look for runaway goroutines, blocking I/O, or an accidental infinite loop.

---

## Test Layout

Every package keeps its own `_test.go` files under `internal/...`. Key areas:

### `internal/state`
- **Reducer suites** (`reducer_*.go`) split by concern:
  - `reducer_navigation_test.go` – cursor movement, paging, scroll bounds
  - `reducer_filter_test.go` – filter typing, backspace, ESC flows
  - `reducer_hidden_files_test.go` – `ToggleHiddenFiles`, cursor placement
  - `reducer_filter_hidden_test.go` – interactions between filter + hidden flag
  - `reducer_state_test.go` – helpers (`getDisplayFiles`, `sortFiles`, etc.)
  - Extras: `reducer_history_simple_test.go`, `test_esc_logic_test.go`,
    `test_filter_cursor_restoration_test.go`
- **I/O & previews** – `reducer_io_test.go` works on temporary directories to cover
  `LoadDirectory`, preview generation, and the text/binary heuristic.
- **Fuzzy integration** – `fuzzy_integration_test.go` drives the full filter pipeline so
  `FilterMatches`, `FilteredIndices`, and selection memory stay in sync.

### `internal/search`
- `fuzzy_test.go`, `fuzzy_benchmark_test.go`, `fuzzy_ascii32_integration_test.go`,
  `fuzzy_dp_ascii*_test.go` – matcher behaviour, SIMD/DP32 paths, score tolerances.
- `global_search_*_test.go`, `global_search_benchmark_test.go`,
  `global_search_sort_test.go`, `global_search_ignore_test.go` – walker/index pipelines,
  async batching, sort consistency, gitignore handling.
- `asm_benchmark_test.go` – arm64 microbenchmarks for the assembly helpers (runs only on arm64 hosts).

### `internal/ui`
- `render/renderer_test.go` and `render/footer_help_test.go` check header/footer formatting,
  help text, and text-measurement helpers.
- `input/handler_test.go` ensures keyboard events map to the right actions without a real screen.
- `pager/` runs on a real TTY (raw mode), so it’s manually verified via `→`, scroll keys, `w` (wrap),
  and `q`/`Esc` to exit; no automated harness yet.

---

## Adding or Updating Tests

1. **Pick the package** – keep tests next to the code they exercise (`internal/state`, `internal/search`, etc.).
2. **Build minimal state** – instantiate structs like `AppState` or `GlobalSearchResult`; avoid global fixtures.
3. **Invoke the reducer/function** – e.g., `reducer.Reduce(state, ToggleHiddenFilesAction{})`.
4. **Assert results** – use `t.Fatalf` for unexpected errors and `t.Errorf`/`cmp.Diff` for field checks.
5. **Name new files `_test.go`** – Go’s tooling will discover them automatically.

### Example: ToggleHiddenFiles

```go
func TestToggleHiddenFilesRestoresVisibleSelection(t *testing.T) {
	state := &AppState{
		Files: []FileEntry{
			{Name: ".git", IsDir: true},
			{Name: "src", IsDir: true},
		},
		SelectedIndex: 1,
		HideHiddenFiles: true,
	}

	reducer := NewStateReducer()
	if _, err := reducer.Reduce(state, ToggleHiddenFilesAction{}); err != nil {
		t.Fatalf("toggle failed: %v", err)
	}

	if got := state.DisplaySelectedIndex(); got != 1 {
		t.Errorf("expected cursor to stay on visible entry, got %d", got)
	}
}
```

Pattern: set up state → run the action → validate a handful of fields.

---

## Debugging Failures

```bash
go test ./internal/state -run TestNavigateDown -v
go test ./internal/search -run TestGlobalSearcher_Index -count=1
```

- Sprinkle `t.Logf`/`spew.Dump` for extra context.
- Pass `-count=1` when a test depends on time or filesystem state to bypass Go’s cache.
- For filesystem-heavy suites, point `TMPDIR` at `./temp` so leftovers stay inside the repo.

---

## Continuous Integration

Example pipeline snippet:

```bash
#!/usr/bin/env bash
set -euo pipefail

make fmt
make lint          # optional (requires golangci-lint)
make test
make test-coverage
```

Store `make test-coverage` output in CI logs. Locally, run `make bench-fuzzy` (and its
ASCII/DP32 variants) before shipping matcher or global-search changes.

---

## Summary

| Area                    | Coverage focus                                             |
|-------------------------|------------------------------------------------------------|
| `internal/state`        | Reducers: navigation, filter, hidden files, history, ESC   |
| `internal/state` (I/O)  | Previews, text/binary heuristics, real filesystem          |
| `internal/search`       | Fuzzy matcher + ASCII32/DP32, global search, gitignore     |
| `internal/ui`           | Renderer helpers, footer/help text, input handler mapping  |
| `make test`             | `go test ./internal/...` smoke                             |
| `make test-race`        | Required for concurrency / async search changes            |

Because reducers are pure functions, test cases stay short and deterministic. Before opening a PR, run at least `make fmt && make test`; add `make test-coverage` and the relevant benchmarks whenever you touch scoring or the global-search pipeline.
