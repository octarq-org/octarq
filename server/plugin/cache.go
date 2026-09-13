package plugin

import (
	"context"
	"time"
)

// ScopedCache provides scoped caching for plugins.
// The host automatically prefixes cache keys with the plugin's namespace (and org scope where applicable)
// to prevent key collisions across plugins and tenants.
type ScopedCache interface {
	// Get retrieves a cached value by key and unmarshals it into dest (dest must be a pointer).
	// Returns (found bool, err error). When key is not found, found is false and err is nil.
	Get(ctx context.Context, key string, dest any) (bool, error)

	// Set stores a value in cache with a given TTL. If ttl <= 0, the value does not expire automatically.
	Set(ctx context.Context, key string, val any, ttl time.Duration) error

	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error

	// InvalidateTag invalidates all keys associated with the given tag.
	InvalidateTag(ctx context.Context, tag string) error
}
