# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**rdir** is a terminal-based file manager inspired by macOS Finder, written in Go using the tcell library for terminal UI. It features a Redux-like architecture with centralized state management, fuzzy search capabilities, global search with gitignore support, built-in pager, and comprehensive test coverage.

- **Language**: Go 1.25.1
- **Primary dependency**: tcell/v2 (terminal UI)
- **Architecture**: Redux-like with pure reducers, centralized state, unidirectional data flow
- **Code size**: ~22,000 lines (excluding tests), ~19,000 lines of tests
- **Test coverage**: 518 tests across 55 test files, 100% pass rate

## Build & Test Commands

The project includes a **Makefile** for convenient task execution:

```bash
make build          # Build binary to build/rdir
make test           # Run all tests with verbose output
make test-coverage  # Run tests with coverage report
make test-race      # Run tests with race detector
make bench-fuzzy    # Run fuzzy matching benchmarks
make run            # Build and run the application
make clean          # Remove build artifacts
make fmt            # Format code with go fmt
make lint           # Run linter (requires golangci-lint)
make help           # Show available commands
```

### Direct `go` commands (alternative):

```bash
go build -o build/rdir ./cmd/rdir    # Build binary
go test ./internal/...               # Run all tests
go test ./internal/... -v -run Name  # Run specific test
go test ./internal/... -cover        # Coverage report
go test ./internal/... -race         # Race detector
go test ./internal/search -bench .   # Run benchmarks
```

## Code Architecture

### Unidirectional Data Flow

```
Terminal Events → InputHandler → Actions → StateReducer → AppState → Renderer → Screen
```

### Package Structure

```
rdir/
├── cmd/rdir/                    # Entry point (main.go)
├── internal/
│   ├── app/                     # Application lifecycle, event loop
│   │   ├── application.go       # App struct and initialization
│   │   ├── loop.go              # Main event loop (696 lines)
│   │   ├── actions.go           # App-level actions
│   │   └── platform.go          # Platform-specific code
│   ├── fs/                      # Filesystem utilities
│   │   ├── text.go              # Text detection heuristics
│   │   ├── entry.go             # FileEntry type
│   │   └── hidden_*.go          # Platform-specific hidden file detection
│   ├── search/                  # Search functionality
│   │   ├── fuzzy.go             # Fuzzy matching engine (1,227 lines)
│   │   ├── fuzzy_dp_ascii32.go  # SIMD-optimized matching
│   │   ├── global_search*.go    # Recursive indexed search
│   │   ├── gitignore.go         # Gitignore parsing and matching
│   │   └── ignore_provider.go   # Ignore file handling
│   ├── state/                   # State management
│   │   ├── state.go             # AppState struct
│   │   ├── reducer.go           # Main reducer (2,226 lines)
│   │   ├── actions.go           # Action definitions
│   │   ├── state_*.go           # State helpers (navigation, display, filter, etc.)
│   │   ├── preview_*.go         # Preview generation and formatting
│   │   └── markdown_*.go        # Markdown parsing and rendering
│   ├── ui/
│   │   ├── input/handler.go     # Event → Action conversion
│   │   ├── render/              # UI rendering
│   │   │   ├── renderer.go      # Main renderer (1,047 lines)
│   │   │   ├── preview.go       # Preview panel rendering
│   │   │   └── layout.go        # Layout calculations
│   │   └── pager/               # Built-in pager
│   │       ├── pager.go         # Pager implementation (3,672 lines)
│   │       └── search.go        # Pager search functionality
│   ├── shellsetup/              # Shell integration (--setup flag)
│   └── textutil/                # Text utilities (sanitize, tabs)
├── docs/                        # Documentation
└── build/                       # Compiled binaries
```

### Key Components

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `app` | Event loop, application lifecycle | `loop.go`, `application.go` |
| `state` | State management, reducers | `reducer.go`, `state.go` |
| `search` | Fuzzy matching, global search | `fuzzy.go`, `global_search*.go`, `gitignore.go` |
| `ui/render` | Terminal rendering | `renderer.go`, `preview.go` |
| `ui/pager` | Built-in file viewer | `pager.go`, `search.go` |
| `ui/input` | Input handling | `handler.go` |
| `fs` | Filesystem operations | `text.go`, `entry.go` |

## Feature Set

### Navigation
- **↑/↓**: Navigate file list
- **Enter**: Enter directory
- **→**: Open file in built-in pager or enter directory
- **←/Backspace**: Go to parent directory
- **[/]**: Navigate back/forward in history (with cursor restoration)
- **Page Up/Down**: Scroll viewport

### Local Fuzzy Filter
- **/**: Enter fuzzy filter mode (filters current directory)
- Type characters: Case-insensitive pattern matching
- Results sorted by match quality
- **Esc**: Clear filter and exit filter mode

### Global Search
- **Ctrl+P**: Enter global search mode (searches recursively)
- Searches across entire directory tree
- Respects `.gitignore` patterns
- Indexed for performance
- **Esc**: Exit global search

### Built-in Pager
- **→**: Open selected file in pager (text files only)
- **/** in pager: Search within file
- **n/N**: Next/previous search result
- **q**: Exit pager
- Supports large files, UTF-16, syntax highlighting for markdown

