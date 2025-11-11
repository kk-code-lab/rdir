## Global Search Scoring Scenarios

This note captures the edge cases we are targeting while tuning fuzzy scoring
and provides a reproducible benchmark harness.

### How to run

```bash
go test -bench RankingScenarios ./internal/search -benchmem
# Inspect ordered results with verbose logs
go test ./internal/search -run Snapshot -v
```

### Scenario catalog

| Scenario | Query | Candidates | Intent |
|----------|-------|------------|--------|
| `tail_vs_middle_en` | `en` | `repo/fs/hidden_windows.go`, `repo/ui/render/renderer.go`, `docs/content/en.md` | Prefer matches that land in the file name (tail segment) over earlier directory hits. |
| `multi_token_tail_mo_sum` | `mo sum` | `apps/core/modules/summary.html`, `apps/core/modules/transport/templates/summary.html`, `apps/momentum/sum.go`, `apps/monitoring/sumcheck/main.go` | With multi-token queries make sure the final segment containing both tokens outranks deeper directory matches. |
| `short_suffix_go` | `go` | `app/render/layout.go`, `app/render/renderer.go`, `cmd/tool/main.go`, `app/handlers/generic/options` | A short token should still push `.go` filenames above directories that merely contain the substring. |
| `filename_exact_renderer` | `renderer.go` | `pkg/renderer.go`, `pkg/ui/render/renderer.go`, `pkg/ui/renderer.go` | Exact filename queries should prioritize an exact tail match, even if directories also include the same substring. |
| `go_mod_files` | `mod` | `repo/go.mod`, `repo/go.sum`, `…/doc/html/modules.js`, `…/modules.tex` | Prefer `go.mod`/`go.sum` over long directory matches when searching for “mod”. |
| `mixed_case_sensitive` | `FSM` (case-sensitive) | `src/FSM/main.go`, `src/finiteStateMachine/main.go`, `src/fsm/docs/FsmDesign.md` | When case-sensitive search is on, an exact case match should outrank case-insensitive hits. |
| `index_vs_walker` | `index` | `repo/search/global_search_index.go`, `pkg/index.go`, `notes/index_summary.md`, `repo/search/indexer/index_progress.go` | Ensure that both index-related files and generic `index.go` files sort sensibly. |

These strings intentionally point at representative paths, but they do not
require the files to exist—`internal/search/global_search_ranking_benchmark_test.go`
replays them directly against the matcher.

As we add more heuristics (final segment boosts, substring bonuses, etc.) we
can extend this table and the shared benchmark to capture the behavior we care
about.
