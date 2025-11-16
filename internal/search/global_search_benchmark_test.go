package search

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type benchRepoLayout struct {
	Levels      []int
	FilesPerDir int
	FilePrefix  string
	FileSuffix  string
}

func BenchmarkGlobalSearcherWalk(b *testing.B) {
	b.ReportAllocs()
	layout := benchRepoLayout{
		Levels:      []int{24, 6},
		FilesPerDir: 16,
		FilePrefix:  "file",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	addIgnoredFixture(b, root)
	createBenchmarkSentinelFiles(b, root)

	searcher := NewGlobalSearcher(root, false, nil)

	b.Run("ManyMatches", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.SearchRecursive("file", false)
			if len(results) == 0 {
				b.Fatalf("expected matches for pattern")
			}
		}
	})

	b.Run("SingleMatch", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.SearchRecursive("unique_target", false)
			if len(results) != 1 {
				b.Fatalf("expected exactly one result, got %d", len(results))
			}
		}
	})

	b.Run("ManyMatchesAND", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.SearchRecursive("file dir", false)
			if len(results) == 0 {
				b.Fatalf("expected AND query to return matches")
			}
		}
	})

	b.Run("SingleMatchAND", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.SearchRecursive("unique target", false)
			if len(results) != 1 {
				b.Fatalf("expected AND query to return single result, got %d", len(results))
			}
		}
	})
}

func BenchmarkFuzzyGlobalSearcherANDWalk(b *testing.B) {
	b.ReportAllocs()
	layout := benchRepoLayout{
		Levels:      []int{12, 4},
		FilesPerDir: 16,
		FilePrefix:  "file",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	addIgnoredFixture(b, root)
	createBenchmarkSentinelFiles(b, root)

	searcher := NewGlobalSearcher(root, false, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := searcher.SearchRecursive("file dir", false)
		if len(results) == 0 {
			b.Fatalf("expected AND walk query to return matches")
		}
	}
}

func BenchmarkGlobalSearcherIndexBuild(b *testing.B) {
	b.ReportAllocs()
	layout := benchRepoLayout{
		Levels:      []int{20, 8},
		FilesPerDir: 18,
		FilePrefix:  "file",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	createBenchmarkSentinelFiles(b, root)

	b.Setenv(envMaxIndexResults, strconv.Itoa(1_000_000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		searcher := NewGlobalSearcher(root, false, nil)
		_ = searcher.SearchRecursive("no_match_pattern", false)
		waitForIndexReadyTB(b, searcher)
	}
}

func BenchmarkFuzzyGlobalSearcherANDIndex(b *testing.B) {
	b.ReportAllocs()
	layout := benchRepoLayout{
		Levels:      []int{20, 6},
		FilesPerDir: 16,
		FilePrefix:  "file",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	createBenchmarkSentinelFiles(b, root)

	b.Setenv(envMaxIndexResults, strconv.Itoa(1_000_000))

	searcher := NewGlobalSearcher(root, false, nil)
	_ = searcher.SearchRecursive("warmup", false)
	waitForIndexReadyTB(b, searcher)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := searcher.searchIndex("file dir", false)
		if len(results) == 0 {
			b.Fatalf("expected AND index query to return matches")
		}
	}
}

func BenchmarkGlobalSearcherIndexQuery(b *testing.B) {
	b.ReportAllocs()
	layout := benchRepoLayout{
		Levels:      []int{32, 8},
		FilesPerDir: 16,
		FilePrefix:  "file",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	createBenchmarkSentinelFiles(b, root)

	b.Setenv(envMaxIndexResults, strconv.Itoa(1_000_000))

	searcher := NewGlobalSearcher(root, false, nil)
	_ = searcher.SearchRecursive("warmup", false)
	waitForIndexReadyTB(b, searcher)

	b.Run("ManyMatches", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.searchIndex("file", false)
			if len(results) == 0 {
				b.Fatalf("expected matches")
			}
		}
	})

	b.Run("SingleMatch", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.searchIndex("unique_target", false)
			if len(results) != 1 {
				b.Fatalf("expected exactly one indexed match, got %d", len(results))
			}
		}
	})

	b.Run("ManyMatchesAND", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.searchIndex("file dir", false)
			if len(results) == 0 {
				b.Fatalf("expected AND index query to return matches")
			}
		}
	})

	b.Run("SingleMatchAND", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			results := searcher.searchIndex("unique target", false)
			if len(results) != 1 {
				b.Fatalf("expected AND index query to return single result, got %d", len(results))
			}
		}
	})
}

