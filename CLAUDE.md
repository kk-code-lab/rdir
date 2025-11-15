# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**rdir** is a terminal-based file manager inspired by macOS Finder, written in Go using the tcell library for terminal UI. It features a Redux-like architecture with centralized state management, fuzzy search capabilities, and comprehensive test coverage.

- **Language**: Go 1.25.1
- **Primary dependency**: tcell/v2 (terminal UI)
- **Architecture**: Redux-like with pure reducers, centralized state, unidirectional data flow
- **Code size**: ~1900 lines (excluding tests), ~2300 lines of tests
- **Test coverage**: 82 tests across 13 test files, 100% pass rate

## Build & Test Commands

The project includes a **Makefile** for convenient task execution. All commands below can be run with `make`:

```bash
# Build the binary to build/rdir
make build

# Run all tests with verbose output
make test

# Run tests with coverage report
make test-coverage

# Run tests with race detector (Go concurrency safety)
make test-race

# Run fuzzy matching benchmarks with memory stats
make bench-fuzzy

# Build and run the application
make run

# Remove build artifacts
make clean

# Format code with go fmt
make fmt

# Run linter (requires golangci-lint)
make lint

# Show available commands
make help
```

### Direct `go` commands (alternative):

```bash
# Build the binary to build/rdir
go build -o build/rdir ./cmd/rdir

# Run all tests
go test ./internal

# Run specific test
go test ./internal -v -run TestName

# Run with coverage report
go test ./internal -cover

# Run fuzzy matching benchmarks
go test ./internal -bench Fuzzy

# Run tests with race detector (Go concurrency safety)
go test ./internal/... -race
```

## Code Architecture

### Unidirectional Data Flow

```
Terminal Events → InputHandler → Actions → StateReducer → AppState → Renderer → Screen
```

The application follows a strict Redux-like pattern:
1. **InputHandler** (`internal/ui/input/handler.go`) converts tcell events to Action structs
2. **StateReducer** (`internal/state/reducer.go`) applies actions to state (pure functions)
3. **AppState** (`internal/state/state.go`) is the single source of truth
4. **Renderer** (`internal/ui/render/renderer.go`) reads state and renders display (no mutations)
5. **Application** (`internal/app/application.go`) orchestrates the event loop
6. **cmd/rdir/main.go** is the entry point

### Core Components

| File | Purpose | Lines | Key Types |
|------|---------|-------|-----------|
| `internal/state/state.go` | Centralized state container | 210 | AppState, FileEntry, PreviewData |
| `internal/state/reducer.go` | Pure state machine logic | 390 | StateReducer, Action interfaces |
| `internal/state/actions.go` | Action type definitions | 45 | All action structs including RightArrowAction |
| `internal/state/reducer_test.go` | Logic regression suite | 440 | Navigation/filter tests |
| `internal/search/fuzzy.go` | Fuzzy matching engine | 140 | FuzzyMatcher, match scoring |
| `internal/search/global_search.go` | Recursive/indexed search | 500+ | GlobalSearcher, IndexTelemetry |
| `internal/ui/input/handler.go` | Event → Action conversion | 65 | InputHandler, event processing |
| `internal/ui/render/renderer.go` | Display rendering | 460 | Renderer, tcell drawing |
| `internal/app/application.go` | Application lifecycle & pager | 390 | Application, event loop, pager opening |
| `cmd/rdir/main.go` | Entry point | 20 | main() function |

### Key Design Patterns

1. **AppState Struct**: Single centralized state with all mutable data
2. **Action Interface**: Type-safe action dispatch via Go interfaces
3. **Pure Reducer**: `Reduce(state, action)` functions with no side effects
4. **Selection History**: Map tracks cursor position per directory for seamless navigation
5. **Fuzzy Matching**: Pattern-based scoring (similar to fzf/Sublime Text)

## Feature Set

### Navigation
- **↑/↓**: Navigate file list
- **Enter**: Enter directory
- **→**: Open file in pager (text files only) or enter directory
- **←/Backspace**: Go to parent directory
- **[/]**: Navigate back/forward in history (with cursor restoration)

