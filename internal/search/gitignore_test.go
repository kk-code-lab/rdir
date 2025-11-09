package search

import (
	"testing"
)

// TestEmptyLinesAndComments tests that empty lines and comments are ignored
func TestEmptyLinesAndComments(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name: "empty line is ignored",
			gitignore: `
*.log
`,
			path:         "file.txt",
			shouldIgnore: false,
		},
		{
			name: "comment line is ignored",
			gitignore: `# This is a comment
*.log`,
			path:         "debug.log",
			shouldIgnore: true,
		},
		{
			name: "escaped hash at beginning",
			gitignore: `\#pattern
*.log`,
			path:         "#pattern",
			shouldIgnore: true,
		},
		{
			name: "multiple comments and empty lines",
			gitignore: `# Comment 1

# Comment 2

*.log
`,
			path:         "test.log",
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestNegationPatterns tests the ! prefix for negating patterns
func TestNegationPatterns(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name: "negation reverses ignore",
			gitignore: `*.log
!important.log`,
			path:         "important.log",
			shouldIgnore: false,
		},
		{
			name: "negation doesn't affect other files",
			gitignore: `*.log
!important.log`,
			path:         "debug.log",
			shouldIgnore: true,
		},
		{
			name:         "escaped exclamation at beginning",
			gitignore:    `\!literal.txt`,
			path:         "!literal.txt",
			shouldIgnore: true,
		},
		{
			name: "first pattern ignored, negation applies",
			gitignore: `*.c
!special.c`,
			path:         "special.c",
			shouldIgnore: false,
		},
		{
			name: "last matching pattern wins",
			gitignore: `*.log
!*.log
debug.log`,
			path:         "debug.log",
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestDirectoriesVsFiles tests the trailing slash for directory-only patterns
func TestDirectoriesVsFiles(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		isDir        bool
		shouldIgnore bool
	}{
		{
			name:         "trailing slash matches only directories",
			gitignore:    `build/`,
			path:         "build",
			isDir:        true,
			shouldIgnore: true,
		},
		{
			name:         "trailing slash doesn't match file",
			gitignore:    `build/`,
			path:         "build",
			isDir:        false,
			shouldIgnore: false,
		},
		{
			name:         "pattern without slash matches both file and directory",
			gitignore:    `build`,
			path:         "build",
			isDir:        true,
			shouldIgnore: true,
		},
		{
			name:         "pattern without slash matches file",
			gitignore:    `build`,
			path:         "build",
			isDir:        false,
			shouldIgnore: true,
		},
		{
			name:         "trailing slash in nested directory",
			gitignore:    `dist/`,
			path:         "dist",
			isDir:        true,
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.MatchWithType(tt.path, tt.isDir)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q (isDir: %v)", tt.shouldIgnore, result, tt.path, tt.isDir)
			}
		})
	}
}

// TestAnchoring tests path anchoring with leading slashes
func TestAnchoring(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		isDir        bool
		shouldIgnore bool
	}{
		{
			name:         "no slash - matches anywhere",
			gitignore:    `*.log`,
			path:         "logs/debug.log",
			isDir:        false,
			shouldIgnore: true,
		},
		{
			name:         "leading slash - anchors to root",
			gitignore:    `/*.log`,
			path:         "debug.log",
			isDir:        false,
			shouldIgnore: true,
		},
		{
			name:         "leading slash - doesn't match in subdirectories",
			gitignore:    `/*.log`,
			path:         "logs/debug.log",
			isDir:        false,
			shouldIgnore: false,
		},
		{
			name:         "slash in middle - restricts wildcards",
			gitignore:    `docs/*.html`,
			path:         "docs/guide.html",
			isDir:        false,
			shouldIgnore: true,
		},
		{
			name:         "slash in middle - doesn't cross directories",
			gitignore:    `docs/*.html`,
			path:         "docs/en/guide.html",
			isDir:        false,
			shouldIgnore: false,
		},
		{
			name:         "leading slash with directory",
			gitignore:    `/build/`,
			path:         "build",
			isDir:        true,
			shouldIgnore: true,
		},
		{
			name:         "leading slash with directory - not in subdirectories",
			gitignore:    `/build/`,
			path:         "src/build",
			isDir:        true,
			shouldIgnore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.MatchWithType(tt.path, tt.isDir)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q (isDir: %v)", tt.shouldIgnore, result, tt.path, tt.isDir)
			}
		})
	}
}

