package plugin

import (
	"context"
	"time"
)

type ScopedCache interface {
	Get(ctx context.Context, key string, dest any) (bool, error)

	Set(ctx context.Context, key string, val any, ttl time.Duration) error

	Delete(ctx context.Context, key string) error

	InvalidateTag(ctx context.Context, tag string) error
}
