# Repository Guidelines

## Project Structure & Module Organization
- `cmd/rdir/main.go` holds the CLI entrypoint and wires the TUI loop.
- `internal/` contains the core reducers, fuzzy search, and platform glue; unit and integration tests live beside their packages (see `internal/state/fuzzy_integration_test.go`).
- `build/rdir` is the generated binary; it stays out of version control via `.gitignore`.
- `temp/` is gitignored scratch space for temporary scripts, data, or experiments.
- `docs/` provides deeper reference material such as `docs/TEST_GUIDE.md` and `docs/IMPLEMENTATION.md`.
- `scripts/make.ps1` captures the supported workflows for Windows/macOS/Linux (run `./scripts/make.ps1 <target>`). A GNU Makefile is kept for compatibility but is not the primary entrypoint.

## Build, Test & Development Commands
- Use `./scripts/make.ps1 <target>` for builds/tests/benchmarks; targets mirror the Makefile for users who still invoke `make`.
- `./scripts/make.ps1 build` compiles the binary to `build/rdir`; `./scripts/make.ps1 run` rebuilds and launches it locally.
- `./scripts/make.ps1 test` runs the full suite in `./internal/...`; use `test-coverage` and `test-race` targets for coverage and race checks.
- `./scripts/make.ps1 check` runs lint, builds the binary, and compiles tests without executing them (`go test -run '^$' ./internal/...`).
- `./scripts/make.ps1 bench-fuzzy` benchmarks the fuzzy matchers; run it before and after algorithm changes to confirm deltas.
- `./scripts/make.ps1 fmt` applies `go fmt ./...`; `./scripts/make.ps1 lint` shells to `golangci-lint run ./...` (install it locally first).

## Coding Style & Naming Conventions
- Follow idiomatic Go: tabs for indentation, `camelCase` for locals, `PascalCase` for exported identifiers, and `_test.go` suffixes for test files.
- New packages should sit under `internal/` and expose only the minimum public surface; name directories after their domain (`internal/global_search`, etc.) when splitting grows necessary.
- Run `make fmt` (and optionally `golangci-lint run ./...`) before sending changes to keep imports and formatting consistent.

## Testing Guidelines
- Prefer table-driven tests and subtests as shown in `internal/state/reducer_test.go`; mirror production file names to keep coverage obvious.
- Integration flows belong in `*_integration_test.go`; gate expensive cases with short timeouts so they pass on CI.
- Keep existing coverage steady; if coverage dips, add focused tests or reroute logic through existing suites.

## Commit & Pull Request Guidelines
- Write imperative, scope-first commit messages (e.g., `Speed up gitignore matching`, `Reuse async walk buffers`); keep the summary under 72 characters.
- Each PR should include: a plain-language summary, any relevant `docs/` updates, links to tracking issues, and output snippets for `make test` (and `make lint` when applicable).
- Before requesting review, ensure the working tree is clean, binaries are ignored, and benchmarks or race tests have been run for performance-sensitive changes.
