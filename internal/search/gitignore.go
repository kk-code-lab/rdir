package search

import (
	"path/filepath"
	"strings"
)

// GitignoreMatcher parses and matches patterns against file paths
type GitignoreMatcher struct {
	patterns []gitignorePattern
}

type gitignorePattern struct {
	pattern  string // The parsed pattern
	negation bool   // Whether this is a negation pattern (!)
	dirOnly  bool   // Whether pattern ends with / (matches only directories)
	anchored bool   // Whether pattern is anchored to root (starts with /)
	hasSlash bool   // Whether pattern contains / in the middle
	basePath string // Base directory for this pattern
	original string // Original pattern for reference
	literal  string // Exact literal match (no wildcards)
	prefix   string // Simple prefix match (foo*)
	suffix   string // Simple suffix match (*foo)
}

// NewGitignoreMatcher creates a new gitignore matcher
func NewGitignoreMatcher() *GitignoreMatcher {
	return &GitignoreMatcher{
		patterns: make([]gitignorePattern, 0),
	}
}

// Clone creates a deep copy of the matcher so callers can extend rule sets
// without mutating the original slice shared with other directories.
func (gm *GitignoreMatcher) Clone() *GitignoreMatcher {
	if gm == nil {
		return NewGitignoreMatcher()
	}

	clone := NewGitignoreMatcher()
	if len(gm.patterns) > 0 {
		clone.patterns = make([]gitignorePattern, len(gm.patterns))
		copy(clone.patterns, gm.patterns)
	}
	return clone
}

// AddPatterns parses a gitignore file content and adds all patterns
func (gm *GitignoreMatcher) AddPatterns(content string, basePath string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		gm.addPattern(line, basePath)
	}
}

// addPattern parses a single line and adds it as a pattern
func (gm *GitignoreMatcher) addPattern(line string, basePath string) {
	original := line

	// Trim trailing spaces (unless they were escaped)
	line = gm.trimTrailingSpaces(line)

	// Skip empty lines
	if line == "" {
		return
	}

	// Skip comments (but not escaped #)
	// Check for comment BEFORE processing escapes so \# is not treated as comment
	if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "\\#") {
		return
	}

	// Parse negation BEFORE processing escapes
	negation := false
	if strings.HasPrefix(line, "!") && !strings.HasPrefix(line, "\\!") {
		negation = true
		line = line[1:]
	}

	// Process escapes - convert \# to #, \! to !, etc.
	line = gm.processEscapes(line)

	// Check for directory-only pattern
	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// Check if pattern is anchored to root
	anchored := false
	if strings.HasPrefix(line, "/") {
		anchored = true
		line = line[1:]
	}

	// Check if pattern has slash in the middle
	hasSlash := strings.ContainsRune(line, '/')

	// Skip if pattern is empty after processing
	if line == "" {
		return
	}

	blankslash := strings.ContainsRune(line, '\\')
	literal := ""
	prefix := ""
	suffix := ""

	if !blankslash && !strings.ContainsAny(line, "*?[") {
		literal = line
	} else if !blankslash {
		if strings.HasPrefix(line, "*") && !strings.HasPrefix(line, "**") {
			rest := line[1:]
			if rest != "" && !strings.ContainsAny(rest, "*?[") {
				suffix = rest
			}
		}
		if strings.HasSuffix(line, "*") && !strings.HasSuffix(line, "**") {
			start := line[:len(line)-1]
			if start != "" && !strings.ContainsAny(start, "*?[") {
				prefix = start
			}
		}
	}

	gm.patterns = append(gm.patterns, gitignorePattern{
		pattern:  line,
		negation: negation,
		dirOnly:  dirOnly,
		anchored: anchored,
		hasSlash: hasSlash,
		basePath: basePath,
		original: original,
		literal:  literal,
		prefix:   prefix,
		suffix:   suffix,
	})
}

// processEscapes handles backslash escape sequences
// Converts \# to #, \! to !, \  to (space), etc.
func (gm *GitignoreMatcher) processEscapes(line string) string {
	var result strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '\\' && i+1 < len(line) {
			// Skip the backslash and take the next character literally
			i++
			result.WriteByte(line[i])
			i++
		} else {
			result.WriteByte(line[i])
			i++
		}
	}
	return result.String()
}

// trimTrailingSpaces trims trailing spaces but preserves escaped spaces
func (gm *GitignoreMatcher) trimTrailingSpaces(line string) string {
	// Find the last non-space character that's not escaped
	i := len(line) - 1
	for i >= 0 && line[i] == ' ' {
		// Check if this space is escaped
		numBackslashes := 0
		j := i - 1
		for j >= 0 && line[j] == '\\' {
			numBackslashes++
			j--
		}
		// If odd number of backslashes, the space is escaped
		if numBackslashes%2 == 1 {
			break
		}
		i--
	}
	return line[:i+1]
}

