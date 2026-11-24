package help

import "testing"

func TestHelpLRUCacheBoundsEntries(t *testing.T) {
	cache := newLRU[int, int](2)
	cache.Put(1, 10)
	cache.Put(2, 20)
	cache.Put(3, 30)

	if _, ok := cache.Get(1); ok {
		t.Error("oldest entry was not evicted")
	}
	if value, ok := cache.Get(2); !ok || value != 20 {
		t.Errorf("Get(2) = %d, %v", value, ok)
	}
	if value, ok := cache.Get(3); !ok || value != 30 {
		t.Errorf("Get(3) = %d, %v", value, ok)
	}
}

func TestHelpLRUCacheNilSafety(t *testing.T) {
	var cache *lruCache[int, int]
	if _, ok := cache.Get(1); ok {
		t.Error("nil cache returned a hit")
	}
	cache.Put(1, 1)
}