func BenchmarkIndexCandidatesAND(b *testing.B) {
	b.ReportAllocs()

	root := b.TempDir()
	onlyFoo := 100000
	onlyBar := 100000
	withBoth := 5000

	for i := 0; i < onlyFoo; i++ {
		writeBenchFile(b, root, fmt.Sprintf("foo_%d.txt", i))
	}
	for i := 0; i < onlyBar; i++ {
		writeBenchFile(b, root, fmt.Sprintf("bar_%d.txt", i))
	}
	for i := 0; i < withBoth; i++ {
		writeBenchFile(b, root, fmt.Sprintf("foo_bar_%d.txt", i))
	}

	b.Setenv(envMaxIndexResults, strconv.Itoa(1_000_000))
	b.Setenv(envIndexMaxWorkers, "2") // keep indexing predictable for benchmarks

	searcher := NewGlobalSearcher(root, false, nil)
	_ = searcher.SearchRecursive("warmup", false)
	waitForIndexReadyTB(b, searcher)

	entries := searcher.snapshotEntries(0, -1)
	tokens, _ := prepareQueryTokens("foo bar", false)
	searcher.orderTokens(tokens)

	requiredBits := runeBitset{}
	for _, token := range tokens {
		for _, r := range token.runes {
			if idx := runeBitIndex(r); idx >= 0 {
				requiredBits.set(idx)
			}
		}
	}

	b.Run("BitsetBuckets", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			candidates := searcher.indexCandidates(tokens, entries)
			if len(candidates) == 0 {
				b.Fatalf("expected candidates")
			}
			releaseCandidateBuffer(candidates)
		}
	})

	b.Run("SequentialFallback", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			count := 0
			for idx := range entries {
				if entries[idx].runeBits.contains(requiredBits) {
					count++
				}
			}
			if count == 0 {
				b.Fatalf("expected sequential filtering to find matches")
			}
		}
	})
}

func BenchmarkGlobalSearcherAsyncWalk(b *testing.B) {
	b.ReportAllocs()
	layout := benchRepoLayout{
		Levels:      []int{16, 6},
		FilesPerDir: 16,
		FilePrefix:  "file",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	createBenchmarkSentinelFiles(b, root)

	searcher := NewGlobalSearcher(root, false, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		var once sync.Once
		searcher.SearchRecursiveAsync("file", false, func(results []GlobalSearchResult, isDone bool, inProgress bool) {
			if !isDone {
				return
			}
			if len(results) == 0 {
				b.Fatalf("expected matches in async results")
			}
			once.Do(func() {
				close(done)
			})
		})
		<-done
	}
}

func BenchmarkIgnoreProviderMatcherFor(b *testing.B) {
	layout := benchRepoLayout{
		Levels:      []int{1},
		FilesPerDir: 0,
		FilePrefix:  "ignored",
		FileSuffix:  ".txt",
	}

	root := createBenchmarkRepo(b, layout)
	depth := 32
	keys := make([]string, 0, depth)

	parent := root
	for i := 0; i < depth; i++ {
		parent = filepath.Join(parent, fmt.Sprintf("level_%02d", i))
		if err := os.MkdirAll(parent, 0o755); err != nil {
			b.Fatalf("mkdir %s: %v", parent, err)
		}
		pattern := fmt.Sprintf("ignored_level_%02d/\n!keep_level_%02d.txt\n", i, i)
		if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(pattern), 0o644); err != nil {
			b.Fatalf("write gitignore: %v", err)
		}
		if err := os.WriteFile(filepath.Join(parent, fmt.Sprintf("keep_level_%02d.txt", i)), []byte("keep"), 0o644); err != nil {
			b.Fatalf("write keep: %v", err)
		}
		rel, err := filepath.Rel(root, parent)
		if err != nil {
			b.Fatalf("rel path: %v", err)
		}
		keys = append(keys, rel)
	}

	b.Run("Cold", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			provider := newIgnoreProvider(root)
			for _, key := range keys {
				provider.MatcherFor(key)
			}
		}
	})

	b.Run("Warm", func(b *testing.B) {
		b.ReportAllocs()
		provider := newIgnoreProvider(root)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, key := range keys {
				provider.MatcherFor(key)
			}
		}
	})
}

