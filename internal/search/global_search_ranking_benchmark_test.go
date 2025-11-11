package search

import (
	"sort"
	"testing"
	"unicode/utf8"
)

type rankingScenario struct {
	name          string
	query         string
	caseSensitive bool
	candidates    []string
}

var rankingScenarios = []rankingScenario{
	{
		name:  "tail_vs_middle_en",
		query: "en",
		candidates: []string{
			"repo/fs/hidden_windows.go",
			"repo/ui/render/renderer.go",
			"docs/content/en.md",
		},
	},
	{
		name:  "multi_token_tail_mo_sum",
		query: "mo sum",
		candidates: []string{
			"apps/core/modules/summary.html",
			"apps/core/modules/transport/templates/summary.html",
			"apps/momentum/sum.go",
			"apps/monitoring/sumcheck/main.go",
		},
	},
	{
		name:  "multi_token_tail_boost",
		query: "mo sum",
		candidates: []string{
			"apps/core/modules/transport/templates/sum_preview.html",
			"apps/core/modules/summary.html",
			"pkg/sum.go",
		},
	},
	{
		name:  "short_suffix_go",
		query: "go",
		candidates: []string{
			"app/handlers/generic/options",
			"app/render/layout.go",
			"app/render/renderer.go",
			"cmd/tool/main.go",
		},
	},
	{
		name:  "filename_exact_renderer",
		query: "renderer.go",
		candidates: []string{
			"pkg/ui/renderer.go",
			"pkg/ui/render/renderer.go",
			"pkg/renderer.go",
		},
	},
	{
		name:  "go_mod_files",
		query: "mod",
		candidates: []string{
			"experiments/device123/sdk/Libraries/Common/doc/html/modules.js",
			"experiments/device123/sdk/Libraries/Common/doc/latex/modules.tex",
			"repo/go.mod",
			"repo/go.sum",
		},
	},
	{
		name:  "mixed_case_sensitive",
		query: "FSM",
		candidates: []string{
			"src/finiteStateMachine/main.go",
			"src/FSM/main.go",
			"src/fsm/docs/FsmDesign.md",
		},
		caseSensitive: true,
	},
	{
		name:  "index_vs_walker",
		query: "index",
		candidates: []string{
			"repo/search/global_search_index.go",
			"repo/search/indexer/index_progress.go",
			"notes/index_summary.md",
			"pkg/index.go",
		},
	},
}

func BenchmarkGlobalSearchRankingScenarios(b *testing.B) {
	gs := &GlobalSearcher{matcher: NewFuzzyMatcher()}

	for _, sc := range rankingScenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			tokens, matchAll := prepareQueryTokens(sc.query, sc.caseSensitive)
			if matchAll {
				b.Skip("query matches everything")
			}

			for i := 0; i < b.N; i++ {
				results := runRankingScenario(gs, sc, tokens)
				if len(results) == 0 {
					b.Fatalf("scenario %s produced no matches", sc.name)
				}
			}
		})
	}
}

func TestGlobalSearchRankingScenariosSnapshot(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("enable with go test -run Snapshot -v to inspect rankings")
	}
	gs := &GlobalSearcher{matcher: NewFuzzyMatcher()}
	for _, sc := range rankingScenarios {
		tokens, matchAll := prepareQueryTokens(sc.query, sc.caseSensitive)
		if matchAll {
			t.Logf("%s: query matches everything, skipping", sc.name)
			continue
		}
		results := runRankingScenario(gs, sc, tokens)
		t.Logf("Scenario %s (query %q)", sc.name, sc.query)
		for idx, res := range results {
			t.Logf("  %d. %-50s score=%6.3f span=%d-%d", idx+1, res.FilePath, res.Score, res.MatchStart, res.MatchEnd)
		}
	}
}

func runRankingScenario(gs *GlobalSearcher, sc rankingScenario, tokens []queryToken) []GlobalSearchResult {
	results := make([]GlobalSearchResult, 0, len(sc.candidates))
	for idx, path := range sc.candidates {
		score, matched, details := gs.matchTokens(tokens, path, sc.caseSensitive, false)
		if !matched {
			continue
		}

		score += computeSegmentBoost(sc.query, path, details)

		pathLength := details.TargetLength
		if pathLength == 0 {
			pathLength = utf8.RuneCountInString(path)
		}

		result := GlobalSearchResult{
			FilePath:     path,
			Score:        score,
			PathLength:   pathLength,
			MatchStart:   details.Start,
			MatchEnd:     details.End,
			MatchCount:   details.MatchCount,
			WordHits:     details.WordHits,
			PathSegments: countPathSegments(path),
			InputOrder:   idx,
			HasMatch:     sc.query != "",
			MatchSpans:   details.Spans,
		}
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return compareResults(results[i], results[j]) < 0
	})
	return results
}
