package search

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

type ignoreProvider struct {
	root  string
	cache sync.Map // map[string]*GitignoreMatcher
}

func newIgnoreProvider(root string) *ignoreProvider {
	provider := &ignoreProvider{
		root: root,
	}

	base := NewGitignoreMatcher()
	provider.applyGlobalPatterns(base)
	provider.addPatternFileIfExists(base, filepath.Join(root, ".git", "info", "exclude"), root)
	provider.applyDirectoryPatterns(base, root)
	provider.cache.Store(".", base)

	return provider
}

func (p *ignoreProvider) MatcherFor(relDir string) *GitignoreMatcher {
	key := normalizeDirKey(relDir)

	if matcher, ok := p.cache.Load(key); ok {
		return matcher.(*GitignoreMatcher)
	}

	parentKey := parentDirKey(key)
	parentMatcher := p.MatcherFor(parentKey)
	child := parentMatcher.Clone()

	fullDir := p.fullPathFromKey(key)
	p.applyDirectoryPatterns(child, fullDir)

	p.cache.Store(key, child)
	return child
}

func (p *ignoreProvider) fullPathFromKey(key string) string {
	if key == "." {
		return p.root
	}
	return filepath.Join(p.root, filepath.FromSlash(key))
}

func (p *ignoreProvider) applyDirectoryPatterns(matcher *GitignoreMatcher, dir string) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}

	// Lowest priority first so later files can override with negations.
	p.addPatternFileIfExists(matcher, filepath.Join(dir, ".gitignore"), dir)
	p.addPatternFileIfExists(matcher, filepath.Join(dir, ".ignore"), dir)
	p.addPatternFileIfExists(matcher, filepath.Join(dir, ".rdirignore"), dir)
}

func (p *ignoreProvider) applyGlobalPatterns(matcher *GitignoreMatcher) {
	seen := make(map[string]struct{})

	add := func(candidate string) {
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		if p.addPatternFileIfExists(matcher, candidate, p.root) {
			seen[candidate] = struct{}{}
		}
	}

	add(p.coreExcludesFile())

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".gitignore"))
		add(filepath.Join(home, ".gitignore_global"))
		add(filepath.Join(home, ".config", "git", "ignore"))
	}
}

func (p *ignoreProvider) addPatternFileIfExists(matcher *GitignoreMatcher, filePath string, base string) bool {
	if filePath == "" {
		return false
	}

	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}

	data, err := os.ReadFile(filePath)
	if err != nil || len(data) == 0 {
		return false
	}

	matcher.AddPatterns(string(data), base)
	return true
}

func (p *ignoreProvider) coreExcludesFile() string {
	configPath := filepath.Join(p.root, ".git", "config")
	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	inCore := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.TrimSpace(line))
			inCore = strings.HasPrefix(section, "[core")
			continue
		}

		if !inCore {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "excludesfile") {
			value := extractConfigValue(line)
			value = expandUserPath(value)
			if value == "" {
				continue
			}
			if !filepath.IsAbs(value) {
				value = filepath.Join(p.root, value)
			}
			return value
		}
	}

	return ""
}

func extractConfigValue(line string) string {
	if idx := strings.Index(line, "="); idx >= 0 {
		return strings.TrimSpace(line[idx+1:])
	}

	fields := strings.Fields(line)
	if len(fields) <= 1 {
		return ""
	}
	return strings.Join(fields[1:], " ")
}

func expandUserPath(value string) string {
	if value == "" {
		return ""
	}

	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return value
		}

		if value == "~" {
			return home
		}

		if strings.HasPrefix(value, "~/") {
			return filepath.Join(home, value[2:])
		}
	}

	return value
}

func normalizeDirKey(relDir string) string {
	if relDir == "" {
		return "."
	}

	cleaned := filepath.Clean(relDir)
	if cleaned == "." {
		return "."
	}

	cleaned = filepath.ToSlash(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "" || cleaned == "/" {
		return "."
	}

	return cleaned
}

func parentDirKey(relDir string) string {
	if relDir == "." {
		return "."
	}

	parent := path.Dir(relDir)
	if parent == "." || parent == "/" {
		return "."
	}

	return parent
}
