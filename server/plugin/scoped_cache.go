package plugin

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TieredCache implements the ScopedCache interface with an L1 local cache
// and an optional L2 distributed cache, with automatic plugin key prefixing.
type TieredCache struct {
	prefix string
	l1     ScopedCache
	l2     ScopedCache
}

// Ensure TieredCache implements ScopedCache.
var _ ScopedCache = (*TieredCache)(nil)

// TwoLevelCache is an alias for TieredCache.
type TwoLevelCache = TieredCache

// NewScopedCache creates a new plugin-scoped two-level cache.
// prefix is the plugin name used to namespace all keys.
// l1 is the local/in-memory cache layer; l2 is the optional remote/distributed cache layer.
func NewScopedCache(pluginName string, l1, l2 ScopedCache) *TieredCache {
	return &TieredCache{
		prefix: pluginName,
		l1:     l1,
		l2:     l2,
	}
}

// NewTenantScopedCache creates a new tenant-and-plugin scoped two-level cache.
// Keys are namespaced under org:<orgID>:<pluginName> to ensure strict tenant isolation.
func NewTenantScopedCache(orgID uint, pluginName string, l1, l2 ScopedCache) *TieredCache {
	prefix := pluginName
	if orgID > 0 {
		if pluginName != "" {
			prefix = fmt.Sprintf("org:%d:%s", orgID, pluginName)
		} else {
			prefix = fmt.Sprintf("org:%d", orgID)
		}
	}
	return NewScopedCache(prefix, l1, l2)
}

// ForOrg returns a tenant-scoped view of this TieredCache with keys prefixed by org:<orgID>:<prefix>.
func (s *TieredCache) ForOrg(orgID uint) *TieredCache {
	return NewTenantScopedCache(orgID, s.prefix, s.l1, s.l2)
}

func (s *TieredCache) key(ctx context.Context, k string) string {
	base := k
	if s.prefix != "" {
		base = s.prefix + ":" + k
	}
	if ctx != nil && !strings.HasPrefix(s.prefix, "org:") && !strings.HasPrefix(k, "org:") {
		if orgID := OrgIDFromContext(ctx); orgID > 0 {
			return fmt.Sprintf("org:%d:%s", orgID, base)
		}
	}
	return base
}

// Key returns the fully namespaced cache key for the given key string (with no tenant context).
func (s *TieredCache) Key(k string) string {
	return s.key(context.Background(), k)
}

// KeyForOrg returns the fully namespaced cache key for the given orgID and key string.
func (s *TieredCache) KeyForOrg(orgID uint, k string) string {
	ctx := WithOrgID(context.Background(), orgID)
	return s.key(ctx, k)
}

// Prefix returns the plugin namespace prefix.
func (s *TieredCache) Prefix() string {
	return s.prefix
}

// Get retrieves a key, first checking L1. If missed and L2 is present,
// it checks L2 and backfills L1 upon hit.
func (s *TieredCache) Get(ctx context.Context, k string, dest any) (bool, error) {
	fullKey := s.key(ctx, k)
	if s.l1 != nil {
		found, err := s.l1.Get(ctx, fullKey, dest)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	if s.l2 != nil {
		found, err := s.l2.Get(ctx, fullKey, dest)
		if err != nil {
			return false, err
		}
		if found {
			// Backfill L1 with a safety TTL (1 minute) to avoid permanent stale cache
			if s.l1 != nil {
				_ = s.l1.Set(ctx, fullKey, dest, time.Minute)
			}
			return true, nil
		}
	}
	return false, nil
}

// Set writes through to both L1 and L2.
func (s *TieredCache) Set(ctx context.Context, k string, val any, ttl time.Duration) error {
	fullKey := s.key(ctx, k)
	if s.l1 != nil {
		if err := s.l1.Set(ctx, fullKey, val, ttl); err != nil {
			return err
		}
	}
	if s.l2 != nil {
		if err := s.l2.Set(ctx, fullKey, val, ttl); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the key from both L1 and L2.
func (s *TieredCache) Delete(ctx context.Context, k string) error {
	fullKey := s.key(ctx, k)
	var err1, err2 error
	if s.l1 != nil {
		err1 = s.l1.Delete(ctx, fullKey)
	}
	if s.l2 != nil {
		err2 = s.l2.Delete(ctx, fullKey)
	}
	if err1 != nil {
		return err1
	}
	return err2
}

// InvalidateTag invalidates the tag in both L1 and L2.
func (s *TieredCache) InvalidateTag(ctx context.Context, tag string) error {
	fullTag := s.key(ctx, tag)
	var err1, err2 error
	if s.l1 != nil {
		err1 = s.l1.InvalidateTag(ctx, fullTag)
	}
	if s.l2 != nil {
		err2 = s.l2.InvalidateTag(ctx, fullTag)
	}
	if err1 != nil {
		return err1
	}
	return err2
}