// TestDoubleStarPatterns tests ** wildcard handling
func TestDoubleStarPatterns(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name:         "** at start matches anywhere",
			gitignore:    `**/foo`,
			path:         "foo",
			shouldIgnore: true,
		},
		{
			name:         "** at start matches in subdirectories",
			gitignore:    `**/foo`,
			path:         "dir/foo",
			shouldIgnore: true,
		},
		{
			name:         "** at start matches deeply nested",
			gitignore:    `**/foo`,
			path:         "a/b/c/foo",
			shouldIgnore: true,
		},
		{
			name:         "** at end matches everything inside",
			gitignore:    `build/**`,
			path:         "build/output.o",
			shouldIgnore: true,
		},
		{
			name:         "** at end matches nested content",
			gitignore:    `build/**`,
			path:         "build/sub/file.o",
			shouldIgnore: true,
		},
		{
			name:         "** in middle matches zero or more dirs",
			gitignore:    `a/**/b`,
			path:         "a/b",
			shouldIgnore: true,
		},
		{
			name:         "** in middle matches one dir",
			gitignore:    `a/**/b`,
			path:         "a/x/b",
			shouldIgnore: true,
		},
		{
			name:         "** in middle matches multiple dirs",
			gitignore:    `a/**/b`,
			path:         "a/x/y/z/b",
			shouldIgnore: true,
		},
		{
			name:         "** with pattern after",
			gitignore:    `**/foo/bar`,
			path:         "x/y/foo/bar",
			shouldIgnore: true,
		},
		{
			name:         "** doesn't match without full pattern",
			gitignore:    `a/**/b`,
			path:         "a/b/c",
			shouldIgnore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestWildcards tests single character and multi-character wildcards
func TestWildcards(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name:         "? matches single character",
			gitignore:    `file?.txt`,
			path:         "file1.txt",
			shouldIgnore: true,
		},
		{
			name:         "? doesn't match slash",
			gitignore:    `file?.txt`,
			path:         "file/1.txt",
			shouldIgnore: false,
		},
		{
			name:         "* matches zero or more characters",
			gitignore:    `*.log`,
			path:         "debug.log",
			shouldIgnore: true,
		},
		{
			name:         "* at any level with no slash",
			gitignore:    `*.log`,
			path:         "logs/debug.log",
			shouldIgnore: true,
		},
		{
			name:         "? doesn't match / even in pattern",
			gitignore:    `file?.txt`,
			path:         "file/txt",
			shouldIgnore: false,
		},
		{
			name:         "character class matches",
			gitignore:    `file[abc].txt`,
			path:         "filea.txt",
			shouldIgnore: true,
		},
		{
			name:         "character class range matches",
			gitignore:    `file[a-z].txt`,
			path:         "filem.txt",
			shouldIgnore: true,
		},
		{
			name:         "character class negation matches",
			gitignore:    `file[!0-9].txt`,
			path:         "filea.txt",
			shouldIgnore: true,
		},
		{
			name:         "character class negation doesn't match",
			gitignore:    `file[!0-9].txt`,
			path:         "file5.txt",
			shouldIgnore: false,
		},
		{
			name:         "multiple wildcards",
			gitignore:    `*.tar.gz`,
			path:         "archive.tar.gz",
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestTrailingSpaces tests handling of trailing spaces
func TestTrailingSpaces(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name:         "trailing spaces are ignored",
			gitignore:    `*.log   `,
			path:         "debug.log",
			shouldIgnore: true,
		},
		{
			name:         "escaped trailing spaces are preserved",
			gitignore:    `pattern\ `,
			path:         "pattern ",
			shouldIgnore: true,
		},
		{
			name:         "escaped trailing spaces don't match without space",
			gitignore:    `pattern\ `,
			path:         "pattern",
			shouldIgnore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestSequentialPatterns tests that patterns are checked in order and last match wins
func TestSequentialPatterns(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name: "last matching pattern wins - ignore wins",
			gitignore: `*.log
!debug.log
debug.log`,
			path:         "debug.log",
			shouldIgnore: true,
		},
		{
			name: "last matching pattern wins - negation wins",
			gitignore: `*.log
debug.log
!debug.log`,
			path:         "debug.log",
			shouldIgnore: false,
		},
		{
			name: "multiple negations",
			gitignore: `*.log
!*.log
debug.log`,
			path:         "debug.log",
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestComplexExamples tests real-world examples from gitignore documentation
func TestComplexExamples(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name: "ignore build directory and keep gitkeep",
			gitignore: `build/
!build/.gitkeep`,
			path:         "build/.gitkeep",
			shouldIgnore: false,
		},
		{
			name: "ignore all .a except lib.a",
			gitignore: `*.a
!lib.a`,
			path:         "lib.a",
			shouldIgnore: false,
		},
		{
			name: "ignore all .a except lib.a - other .a files",
			gitignore: `*.a
!lib.a`,
			path:         "other.a",
			shouldIgnore: true,
		},
		{
			name:         "ignore TODO only in root",
			gitignore:    `/TODO`,
			path:         "TODO",
			shouldIgnore: true,
		},
		{
			name:         "ignore TODO only in root - not in subdirs",
			gitignore:    `/TODO`,
			path:         "docs/TODO",
			shouldIgnore: false,
		},
		{
			name:         "ignore all txt in doc directory",
			gitignore:    `doc/*.txt`,
			path:         "doc/notes.txt",
			shouldIgnore: true,
		},
		{
			name:         "don't ignore txt in doc subdirectories",
			gitignore:    `doc/*.txt`,
			path:         "doc/notes/notes.txt",
			shouldIgnore: false,
		},
		{
			name:         "ignore doc notes with double star",
			gitignore:    `doc/**/*.txt`,
			path:         "doc/server/arch.txt",
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestLeadingWhitespace tests that leading whitespace is preserved
func TestLeadingWhitespace(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		shouldIgnore bool
	}{
		{
			name:         "leading spaces are preserved in pattern",
			gitignore:    `  *.log`,
			path:         "  debug.log",
			shouldIgnore: true, // Pattern with spaces matches files with those spaces
		},
		{
			name:         "leading spaces don't match files without spaces",
			gitignore:    `  *.log`,
			path:         "debug.log",
			shouldIgnore: false, // Different spacing doesn't match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestNestedGitignore tests multiple .gitignore files in different directories
func TestNestedGitignore(t *testing.T) {
	tests := []struct {
		name          string
		gitignorePath string
		gitignore     string
		path          string
		shouldIgnore  bool
	}{
		{
			name:          "pattern relative to gitignore location",
			gitignorePath: "logs",
			gitignore:     `debug.log`,
			path:          "logs/debug.log",
			shouldIgnore:  true,
		},
		{
			name:          "pattern applies from gitignore directory",
			gitignorePath: "docs",
			gitignore:     `*.pdf`,
			path:          "docs/guide.pdf",
			shouldIgnore:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, tt.gitignorePath)
			result := matcher.Match(tt.path)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q", tt.shouldIgnore, result, tt.path)
			}
		})
	}
}

// TestEdgeCases tests special cases and edge cases
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		gitignore    string
		path         string
		isDir        bool
		shouldIgnore bool
	}{
		{
			name:         "backslash at end of line",
			gitignore:    `pattern\`,
			path:         "pattern\\",
			isDir:        false,
			shouldIgnore: false,
		},
		{
			name:         "empty pattern",
			gitignore:    ``,
			path:         "anything.txt",
			isDir:        false,
			shouldIgnore: false,
		},
		{
			name:         "whitespace only",
			gitignore:    `   `,
			path:         "file.txt",
			isDir:        false,
			shouldIgnore: false,
		},
		{
			name:         "double asterisk alone",
			gitignore:    `**`,
			path:         "anything.txt",
			isDir:        false,
			shouldIgnore: true,
		},
		{
			name:         "double asterisk slash matches everything",
			gitignore:    `**/`,
			path:         "dir",
			isDir:        true,
			shouldIgnore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewGitignoreMatcher()
			matcher.AddPatterns(tt.gitignore, ".")
			result := matcher.MatchWithType(tt.path, tt.isDir)
			if result != tt.shouldIgnore {
				t.Errorf("expected %v, got %v for path %q (isDir: %v)", tt.shouldIgnore, result, tt.path, tt.isDir)
			}
		})
	}
}
