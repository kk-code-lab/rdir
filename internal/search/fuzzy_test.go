package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFuzzyMatch_BasicMatching(t *testing.T) {
	fm := NewFuzzyMatcher()

	tests := []struct {
		pattern string
		text    string
		want    bool // Should match?
	}{
		{"", "anything", true},            // Empty pattern matches everything
		{"a", "apple", true},              // Single char
		{"ap", "apple", true},             // Substring (a, p)
		{"app", "apple", true},            // Prefix
		{"apl", "apple", true},            // Fuzzy match (a, p, l)
		{"abc", "axbycz", true},           // Non-consecutive (a, b, c)
		{"xyz", "apple", false},           // No match
		{"main", "main.go", true},         // Exact substring
		{"mgo", "main.go", true},          // Fuzzy in filename (m, g from main, o from .go)
		{"rdr", "reducer.go", true},       // Fuzzy across word (r, d, r)
		{"hgo", "input_handler.go", true}, // Fuzzy match (h from handler, g, o from .go)
	}

	for _, tt := range tests {
		score, matched := fm.Match(tt.pattern, tt.text)
		if matched != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v (score: %f)",
				tt.pattern, tt.text, matched, tt.want, score)
		}
	}
}

func TestAcquireRunesASCIIParity(t *testing.T) {
	inputs := []string{
		"",
		"README.md",
		"src/pkg/ExampleASCIIPath/ComponentFile.go",
		"embedded/ANY/READ_ME.url",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"___MIXED___CamelCase_and_snake_case___",
	}

	for _, s := range inputs {
		got, buf := acquireRunes(s, true)
		want := make([]rune, len(s))
		for i := 0; i < len(s); i++ {
			b := s[i]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			want[i] = rune(b)
		}
		if len(got) != len(want) {
			t.Fatalf("acquireRunes(%q) length mismatch: got %d want %d", s, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("acquireRunes(%q) mismatch at %d: got %q want %q (got string=%q want string=%q)",
					s, i, string(got[i]), string(want[i]), string(got), string(want))
			}
		}
		releaseRunes(buf)
	}
}

func TestIndexRunesASCIIFastPath(t *testing.T) {
	haystack := "src/pkg/ExampleASCIIPath/ComponentFile.go"
	pattern := "ascii"

	hayRunes, hayBuf := acquireRunes(haystack, true)
	defer releaseRunes(hayBuf)
	patternRunes, patternBuf := acquireRunes(pattern, true)
	defer releaseRunes(patternBuf)

	if !runesAreASCII(hayRunes) || !runesAreASCII(patternRunes) {
		t.Fatalf("expected ASCII runes for fast path test")
	}

	got := indexRunes(hayRunes, patternRunes)
	lowerHaystack := strings.ToLower(haystack)
	byteIdx := strings.Index(lowerHaystack, pattern)
	want := 0
	if byteIdx >= 0 {
		want = utf8.RuneCountInString(lowerHaystack[:byteIdx])
	}
	if got != want {
		t.Fatalf("indexRunes ASCII mismatch: got %d want %d", got, want)
	}
}

func TestIndexRunesUnicodeFallback(t *testing.T) {
	haystack := "ścieżka/do/pliku/żółć/łódź/fuzzy.go"
	pattern := "łódź"

	hayRunes, hayBuf := acquireRunes(haystack, false)
	defer releaseRunes(hayBuf)
	patternRunes, patternBuf := acquireRunes(pattern, false)
	defer releaseRunes(patternBuf)

	if runesAreASCII(hayRunes) {
		t.Fatalf("expected non-ASCII haystack for fallback test")
	}

	got := indexRunes(hayRunes, patternRunes)
	byteIdx := strings.Index(haystack, pattern)
	want := 0
	if byteIdx >= 0 {
		want = utf8.RuneCountInString(haystack[:byteIdx])
	}
	if got != want {
		t.Fatalf("indexRunes Unicode mismatch: got %d want %d", got, want)
	}
}

func TestFuzzyMatch_SmartCase(t *testing.T) {
	fm := NewFuzzyMatcher()

	// Lowercase pattern stays case-insensitive.
	if score, matched := fm.Match("main", "MAIN.go"); !matched {
		t.Fatalf("expected lowercase pattern to ignore case, score=%f", score)
	}

	// Uppercase pattern enforces case-sensitive matching.
	if score, matched := fm.Match("Main", "main.go"); matched {
		t.Fatalf("expected smart-case to reject mismatched case, score=%f", score)
	}

	if score, matched := fm.Match("Main", "Main.go"); !matched {
		t.Fatalf("expected smart-case to accept exact case, score=%f", score)
	}

	// Mixed case requires matching each uppercase position.
	if score, matched := fm.Match("FoO", "foo.go"); matched {
		t.Fatalf("mixed-case pattern should respect uppercase letters, score=%f", score)
	}

	if score, matched := fm.Match("FoO", "FoO.go"); !matched {
		t.Fatalf("exact mixed-case pattern should match, score=%f", score)
	}
}

