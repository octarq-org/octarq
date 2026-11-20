package plugin

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type TieredCache struct {
	prefix string
	l1     ScopedCache
	l2     ScopedCache
}

var _ ScopedCache = (*TieredCache)(nil)

type TwoLevelCache = TieredCache

func NewScopedCache(pluginName string, l1, l2 ScopedCache) *TieredCache {
	return &TieredCache{
		prefix: pluginName,
		l1:     l1,
		l2:     l2,
	}
}

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

func (s *TieredCache) Key(k string) string {
	return s.key(context.Background(), k)
}

func (s *TieredCache) KeyForOrg(orgID uint, k string) string {
	ctx := WithOrgID(context.Background(), orgID)
	return s.key(ctx, k)
}

func (s *TieredCache) Prefix() string {
	return s.prefix
}

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
			if s.l1 != nil {
				_ = s.l1.Set(ctx, fullKey, dest, time.Minute)
			}
			return true, nil
		}
	}
	return false, nil
}

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