// Match checks if a path should be ignored
// It assumes the path is a file (not a directory)
func (gm *GitignoreMatcher) Match(path string) bool {
	return gm.MatchWithType(path, false)
}

// MatchWithType checks if a path should be ignored, with directory type information
func (gm *GitignoreMatcher) MatchWithType(path string, isDir bool) bool {
	// Normalize path separators
	path = filepath.ToSlash(path)

	ignored := false

	// Check all patterns in order - last matching pattern wins
	for _, p := range gm.patterns {
		if gm.matchesPattern(path, isDir, p) {
			ignored = !p.negation
		}
	}

	return ignored
}

// matchesPattern checks if a path matches a single pattern
func (gm *GitignoreMatcher) matchesPattern(path string, isDir bool, p gitignorePattern) bool {
	// Skip if this is a directory-only pattern and it's not a directory
	if p.dirOnly && !isDir {
		return false
	}

	// For anchored patterns, path must be relative to basePath
	checkPath := path
	if p.basePath != "." {
		// If the path doesn't start with the base path, it doesn't match
		// This handles nested .gitignore files
		basePath := filepath.ToSlash(p.basePath)
		if !strings.HasPrefix(path, basePath) {
			return false
		}
		// Remove the base path prefix for matching
		checkPath = strings.TrimPrefix(path, basePath+"/")
		if checkPath == path {
			// The path is the base directory itself
			checkPath = filepath.Base(path)
		}
	}

	filename := checkPath
	if idx := strings.LastIndexByte(checkPath, '/'); idx >= 0 {
		filename = checkPath[idx+1:]
	}

	componentMatch := !p.hasSlash && !p.anchored

	// Fast path: literal match with or without slash
	if p.literal != "" {
		if componentMatch {
			if filename == p.literal || checkPath == p.literal {
				return true
			}
		} else if checkPath == p.literal {
			return true
		}
	}

	if p.suffix != "" && !p.anchored {
		if (componentMatch && strings.HasSuffix(filename, p.suffix)) || strings.HasSuffix(checkPath, p.suffix) {
			return true
		}
	}

	if p.prefix != "" && !p.anchored {
		if (componentMatch && strings.HasPrefix(filename, p.prefix)) || strings.HasPrefix(checkPath, p.prefix) {
			return true
		}
	}

	// Handle special case: ** matches everything
	if p.pattern == "**" {
		return true
	}

	// Handle ** at the beginning
	if strings.HasPrefix(p.pattern, "**/") {
		subPattern := strings.TrimPrefix(p.pattern, "**/")
		// ** at beginning matches at any depth
		return gm.matchesPathComponent(checkPath, subPattern, p.hasSlash)
	}

	// Handle ** at the end
	if strings.HasSuffix(p.pattern, "/**") {
		prefix := strings.TrimSuffix(p.pattern, "/**")
		// Everything inside this directory is matched
		return checkPath == prefix || strings.HasPrefix(checkPath, prefix+"/")
	}

	// Handle ** in the middle
	if strings.Contains(p.pattern, "/**/") {
		parts := strings.Split(p.pattern, "/**/")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			// Pattern like a/**/b matches a/b, a/x/b, a/x/y/b, etc.
			if !strings.HasPrefix(checkPath, prefix+"/") && checkPath != prefix {
				return false
			}
			if strings.HasPrefix(checkPath, prefix+"/") {
				remaining := strings.TrimPrefix(checkPath, prefix+"/")
				return gm.matchesDoubleStarPattern(remaining, suffix)
			}
			if checkPath == prefix {
				return gm.fnmatch(suffix, "")
			}
			return false
		}
	}

	// If pattern is anchored to root, it must match from the beginning
	if p.anchored {
		return gm.fnmatch(p.pattern, checkPath)
	}

	// If pattern has no slash, it can match at any directory level
	// Check if pattern matches the full path or any suffix of it
	if !p.hasSlash {
		// First try the full path
		if gm.fnmatch(p.pattern, checkPath) {
			return true
		}

		// Try each path suffix (by removing leading components)
		parts := strings.Split(checkPath, "/")
		for i := 1; i < len(parts); i++ {
			suffix := strings.Join(parts[i:], "/")
			if gm.fnmatch(p.pattern, suffix) {
				return true
			}
		}
		return false
	}

	// Pattern has slash - match exact path
	return gm.fnmatch(p.pattern, checkPath)
}

// matchesPathComponent matches a pattern against any path component
// Used for ** at the beginning of pattern
func (gm *GitignoreMatcher) matchesPathComponent(path string, pattern string, hasSlash bool) bool {
	// Try matching at each path component
	if gm.fnmatch(pattern, path) {
		return true
	}

	// Also try matching against file basename for patterns without slashes
	if !hasSlash {
		filename := filepath.Base(path)
		if gm.fnmatch(pattern, filename) {
			return true
		}
	}

	// Try parent directories if the path contains slashes
	if strings.Contains(path, "/") {
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			subPath := strings.Join(parts[i:], "/")
			if gm.fnmatch(pattern, subPath) {
				return true
			}
		}
	}

	return false
}

