package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/octarq-org/octarq/pkg/telemetry"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// Cache defines the operations for our caching layer.
type Cache interface {
	Get(ctx context.Context, key string, dst any) bool
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	IsRedis() bool
}

// New returns a Cache implementation. If redisURL is empty, it returns an in-memory Cache.
// If redisURL is set but connection fails initially, it logs the error and falls back to MemoryCache.
func New(redisURL string) Cache {
	if redisURL == "" {
		return NewMemoryCache(10000)
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("cache: failed to parse redis URL %q: %v. Falling back to memory cache.", redisURL, err)
		return NewMemoryCache(10000)
	}

	client := redis.NewClient(opts)
	_ = redisotel.InstrumentTracing(client)
	_ = redisotel.InstrumentMetrics(client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("cache: redis connection failed to %q: %v. Falling back to memory cache.", redisURL, err)
		return NewMemoryCache(10000)
	}
	log.Printf("cache: connected to Redis at %s", opts.Addr)

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
func (n *NoopCache) IsRedis() bool                                { return false }

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
		telemetry.Global().Metrics.RecordCacheMiss(ctx, "redis")
		if err != redis.Nil {
			log.Printf("cache: redis Get error on key %q (falling back to DB): %v", key, err)
		}
		return false
	}
	if err := json.Unmarshal([]byte(val), dst); err != nil {
		telemetry.Global().Metrics.RecordCacheMiss(ctx, "redis")
		log.Printf("cache: unmarshal error on key %q: %v", key, err)
		return false
	}
	telemetry.Global().Metrics.RecordCacheHit(ctx, "redis")
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
