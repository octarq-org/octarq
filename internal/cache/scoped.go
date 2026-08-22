package cache

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// ScopedCache wraps a Cache instance and prefixes all keys with a plugin namespace.
type ScopedCache struct {
	backend Cache
	prefix  string
	mu      sync.RWMutex
	tags    map[string]map[string]struct{} // tag -> set of keys (with prefix)
}

// NewScoped constructs a ScopedCache for the given plugin namespace.
func NewScoped(backend Cache, prefix string) plugin.ScopedCache {
	if !strings.HasSuffix(prefix, ":") && prefix != "" {
		prefix += ":"
	}
	return &ScopedCache{
		backend: backend,
		prefix:  prefix,
		tags:    make(map[string]map[string]struct{}),
	}
}

func (s *ScopedCache) fullKey(key string) string {
	return s.prefix + key
}

func (s *ScopedCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	if s.backend == nil {
		return false, nil
	}
	found := s.backend.Get(ctx, s.fullKey(key), dest)
	return found, nil
}

func (s *ScopedCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if s.backend == nil {
		return nil
	}
	return s.backend.Set(ctx, s.fullKey(key), val, ttl)
}

func (s *ScopedCache) Delete(ctx context.Context, key string) error {
	if s.backend == nil {
		return nil
	}
	return s.backend.Delete(ctx, s.fullKey(key))
}

func (s *ScopedCache) InvalidateTag(ctx context.Context, tag string) error {
	if s.backend == nil {
		return nil
	}
	s.mu.Lock()
	keys, ok := s.tags[tag]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.tags, tag)
	s.mu.Unlock()

	for k := range keys {
		_ = s.backend.Delete(ctx, k)
	}
	return nil
}
