package mail

import (
	"context"
	"log"
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

// allow reports whether the next send may proceed within the key's budget.
// A store error is returned to the caller instead of being swallowed: the
// budget guards the sending domain's reputation, so an unreadable counter must
// stop sending, and the caller can then say the check failed instead of
// claiming a limit the caller never reached.
func (rl *rateLimiter) allow(ip string) (bool, error) {
	ctx := context.Background()
	limiterCtx, err := rl.store.Peek(ctx, ip, rl.rate)
	if err != nil {
		log.Printf("mail: rate limit store read failed for %s: %v", ip, err)
		return false, err
	}
	return limiterCtx.Remaining > 0, nil
}

func (rl *rateLimiter) recordFailure(ip string) {
	ctx := context.Background()
	if _, err := rl.store.Increment(ctx, ip, 1, rl.rate); err != nil {
		log.Printf("mail: failed to increment rate limit failure count for ip %s: %v", ip, err)
	}
}
