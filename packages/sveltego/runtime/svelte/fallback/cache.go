package fallback

import (
	"container/list"
	"sync"
	"time"
)

// cacheEntry stores one rendered fallback response. body and head are
// the sidecar's output for the request; expires marks the wall-clock
// instant the entry should be evicted.
type cacheEntry struct {
	body    string
	head    string
	expires time.Time
}

// lruCache is a fixed-capacity, TTL-bounded LRU. Values are looked up
// by an opaque string key composed of `(route, hash(load_result))` —
// the caller decides what hash function suits the data shape; this
// cache only sees the resulting key. Two locks would over-engineer
// the access pattern; one mutex is enough because every operation
// already touches both the hash map and the doubly-linked recency list.
//
// The cache is intentionally simple: no metrics, no admission policy,
// no probabilistic TTL. The fallback path is itself a tail case (only
// annotated routes hit it); the cost we are paying down here is the
// sidecar round-trip, not steady-state contention.
type lruCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
	now      func() time.Time
}

// listValue couples the cache key with the entry so List eviction has
// access to the key without a reverse map.
type listValue struct {
	key   string
	entry cacheEntry
}

func newLRUCache(capacity int, ttl time.Duration) *lruCache {
	if capacity <= 0 {
		capacity = 1
	}
	return &lruCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
		now:      time.Now,
	}
}

// Get returns the cached entry for key when it exists and has not
// expired. Hits move the entry to the MRU end. Expired entries are
// evicted on lookup so the next miss has room.
func (c *lruCache) Get(key string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return cacheEntry{}, false
	}
	v := el.Value.(*listValue)
	if c.ttl > 0 && c.now().After(v.entry.expires) {
		c.order.Remove(el)
		delete(c.items, key)
		return cacheEntry{}, false
	}
	c.order.MoveToFront(el)
	return v.entry, true
}

// Put inserts or refreshes the entry for key. When at capacity the
// least-recently-used entry is evicted.
func (c *lruCache) Put(key string, entry cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ttl > 0 {
		entry.expires = c.now().Add(c.ttl)
	}
	if el, ok := c.items[key]; ok {
		el.Value.(*listValue).entry = entry
		c.order.MoveToFront(el)
		return
	}
	for c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		ov := oldest.Value.(*listValue)
		c.order.Remove(oldest)
		delete(c.items, ov.key)
	}
	el := c.order.PushFront(&listValue{key: key, entry: entry})
	c.items[key] = el
}

// Len reports the current number of cached entries (including expired
// ones not yet evicted by a Get). Test-only utility; not exported.
func (c *lruCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
