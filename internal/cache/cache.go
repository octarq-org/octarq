package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache defines the operations for our optional caching layer.
type Cache interface {
	Get(ctx context.Context, key string, dst any) bool
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	IsRedis() bool
}

// New returns a Cache implementation. If redisURL is empty, it returns a NoopCache.
// If redisURL is set but connection fails initially, it logs the error but still
// returns a valid RedisCache that gracefully falls back to noop-like behavior on failure.
func New(redisURL string) Cache {
	if redisURL == "" {
		return &NoopCache{}
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("cache: failed to parse redis URL %q: %v. Falling back to DB-only cache.", redisURL, err)
		return &NoopCache{}
	}

	client := redis.NewClient(opts)
	// Quick ping to check if Redis is up
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("cache: redis connection failed to %q: %v. Cache operations will fall back to DB.", redisURL, err)
	} else {
		log.Printf("cache: connected to Redis at %s", opts.Addr)
	}

	return &RedisCache{client: client}
}

// NoopCache represents a cache that is disabled, effectively acting as a bypass
// to GORM/DB operations.
type NoopCache struct{}

func (n *NoopCache) Get(ctx context.Context, key string, dst any) bool { return false }
func (n *NoopCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	return nil
}
func (n *NoopCache) Delete(ctx context.Context, key string) error { return nil }
func (n *NoopCache) IsRedis() bool                               { return false }

// RedisCache implements Cache interface using Redis client.
type RedisCache struct {
	client *redis.Client
}

func (rc *RedisCache) IsRedis() bool { return true }

// Get retrieves a key, deserializing it into dst. If Redis has network issues
// or key doesn't exist, it returns false (triggering GORM fallback).
func (rc *RedisCache) Get(ctx context.Context, key string, dst any) bool {
	val, err := rc.client.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Printf("cache: redis Get error on key %q (falling back to DB): %v", key, err)
		}
		return false
	}
	if err := json.Unmarshal([]byte(val), dst); err != nil {
		log.Printf("cache: unmarshal error on key %q: %v", key, err)
		return false
	}
	return true
}

// Set serializes val into JSON and stores it in Redis with the given TTL.
// Gracefully logs and ignores network errors to avoid crashing requests.
func (rc *RedisCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	err = rc.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		log.Printf("cache: redis Set error on key %q: %v", key, err)
	}
	return err
}

// Delete removes a key from Redis.
func (rc *RedisCache) Delete(ctx context.Context, key string) error {
	err := rc.client.Del(ctx, key).Err()
	if err != nil {
		log.Printf("cache: redis Delete error on key %q: %v", key, err)
	}
	return err
}