### Preview Panel
Displays file content preview:
- **Text**: First 64 KB with syntax highlighting (Markdown, JSON)
- **Binary**: Hex dump with ASCII gutter (up to 1 KB)
- **Directories**: Lists first 10 items
- **Metadata**: Size, modified time, permissions

### Shell Integration
- **q**: Exit rdir and output current directory to stdout
- **`rdir --setup`**: Print shell function for cd-on-exit (bash/zsh/fish/pwsh)

## Testing Strategy

### Test Organization

Tests are organized by feature area:

**Reducer tests** (`internal/state/`):
- `reducer_navigation_test.go` - Navigation actions
- `reducer_filter_test.go` - Local fuzzy filter
- `reducer_global_search_test.go` - Global search functionality
- `reducer_hidden_files_test.go` - Hidden file handling
- `reducer_filter_hidden_test.go` - Filter + hidden interactions
- `reducer_state_test.go` - State helpers
- `reducer_history_simple_test.go` - History navigation
- `reducer_io_test.go` - Filesystem integration
- `reducer_symlink_test.go` - Symlink handling
- `reducer_preview_test.go` - Preview generation
- `reducer_refresh_test.go` - Directory refresh

**Search tests** (`internal/search/`):
- `fuzzy_test.go` - Fuzzy matching algorithm
- `fuzzy_benchmark_test.go` - Performance benchmarks
- `global_search_*_test.go` - Global search tests
- `global_search_ignore_test.go` - Gitignore integration

**UI tests** (`internal/ui/`):
- `input/handler_test.go` - Input handling
- `render/renderer_test.go` - Rendering
- `pager/pager_test.go` - Pager functionality

**Integration tests**:
- `fuzzy_integration_test.go` - End-to-end filter tests
- `test_esc_logic_test.go` - Esc key behavior
- `test_filter_cursor_restoration_test.go` - Cursor restoration

### Running Tests

```bash
# All tests
go test ./internal/...

# Specific package
go test ./internal/state -v
go test ./internal/search -v
go test ./internal/ui/pager -v

# Pattern matching
go test ./internal/... -v -run Navigate
go test ./internal/... -v -run Filter
go test ./internal/... -v -run GlobalSearch
go test ./internal/... -v -run Pager

# Benchmarks
go test ./internal/search -bench . -benchmem
```

## Key Design Patterns

### AppState (Single Source of Truth)
All mutable data lives in `AppState` struct. UI reads state, never modifies directly.

### Pure Reducer
`Reduce(state, action)` functions have no side effects. Same input → same output.

### Selection History
`selectionHistory map[string]int` tracks cursor position per directory for seamless navigation.

### Gitignore Support
`internal/search/gitignore.go` implements full gitignore pattern matching for global search.

### Preview Formatting
Extensible formatter system in `internal/state/preview_formatter_*.go`:
- Text, Markdown, JSON, Binary formats
- Markdown AST parsing and styled rendering

## Common Development Tasks

### Adding a New Action
1. Define in `internal/state/actions.go` or `internal/app/actions.go`
2. Handle in `reducer.go` or `loop.go`
3. Add key binding in `internal/ui/input/handler.go`
4. Write tests

### Adding a Preview Format
1. Create `internal/state/preview_formatter_<format>.go`
2. Register in `preview_formatter.go`
3. Add tests in `preview_formatter_<format>_test.go`

### Modifying Global Search
1. Core logic in `internal/search/global_search*.go`
2. State integration in `internal/state/state_global_search.go`
3. Tests in `internal/search/global_search_*_test.go`

## Performance Notes

Fuzzy matching is highly optimized:
- SIMD acceleration on ARM64 (`fuzzy_simd_arm64.go`)
- ASCII32 DP optimization (`fuzzy_dp_ascii32.go`)
- Benchmarks available in `fuzzy_benchmark_test.go`

## Documentation

All documentation is in `docs/`:
- **IMPLEMENTATION.md** - Architecture and feature details
- **TEST_GUIDE.md** - Testing philosophy
- **FUZZY_SEARCH.md** - Fuzzy algorithm explanation
- **OPTIMIZATIONS.md** - Performance optimization notes
- **ASCII_DP32_REPORT.md** - SIMD optimization report
- **FUZZY_OPT_PROGRESS.md** - Optimization progress
- **SCORING_SCENARIOS.md** - Scoring test scenarios

## Debugging Tips

- All state logic testable without UI
- Use `t.Logf()` in tests to inspect state
- `Reduce()` is pure - deterministic behavior
- Use `-race` flag for concurrency issues
- Pager has extensive test coverage for edge cases
