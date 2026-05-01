package fallback

import (
	"testing"
	"time"
)

func TestLRUCachePutGet(t *testing.T) {
	t.Parallel()
	c := newLRUCache(3, time.Minute)
	c.Put("a", cacheEntry{body: "A"})
	c.Put("b", cacheEntry{body: "B"})
	c.Put("c", cacheEntry{body: "C"})
	if e, ok := c.Get("a"); !ok || e.body != "A" {
		t.Fatalf("Get(a) = %+v, ok=%v", e, ok)
	}
	if e, ok := c.Get("c"); !ok || e.body != "C" {
		t.Fatalf("Get(c) = %+v, ok=%v", e, ok)
	}
}

func TestLRUCacheEvictOldest(t *testing.T) {
	t.Parallel()
	c := newLRUCache(2, time.Minute)
	c.Put("a", cacheEntry{body: "A"})
	c.Put("b", cacheEntry{body: "B"})
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected a hit")
	}
	c.Put("c", cacheEntry{body: "C"})
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected b evicted (LRU)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should still be present after most recent access")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("c should be present (last inserted)")
	}
}

func TestLRUCacheTTLEvict(t *testing.T) {
	t.Parallel()
	c := newLRUCache(2, 100*time.Millisecond)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }
	c.Put("a", cacheEntry{body: "A"})
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should be hot")
	}
	now = now.Add(101 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have expired")
	}
	if c.Len() != 0 {
		t.Fatalf("Len = %d, want 0 (expired entry should be evicted on lookup)", c.Len())
	}
}

func TestLRUCacheRefreshOnPut(t *testing.T) {
	t.Parallel()
	c := newLRUCache(2, time.Minute)
	c.Put("a", cacheEntry{body: "A"})
	c.Put("a", cacheEntry{body: "A2"})
	if e, _ := c.Get("a"); e.body != "A2" {
		t.Fatalf("expected refreshed value A2, got %q", e.body)
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (refresh shouldn't double-count)", c.Len())
	}
}
