package state

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const markdownFrontmatterMaxBytes = 16 * 1024

func stripMarkdownFrontmatter(lines []string) []string {
	_, _, body, ok := splitMarkdownFrontmatter(lines)
	if ok {
		return body
	}
	return lines
}

func splitMarkdownFrontmatter(lines []string) (map[string]any, string, []string, bool) {
	if len(lines) == 0 {
		return nil, "", lines, false
	}
	if strings.TrimSpace(lines[0]) != "---" {
		return nil, "", lines, false
	}

	size := 0
	end := -1
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" || trimmed == "..." {
			end = i
			break
		}
		size += len(lines[i]) + 1
		if size > markdownFrontmatterMaxBytes {
			return nil, "", lines, false
		}
	}
	if end == -1 {
		return nil, "", lines, false
	}

	raw := strings.Join(lines[1:end], "\n")
	if len(raw) > markdownFrontmatterMaxBytes {
		return nil, "", lines, false
	}
	if raw == "" {
		return map[string]any{}, "", lines[end+1:], true
	}

	meta, ok := parseMarkdownFrontmatter(raw)
	if !ok {
		return nil, "", lines, false
	}

	return meta, raw, lines[end+1:], true
}

func parseMarkdownFrontmatter(raw string) (map[string]any, bool) {
	var data any
	if err := yaml.Unmarshal([]byte(raw), &data); err != nil {
		return nil, false
	}
	normalized, ok := normalizeFrontmatterValue(data)
	if !ok {
		return nil, false
	}
	meta, ok := normalized.(map[string]any)
	if !ok {
		return nil, false
	}
	return meta, true
}

func normalizeFrontmatterValue(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			normalized, ok := normalizeFrontmatterValue(val)
			if !ok {
				return nil, false
			}
			out[key] = normalized
		}
		return out, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for rawKey, val := range typed {
			key, ok := rawKey.(string)
			if !ok {
				return nil, false
			}
			normalized, ok := normalizeFrontmatterValue(val)
			if !ok {
				return nil, false
			}
			out[key] = normalized
		}
		return out, true
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			normalized, ok := normalizeFrontmatterValue(item)
			if !ok {
				return nil, false
			}
			out[i] = normalized
		}
		return out, true
	case string, bool, int, int64, float64, uint64, nil:
		return typed, true
	default:
		return nil, false
	}
}