### Search/Filter
- **/**: Enter fuzzy search mode
- Type characters: Case-insensitive pattern matching (characters in order)
- **↑/↓**: Navigate filtered results (sorted by match quality)
- **Esc**: Clear filter and exit search mode

### Display
- Sidebar: Breadcrumb navigation path
- Main panel: File list with directories first, then alphabetically
- Preview panel: File metadata and content preview
- Scrolling: Page Up/Down for viewport management

### File Preview
- **→ (Right Arrow)**: Open file in pager for full-screen viewing
  - Text files: Opens in `$PAGER` environment variable (defaults to `less`)
  - Binary files: Ignored (no action)
  - Directories: Enters the directory (same as Enter key)
  - Terminal is properly restored after pager closes
  - Supports pager commands with flags (e.g., `less -R` for colored output)
  - **Windows**: Uses `more.com` or `cmd /c type` if `$PAGER` not set
  - **macOS/Linux**: Uses `/dev/tty` when available, with fallback support

### Exit and Directory Change
- **q**: Exit rdir and output current directory to stdout
- **Shell Setup**: Run `rdir --setup` to print the appropriate shell function/alias (bash/zsh/fish/pwsh/cmd) that captures rdir's stdout and `cd`s automatically. You can also force a specific shell, e.g. `rdir --setup=fish`.

### Fuzzy Matching Scoring

The algorithm scores matches based on:
- Base character bonus (1.0 per match)
- Word boundary bonus (1.5 for matches at word starts)
- Consecutive bonus (2.0 for consecutive characters)
- Spannedness metric (how compact the match span is)
- Gap penalties (distance between matched characters)
- Trailing penalties (minimal, only for very long files)

Normalized display: `score / 6.0 * 100` for percentage shown in preview.

## Testing Strategy

### Test Organization

Reducer tests (Pure logic, no filesystem):
1. **reducer_navigation_test.go** (13 tests): NavigateDown, NavigateUp, Scroll operations
2. **reducer_filter_test.go** (8 tests): Filter activation, typing, backspace, clearing
3. **reducer_hidden_files_test.go** (16 tests): ToggleHiddenFiles and cursor positioning
4. **reducer_filter_hidden_test.go** (4 tests): Filter + Hidden file interactions
5. **reducer_state_test.go** (7 tests): State helpers (getDisplayFiles, sortFiles, scrolling)
6. **reducer_history_simple_test.go** (1 test): History navigation

I/O and Integration tests:
7. **reducer_io_test.go**: Filesystem integration
   - Directory loading (valid/invalid paths)
   - Preview generation (text, binary, directories)
   - Text detection heuristics
   - Full navigation flows with real files

8. **fuzzy_test.go**: Fuzzy matching algorithm tests
   - Single file matching with scoring
   - Batch matching and sorting
   - Edge cases (empty patterns, no matches)
   - Performance characteristics

9. **fuzzy_integration_test.go**: Fuzzy search end-to-end
   - Filter mode activation/deactivation
   - Incremental filtering
   - Result ordering and cursor management

10. **test_esc_logic_test.go**: Esc key behavior in filter mode
11. **test_filter_cursor_restoration_test.go**: Cursor restoration after filtering
12. **reducer_symlink_test.go**: Symlink handling and directory navigation
13. **app_right_arrow_test.go** (3 tests): Pager functionality
    - Right arrow behavior on directories vs files
    - Text file detection heuristics
    - Pager interaction

Total: 82 tests covering all reducer and app functionality with 100% pass rate

### Test Pattern

All tests follow this structure:
```go
func TestFeature(t *testing.T) {
    // SETUP: Create initial state
    state := &AppState{ /* ... */ }

    // EXECUTE: Apply action
    reducer := NewStateReducer()
    reducer.Reduce(state, SomeAction{})

    // VERIFY: Assert expected changes
    if state.SelectedIndex != expected {
        t.Errorf("got %d, want %d", state.SelectedIndex, expected)
    }
}
```

### Running Specific Tests

```bash
# Navigation tests only
go test ./internal -v -run Navigate

# Filter tests only
go test ./internal -v -run Filter

# Hidden files tests only
go test ./internal -v -run ToggleHiddenFiles

# Filter+Hidden combination tests
go test ./internal -v -run FilterPlusHidden

# I/O tests only
go test ./internal -v -run "LoadDirectory|GeneratePreview|IsTextFile"

# Fuzzy matching tests
go test ./internal -v -run Fuzzy

