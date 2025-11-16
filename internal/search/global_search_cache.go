package search

import (
	"strings"
	"sync"
)

type cacheKey struct {
	rootPath string
	query    string
	caseSens bool
	indexGen int
}

type cacheValue struct {
	results []GlobalSearchResult
}

type searchCache struct {
	// Cached slices are treated as immutable; callers must not modify returned results.
	mu       sync.RWMutex
	entries  map[cacheKey]cacheValue
	capacity int
}

func newSearchCache() *searchCache {
	return &searchCache{
		entries:  make(map[cacheKey]cacheValue),
		capacity: 32,
	}
}

func (c *searchCache) get(key cacheKey) ([]GlobalSearchResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return value.results, true
}

func (c *searchCache) put(key cacheKey, results []GlobalSearchResult) {
	if len(results) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.capacity {
		c.entries = make(map[cacheKey]cacheValue)
	}
	c.entries[key] = cacheValue{results: results}
}

func (c *searchCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[cacheKey]cacheValue)
}

func normalizeCacheQuery(query string, caseSensitive bool) string {
	if caseSensitive {
		return query
	}
	return strings.ToLower(query)
}