// matchesDoubleStarPattern matches patterns with ** in the middle
// e.g., for pattern "b" in "a/**/b", check if path ends with /b or is exactly b
func (gm *GitignoreMatcher) matchesDoubleStarPattern(path string, pattern string) bool {
	// Check if we can match the suffix at any point
	if gm.fnmatch(pattern, path) {
		return true
	}

	// Check if pattern matches at any subdirectory level
	if strings.Contains(path, "/") {
		parts := strings.Split(path, "/")
		for i := 0; i < len(parts); i++ {
			subPath := strings.Join(parts[i:], "/")
			if gm.fnmatch(pattern, subPath) {
				return true
			}
		}
	}

	return false
}

// fnmatch implements gitignore-style glob matching
// Similar to fnmatch(3) but with Git-specific behavior
func (gm *GitignoreMatcher) fnmatch(pattern string, path string) bool {
	return gm.fnmatchHelper(pattern, path, 0, 0, false)
}

// fnmatchHelper is a recursive helper for fnmatch
func (gm *GitignoreMatcher) fnmatchHelper(pattern string, path string, pi int, pathi int, inBrackets bool) bool {
	patLen := len(pattern)
	pathLen := len(path)

	for pi < patLen && pathi < pathLen {
		pc := pattern[pi]
		pathc := path[pathi]

		switch pc {
		case '*':
			// Check for **
			if pi+1 < patLen && pattern[pi+1] == '*' {
				// ** is not valid in the middle of a pattern for simple matching
				// Just treat it as *
				pi++
				// Try matching zero characters
				if gm.fnmatchHelper(pattern, path, pi+1, pathi, inBrackets) {
					return true
				}
				// Try matching one or more characters
				if pathi < pathLen && pathc != '/' {
					if gm.fnmatchHelper(pattern, path, pi, pathi+1, inBrackets) {
						return true
					}
				}
				return false
			}
			// Single * - matches zero or more characters except /
			if pi+1 >= patLen {
				// * at end matches everything except /
				return !strings.Contains(path[pathi:], "/")
			}

			// Try matching zero characters
			if gm.fnmatchHelper(pattern, path, pi+1, pathi, inBrackets) {
				return true
			}
			// Try matching one or more characters
			if pathc != '/' {
				if gm.fnmatchHelper(pattern, path, pi, pathi+1, inBrackets) {
					return true
				}
			}
			return false

		case '?':
			// ? matches any single character except /
			if pathc == '/' {
				return false
			}
			pi++
			pathi++

		case '[':
			// Character class
			if pathi >= pathLen {
				return false
			}

			// Find the closing ]
			close := gm.findClosingBracket(pattern, pi)
			if close == -1 {
				// No closing bracket, treat [ as literal
				if pathc == '[' {
					pi++
					pathi++
				} else {
					return false
				}
			} else {
				classPattern := pattern[pi+1 : close]
				if !gm.matchCharacterClass(classPattern, pathc) {
					return false
				}
				pi = close + 1
				pathi++
			}

		case '\\':
			// Escaped character
			if pi+1 < patLen {
				pi++
				pc = pattern[pi]
				if pc != pathc {
					return false
				}
				pi++
				pathi++
			} else {
				// Backslash at end - shouldn't happen due to parsing
				return false
			}

		default:
			if pc != pathc {
				return false
			}
			pi++
			pathi++
		}
	}

	// Handle remaining pattern
	for pi < patLen {
		if pattern[pi] == '*' {
			pi++
		} else {
			return false
		}
	}

	return pathi >= pathLen
}

// findClosingBracket finds the closing ] for a character class
func (gm *GitignoreMatcher) findClosingBracket(pattern string, start int) int {
	// start points to [
	i := start + 1
	for i < len(pattern) {
		if pattern[i] == ']' {
			return i
		}
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i += 2
		} else {
			i++
		}
	}
	return -1
}

// matchCharacterClass matches a character against a character class pattern
// e.g., "abc", "a-z", "!abc", "!a-z"
func (gm *GitignoreMatcher) matchCharacterClass(class string, c byte) bool {
	negation := false
	if strings.HasPrefix(class, "!") {
		negation = true
		class = class[1:]
	}

	matched := false
	i := 0
	for i < len(class) {
		if i+2 < len(class) && class[i+1] == '-' {
			// Range like a-z
			start := class[i]
			end := class[i+2]
			if c >= start && c <= end {
				matched = true
				break
			}
			i += 3
		} else {
			// Single character
			if class[i] == c {
				matched = true
				break
			}
			i++
		}
	}

	if negation {
		return !matched
	}
	return matched
}