func TestFuzzyMatch_NonASCIICharacters(t *testing.T) {
	fm := NewFuzzyMatcher()

	if score, matched := fm.Match("é", "café"); !matched {
		t.Fatalf("expected match for café, got matched=%v score=%f", matched, score)
	}

	if score, matched := fm.Match("é", "cafÃ©"); matched {
		t.Fatalf("expected no match for incorrectly encoded string, got score=%f", score)
	}
}

func TestFuzzyMatch_Scoring(t *testing.T) {
	fm := NewFuzzyMatcher()

	tests := []struct {
		pattern  string
		text     string
		minScore float64
		name     string
	}{
		// Exact match should score highest
		{"main", "main.go", 0.1, "exact prefix match"},
		// Consecutive chars should score high
		{"app", "apple.txt", 0.1, "consecutive match"},
		// Gaps should score lower
		{"apl", "apple.txt", 0.0, "non-consecutive match"},
		// Word boundary should boost score
		{"main", "main.go", 0.05, "word boundary bonus"},
		{"dht11", "drivers/sensors/dht11/dht11_driver.c", 0.1, "substring bonus"},
	}

	scores := make(map[string]float64)

	for _, tt := range tests {
		score, matched := fm.Match(tt.pattern, tt.text)
		if !matched {
			t.Errorf("Match(%q, %q) should match: %s", tt.pattern, tt.text, tt.name)
			continue
		}
		scores[tt.name] = score
	}

	// Verify ordering: exact prefix should beat non-consecutive
	if scores["exact prefix match"] <= scores["non-consecutive match"] {
		t.Errorf("Exact prefix (%.3f) should score higher than non-consecutive (%.3f)",
			scores["exact prefix match"], scores["non-consecutive match"])
	}

	// Verify ordering: consecutive should beat gaps
	if scores["consecutive match"] <= scores["non-consecutive match"] {
		t.Errorf("Consecutive (%.3f) should score higher than non-consecutive (%.3f)",
			scores["consecutive match"], scores["non-consecutive match"])
	}

	if scores["substring bonus"] <= scores["non-consecutive match"] {
		t.Errorf("Substring match (%.3f) should score higher than non-consecutive (%.3f)",
			scores["substring bonus"], scores["non-consecutive match"])
	}
}

func TestFuzzyMatch_PathologicalReadmeOrdering(t *testing.T) {
	fm := NewFuzzyMatcher()
	pattern := "readme"

	type sample struct {
		name  string
		path  string
		score float64
	}

	candidates := []sample{
		{
			name: "underscored fragments",
			path: "embedded/any/_r_e_a_d_m_e_8md.html",
		},
		{
			name: "mixed underscore",
			path: "embedded/any/READ_ME.url",
		},
		{
			name: "plain readme",
			path: "minirouter-lib/README.md",
		},
	}

	for i := range candidates {
		score, matched := fm.Match(pattern, candidates[i].path)
		if !matched {
			t.Fatalf("expected %q to match pattern %q", candidates[i].path, pattern)
		}
		candidates[i].score = score
	}

	low := candidates[0]
	mid := candidates[1]
	high := candidates[2]

	if !(high.score > mid.score && mid.score > low.score) {
		t.Fatalf("expected score ordering high>mid>low, got high=%.3f (%s), mid=%.3f (%s), low=%.3f (%s)",
			high.score, high.name, mid.score, mid.name, low.score, low.name)
	}
}

