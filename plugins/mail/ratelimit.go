package mail

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	redisstore "github.com/ulule/limiter/v3/drivers/store/redis"
)

type rateLimiter struct {
	store limiter.Store
	rate  limiter.Rate
}

func newRateLimiter(redisURL string, prefix string, limit int, window time.Duration) *rateLimiter {
	rate := limiter.Rate{
		Limit:  int64(limit),
		Period: window,
	}

	var store limiter.Store
	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err == nil {
			client := redis.NewClient(opts)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if client.Ping(ctx).Err() == nil {
				s, err := redisstore.NewStoreWithOptions(client, limiter.StoreOptions{
					Prefix: "octarq:limit:" + prefix + ":",
				})
				if err == nil {
					store = s
				}
			}
		}
	}

	if store == nil {
		store = memory.NewStore()
	}

	return &rateLimiter{
		store: store,
		rate:  rate,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	ctx := context.Background()
	limiterCtx, err := rl.store.Peek(ctx, ip, rl.rate)
	if err != nil {
		// On store failure, default to allow (fail-soft)
		return true
	}
	return limiterCtx.Remaining > 0
}

func (rl *rateLimiter) recordFailure(ip string) {
	ctx := context.Background()
	_, _ = rl.store.Increment(ctx, ip, 1, rl.rate)
}

func (rl *rateLimiter) reset(ip string) {
	ctx := context.Background()
	_, _ = rl.store.Reset(ctx, ip, rl.rate)
}