func BenchmarkGitignoreMatcherMatch(b *testing.B) {
	b.ReportAllocs()
	matcher := NewGitignoreMatcher()

	var builder strings.Builder
	for i := 0; i < 500; i++ {
		builder.WriteString(fmt.Sprintf("*.log\nbuild_output_%d/\n!keep_%d.log\n", i, i))
	}
	matcher.AddPatterns(builder.String(), ".")

	paths := make([]struct {
		path  string
		isDir bool
	}, 0, 512)

	for i := 0; i < 256; i++ {
		paths = append(paths, struct {
			path  string
			isDir bool
		}{
			path:  fmt.Sprintf("dir_%d/build_output_%d/artifact.bin", i, i),
			isDir: false,
		})
		paths = append(paths, struct {
			path  string
			isDir bool
		}{
			path:  fmt.Sprintf("logs/service_%d.log", i),
			isDir: false,
		})
		paths = append(paths, struct {
			path  string
			isDir bool
		}{
			path:  fmt.Sprintf("logs/keep_%d.log", i),
			isDir: false,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for _, item := range paths {
			if matcher.MatchWithType(item.path, item.isDir) {
				count++
			}
		}
		if count == 0 {
			b.Fatalf("expected some matches in gitignore benchmark")
		}
	}
}

func createBenchmarkRepo(tb testing.TB, layout benchRepoLayout) string {
	tb.Helper()

	root := tb.TempDir()
	prefix := layout.FilePrefix
	if prefix == "" {
		prefix = "file"
	}
	suffix := layout.FileSuffix
	if suffix == "" {
		suffix = ".txt"
	}

	writeFiles := func(dir string, depth int) {
		for i := 0; i < layout.FilesPerDir; i++ {
			name := fmt.Sprintf("%s_d%d_f%03d%s", prefix, depth, i, suffix)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644); err != nil {
				tb.Fatalf("write file %s: %v", name, err)
			}
		}
	}

	var create func(dir string, depth int)
	create = func(dir string, depth int) {
		writeFiles(dir, depth)
		if depth >= len(layout.Levels) {
			return
		}
		count := layout.Levels[depth]
		for i := 0; i < count; i++ {
			child := filepath.Join(dir, fmt.Sprintf("dir_d%d_%03d", depth, i))
			if err := os.MkdirAll(child, 0o755); err != nil {
				tb.Fatalf("mkdir %s: %v", child, err)
			}
			create(child, depth+1)
		}
	}

	create(root, 0)
	return root
}

func addIgnoredFixture(tb testing.TB, root string) {
	tb.Helper()

	ignoredDir := filepath.Join(root, "ignored_dir")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		tb.Fatalf("mkdir ignored dir: %v", err)
	}
	for i := 0; i < 128; i++ {
		name := fmt.Sprintf("ignored_%03d.tmp", i)
		if err := os.WriteFile(filepath.Join(ignoredDir, name), []byte("ignored"), 0o644); err != nil {
			tb.Fatalf("write ignored file: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored_dir/\n"), 0o644); err != nil {
		tb.Fatalf("write .gitignore: %v", err)
	}
}

func createBenchmarkSentinelFiles(tb testing.TB, root string) {
	tb.Helper()

	if err := os.WriteFile(filepath.Join(root, "unique_target.txt"), []byte("unique"), 0o644); err != nil {
		tb.Fatalf("write unique target: %v", err)
	}

	deepDir := filepath.Join(root, "dir_d0_000")
	if err := os.MkdirAll(deepDir, 0o755); err != nil {
		tb.Fatalf("mkdir deep dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deepDir, "needle_target.txt"), []byte("needle"), 0o644); err != nil {
		tb.Fatalf("write needle: %v", err)
	}
}

func waitForIndexReadyTB(tb testing.TB, searcher *GlobalSearcher) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		ready, count, useIndex := searcher.indexSnapshot()
		if ready && useIndex && count > 0 {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("index not ready in time: ready=%v useIndex=%v count=%d", ready, useIndex, count)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func writeBenchFile(tb testing.TB, root, name string) {
	tb.Helper()
	full := filepath.Join(root, name)
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		tb.Fatalf("write bench file %s: %v", name, err)
	}
}