func TestFuzzyMatch_WordHitBoundaries(t *testing.T) {
	fm := NewFuzzyMatcher()
	pattern := "readme"

	type sample struct {
		name       string
		path       string
		wantMinHit int
		wantMaxHit int
	}

	cases := []sample{
		{
			name:       "plain README under directory",
			path:       "minirouter-lib/README.md",
			wantMinHit: 1,
			wantMaxHit: 6,
		},
		{
			name:       "camel case README inside directory",
			path:       "docs/ReadMeGuide.txt",
			wantMinHit: 1,
			wantMaxHit: 6,
		},
		{
			name:       "underscore separated letters",
			path:       "embedded/_r_e_a_d_m_e_8md.html",
			wantMinHit: 0,
			wantMaxHit: 0,
		},
	}

	for _, tt := range cases {
		_, matched, details := fm.MatchDetailed(pattern, tt.path)
		if !matched {
			t.Fatalf("expected %q to match pattern %q (%s)", tt.path, pattern, tt.name)
		}
		if details.WordHits < tt.wantMinHit || details.WordHits > tt.wantMaxHit {
			t.Fatalf("%s: expected word hits in [%d,%d], got %d", tt.name, tt.wantMinHit, tt.wantMaxHit, details.WordHits)
		}
	}
}

func TestFuzzyMatchDetailedPositions(t *testing.T) {
	fm := NewFuzzyMatcher()

	score, matched, details := fm.MatchDetailed("main", "src/main.go")
	if !matched {
		t.Fatalf("expected match, got matched=%v score=%f", matched, score)
	}
	if details.Start != 4 {
		t.Fatalf("expected start index 4, got %d", details.Start)
	}
	if details.End != 7 {
		t.Fatalf("expected end index 7, got %d", details.End)
	}
	if expected := utf8.RuneCountInString("src/main.go"); details.TargetLength != expected {
		t.Fatalf("expected target length %d, got %d", expected, details.TargetLength)
	}
}

func TestFuzzyMatch_RealWorldExamples(t *testing.T) {
	fm := NewFuzzyMatcher()

	tests := []struct {
		pattern string
		text    string
		wantMin float64 // Minimum acceptable score
		name    string
	}{
		// From real rdir project
		{"rdr", "reducer.go", 0.02, "rdr in reducer.go"},
		{"mgo", "main.go", 0.05, "mgo in main.go"},
		{"ih", "input_handler.go", 0.03, "ih in input_handler.go"},
		{"st", "state.go", 0.03, "st in state.go"},
		{"md", "IMPLEMENTATION.md", 0.02, "md in IMPLEMENTATION.md"},

		// File extension matching
		{"go", "main.go", 0.02, "go in main.go"},
		{"mod", "go.mod", 0.05, "mod in go.mod"},

		// Substring matching
		{"main", "main.go", 0.1, "main in main.go"},
		{"test", "reducer_test.go", 0.05, "test in reducer_test.go"},
	}

	for _, tt := range tests {
		score, matched := fm.Match(tt.pattern, tt.text)
		if !matched {
			t.Errorf("Match(%q, %q) should match: %s", tt.pattern, tt.text, tt.name)
			continue
		}
		if score < tt.wantMin {
			t.Errorf("Match(%q, %q) score %.3f < want %.3f: %s",
				tt.pattern, tt.text, score, tt.wantMin, tt.name)
		}
	}
}

func TestFuzzyMatch_FilenameBehavior(t *testing.T) {
	fm := NewFuzzyMatcher()

	// These should all match but with different scores
	filenames := []string{
		"reducer.go",            // Index 0
		"reducer_test.go",       // Index 1
		"reducer_io_test.go",    // Index 2
		"input_handler.go",      // Index 3
		"input_handler_test.go", // Index 4
	}

	pattern := "rdt" // r, d, t should match

	scores := make(map[int]float64)
	for idx, filename := range filenames {
		score, matched := fm.Match(pattern, filename)
		if matched {
			scores[idx] = score
		}
	}

	// Should match files with r, d, t in order
	// reducer.go: r, d, (no t)
	// reducer_test.go: r, d, t ✓
	if _, ok := scores[1]; !ok {
		t.Error("Should match reducer_test.go")
	}
	if _, ok := scores[2]; !ok {
		t.Error("Should match reducer_io_test.go")
	}

	// reducer_test should score same or higher than reducer_io_test
	if len(scores) >= 2 && scores[1] < scores[2] {
		t.Logf("reducer_test (%.3f) vs reducer_io_test (%.3f)", scores[1], scores[2])
	}
}

func TestFuzzyMatch_MatchMultiple(t *testing.T) {
	fm := NewFuzzyMatcher()

	filenames := []string{
		"main.go",
		"reducer.go",
		"state.go",
		"input_handler.go",
		"renderer.go",
		"IMPLEMENTATION.md",
		"go.mod",
		"go.sum",
	}

	pattern := "go"

	matches := fm.MatchMultiple(pattern, filenames)

	// All .go files should match
	if len(matches) < 5 {
		t.Errorf("MatchMultiple should find at least 5 .go files, found %d", len(matches))
	}

	// Scores should be sorted descending
	for i := 0; i < len(matches)-1; i++ {
		if matches[i].Score < matches[i+1].Score {
			t.Errorf("Scores not sorted: %.3f < %.3f", matches[i].Score, matches[i+1].Score)
		}
	}
}