# Single specific test
go test ./internal -v -run TestNavigateDown
```

## Important Implementation Details

### Selection Memory Per Directory

The `selectionHistory` map in StateReducer stores cursor position for each directory:
```go
selectionHistory map[string]int // path → selectedIndex
```

When exiting a directory, the current selection is saved. When re-entering, it's restored. This ensures seamless navigation without losing cursor position.

### History Navigation

The `history` slice and `historyIndex` enable forward/backward navigation:
- `[` key: Go backward in history
- `]` key: Go forward in history
- Cursor position is restored per directory

### File Sorting

Files are **always** maintained in sorted state:
1. Directories first (sorted alphabetically)
2. Then files (sorted alphabetically)

Sorting happens in: `state.sortFiles()` called by state methods.

### Preview Generation

`reducer.generatePreview(state)` creates preview data:
- **For files**: First 15 lines of text (detects text vs binary)
- **For directories**: Lists first 10 items
- **For all**: Shows size, modified time, permissions

Binary detection uses heuristic: if >30% of first 512 bytes are non-printable, treat as binary.

### Viewport Management

The reducer maintains visibility of selected item:
```go
scrollOffset    // Position of top of viewport
selectedIndex   // Currently selected item (always in display range)
screenHeight    // Terminal height
```

`updateScrollVisibility()` ensures selected item is always visible by adjusting scroll offset.

## Common Development Tasks

### Adding a New Navigation Action

1. Define action in `internal/state/actions.go`: `type MyNavigateAction struct{}`
2. Implement handler in `StateReducer.Reduce()` in `internal/state/reducer.go`
3. Write test in `internal/state/reducer_navigation_test.go`
4. Map key binding in `internal/ui/input/handler.go`

### Adding a New Filter Feature

1. Add action to `internal/state/actions.go`
2. Implement in `internal/state/reducer.go` filter action handlers
3. Write tests in `internal/search/fuzzy_test.go`, `internal/state/reducer_filter_test.go`, or `internal/state/reducer_filter_hidden_test.go`
4. Update `internal/ui/input/handler.go` key mapping

### Adding a Preview Type

1. Update `PreviewData` struct in `internal/state/state.go` if needed (e.g., formatted lines, reasons for unavailability)
2. Extend `generatePreview()` in `internal/state/reducer.go`
3. Add rendering in `internal/ui/render/renderer.go`
4. Write tests in `internal/state/reducer_io_test.go` (and formatter-specific tests)

Current formats:
- Text: first 64 KB, tabs expanded, UTF-16 normalized.
- JSON/Markdown: prettified when the file is not truncated and <=32 KB; otherwise stays raw and records `FormattedUnavailableReason`.
- Binary: hex dump up to 1 KB plus ASCII gutter.

## Performance Notes

- **Single fuzzy match**: 24.86 ns/op (47M ops/sec)
- **Batch match (10 files)**: 373.3 ns/op (3M ops/sec)
- **Large directory (1000 files)**: 46.6 µs/op (25K ops/sec)
- **Test suite**: ~0.4 seconds total

The fuzzy matching algorithm is highly optimized and scales linearly with input.

## Key Files to Understand First

1. **internal/state/state.go**: Understand `AppState` structure first - it's the foundation
2. **internal/state/reducer.go**: Main business logic - see how actions transform state
3. **internal/app/application.go**: Understand the event loop and component initialization
4. **internal/state/reducer_navigation_test.go**: See how navigation features are tested
5. **internal/state/reducer_filter_hidden_test.go**: See filter + hidden file interaction tests
6. **internal/search/fuzzy.go**: Pattern matching algorithm details
7. **cmd/rdir/main.go**: Entry point that initializes the application

## Debugging Tips

- All state logic is testable without UI - write minimal test cases
- Use `t.Logf()` in tests to inspect state changes
- `Reduce()` is pure - same input always produces same output
- Renderer only reads state - no bugs from circular updates
- Use `-race` flag to detect concurrent access issues

## Unicode and Locale Support

The application includes proper Unicode support for:
- Polish characters (ąćęłńóśźż)
- Other UTF-8 characters and emojis
- Wide characters (CJK, etc.)

**Requirements:**
- Terminal configured for UTF-8 (`LANG=en_US.UTF-8` or `LANG=pl_PL.UTF-8`)
- TCellv2 encoding registration enabled in main.go
- Runewidth library for accurate character width calculation

**Note:** Character rendering depends on terminal settings and font support.

## Known Limitations

1. Read-only application (no write operations)
2. Some Unicode edge cases may not render perfectly (depends on terminal/font)

## Documentation Files

All documentation is in `docs/`:
- **IMPLEMENTATION.md**: Comprehensive architecture and feature documentation
- **TEST_GUIDE.md**: Testing philosophy and examples
- **FUZZY_SEARCH.md**: Detailed fuzzy matching algorithm explanation

## Project Structure

```
rdir/
├── Makefile                       # Build automation
├── cmd/rdir/
│   └── main.go                    # Entry point
├── internal/                       # All application code
│   ├── actions.go                 # Action type definitions
│   ├── state.go                   # State structures
│   ├── reducer.go                 # State mutation logic
│   ├── app.go                     # Application lifecycle
│   ├── renderer.go                # UI rendering
│   ├── input_handler.go           # Event processing
│   ├── fuzzy.go                   # Fuzzy matching algorithm
│   ├── *_test.go                  # 57 unit tests
│   └── test_*.go                  # Behavior tests
├── docs/                          # Documentation
│   ├── IMPLEMENTATION.md
│   ├── TEST_GUIDE.md
│   └── FUZZY_SEARCH.md
├── build/                         # Compiled binaries
│   └── rdir                       # Executable
├── go.mod
├── go.sum
├── .gitignore
└── CLAUDE.md                      # This file
