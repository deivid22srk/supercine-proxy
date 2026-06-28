package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/types"
)

// Cache is a simple TTL+LRU in-memory cache for upstream GET responses.
// Only successful (2xx) responses with cacheable content types are stored.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*types.CacheEntry
	ttl     time.Duration
	max     int
}

// New creates a Cache with the given TTL and max entries.
func New(ttl time.Duration, max int) *Cache {
	if max <= 0 {
		max = 1000
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Cache{
		entries: make(map[string]*types.CacheEntry, max),
		ttl:     ttl,
		max:     max,
	}
}

// Get returns a cached entry if it exists and hasn't expired.
func (c *Cache) Get(key string) (*types.CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Since(e.StoredAt) > c.ttl {
		return nil, false
	}
	return e, true
}

// Set stores an entry. Evicts an arbitrary old entry when at capacity.
func (c *Cache) Set(key string, e *types.CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.max {
		// Evict the oldest entry by StoredAt.
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.StoredAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.StoredAt
			}
		}
		delete(c.entries, oldestKey)
	}
	e.StoredAt = time.Now()
	c.entries[key] = e
}

// Delete removes a single entry.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear empties the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*types.CacheEntry, c.max)
}

// Size returns the current number of cached entries.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Key produces a stable cache key from method + url + body hash.
func Key(method, url string, body []byte) string {
	h := sha256.Sum256([]byte(method + "|" + url + "|" + string(body)))
	return hex.EncodeToString(h[:])
}