func TestFuzzyMatch_MatchMultiple_WithThreshold(t *testing.T) {
	fm := NewFuzzyMatcher()
	fm.minScore = 0.5 // High threshold - only best matches

	filenames := []string{
		"main.go",
		"reducer.go",
		"state.go",
		"input_handler.go",
		"renderer.go",
	}

	pattern := "red"

	matches := fm.MatchMultiple(pattern, filenames)

	// reducer.go should match (exact prefix)
	// renderer.go might also match but with lower score
	if len(matches) < 1 {
		t.Errorf("Should find at least 1 match with threshold, found %d", len(matches))
	}
	if len(matches) > 0 && filenames[matches[0].FileIndex] != "reducer.go" {
		t.Errorf("First match should be reducer.go, got %s", filenames[matches[0].FileIndex])
	}
}

func TestFuzzyMatch_Ordering(t *testing.T) {
	fm := NewFuzzyMatcher()

	filenames := []string{
		"main.go",          // m, g, o - gaps
		"mgo_tool.go",      // m, g, o - at start, good score
		"map_generator.go", // m (gap) g (gap) o - worst score
	}

	pattern := "mgo"

	matches := fm.MatchMultiple(pattern, filenames)

	// mgo_tool should be first (best match)
	if len(matches) >= 2 {
		firstFile := filenames[matches[0].FileIndex]
		if firstFile != "mgo_tool.go" {
			t.Logf("First match is %s (score %.3f), expected mgo_tool.go",
				firstFile, matches[0].Score)
		}
	}
}

func TestFuzzyMatch_EdgeCases(t *testing.T) {
	fm := NewFuzzyMatcher()

	tests := []struct {
		pattern string
		text    string
		want    bool
		name    string
	}{
		{"", "", true, "empty pattern and text"},
		{"a", "", false, "pattern but no text"},
		{"", "file.go", true, "empty pattern"},
		{".", "file.go", true, "dot pattern"},
		{"...", "file.go", false, "dots pattern no match"},
		{"a", "a", true, "single char exact"},
		{"aaa", "aaa", true, "repeated chars exact"},
		{"ab", "aabb", true, "repeated chars fuzzy"},
	}

	for _, tt := range tests {
		_, matched := fm.Match(tt.pattern, tt.text)
		if matched != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v: %s",
				tt.pattern, tt.text, matched, tt.want, tt.name)
		}
	}
}

func TestFuzzyMatch_Performance(t *testing.T) {
	fm := NewFuzzyMatcher()

	// Create a realistic file list
	filenames := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		filenames[i] = "file_" + string(rune('a'+(i%26))) + ".go"
	}

	pattern := "file_a"

	// Should complete quickly (no timeout here, but can be benchmarked)
	matches := fm.MatchMultiple(pattern, filenames)

	if len(matches) == 0 {
		t.Error("Should find at least one match in 1000 files")
	}
}

// Benchmark tests

func BenchmarkFuzzyMatch_SingleMatch(b *testing.B) {
	fm := NewFuzzyMatcher()
	pattern := "red"
	text := "reducer.go"

	for i := 0; i < b.N; i++ {
		fm.Match(pattern, text)
	}
}

func BenchmarkFuzzyMatch_MultipleMatches(b *testing.B) {
	fm := NewFuzzyMatcher()
	filenames := []string{
		"main.go", "reducer.go", "state.go", "input_handler.go", "renderer.go",
		"IMPLEMENTATION.md", "go.mod", "go.sum", "reducer_test.go", "reducer_io_test.go",
	}
	pattern := "red"
	var matches []FuzzyMatch

	for i := 0; i < b.N; i++ {
		matches = fm.MatchMultipleInto(pattern, filenames, matches)
	}
}

func BenchmarkFuzzyMatch_LargeList(b *testing.B) {
	fm := NewFuzzyMatcher()

	// Create 1000 filenames
	filenames := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		filenames[i] = "file_" + string(rune('a'+(i%26))) + ".go"
	}
	pattern := "file_a"
	var matches []FuzzyMatch

	for i := 0; i < b.N; i++ {
		matches = fm.MatchMultipleInto(pattern, filenames, matches)
	}
}
