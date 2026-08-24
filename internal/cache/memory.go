package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type memoryEntry struct {
	data      []byte
	expiresAt time.Time
}

func (e *memoryEntry) isExpired(now time.Time) bool {
	if e.expiresAt.IsZero() {
		return false
	}
	return now.After(e.expiresAt)
}

// MemoryCache is a concurrency-safe in-memory cache with TTL and capacity bounds.
type MemoryCache struct {
	mu         sync.RWMutex
	items      map[string]memoryEntry
	maxEntries int
}

// NewMemoryCache creates an in-memory Cache with maximum entry capacity.
func NewMemoryCache(maxEntries int) *MemoryCache {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &MemoryCache{
		items:      make(map[string]memoryEntry),
		maxEntries: maxEntries,
	}
}

// NewMemory creates a default in-memory Cache.
func NewMemory() Cache {
	return NewMemoryCache(10000)
}

func (m *MemoryCache) IsRedis() bool {
	return false
}

func (m *MemoryCache) Get(ctx context.Context, key string, dst any) bool {
	m.mu.RLock()
	entry, exists := m.items[key]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	if entry.isExpired(time.Now()) {
		m.mu.Lock()
		// Double check after write lock
		if e, ok := m.items[key]; ok && e.isExpired(time.Now()) {
			delete(m.items, key)
		}
		m.mu.Unlock()
		return false
	}

	if dst == nil {
		return true
	}

	if err := json.Unmarshal(entry.data, dst); err != nil {
		return false
	}
	return true
}

func (m *MemoryCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// If capacity is reached and this is a new key, purge an expired entry or arbitrary item
	if len(m.items) >= m.maxEntries {
		if _, exists := m.items[key]; !exists {
			now := time.Now()
			purged := false
			for k, e := range m.items {
				if e.isExpired(now) {
					delete(m.items, k)
					purged = true
					break
				}
			}
			if !purged {
				// Purge any one item to bound memory
				for k := range m.items {
					delete(m.items, k)
					break
				}
			}
		}
	}

	m.items[key] = memoryEntry{
		data:      data,
		expiresAt: expiresAt,
	}
	return nil
}

func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}
