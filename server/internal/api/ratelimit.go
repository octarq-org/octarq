package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// newRateLimiter builds the counter store for one budget. Without a Redis URL
// the counters live in this process. A configured Redis server that cannot be
// reached returns an error: falling back to memory would multiply every budget
// by the number of running instances, and the deployment would never learn that
// its shared counters are gone.
func newRateLimiter(redisURL string, prefix string, limit int, window time.Duration) (*rateLimiter, error) {
	rate := limiter.Rate{
		Limit:  int64(limit),
		Period: window,
	}

	if redisURL == "" {
		return &rateLimiter{store: memory.NewStore(), rate: rate}, nil
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse the Redis URL for the %q limiter: %w", prefix, err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("reach the Redis server for the %q limiter: %w", prefix, err)
	}
	store, err := redisstore.NewStoreWithOptions(client, limiter.StoreOptions{
		Prefix: "octarq:limit:" + prefix + ":",
	})
	if err != nil {
		return nil, fmt.Errorf("build the %q limiter store: %w", prefix, err)
	}
	return &rateLimiter{store: store, rate: rate}, nil
}

func (rl *rateLimiter) allow(ip string) bool {
	ctx := context.Background()
	limiterCtx, err := rl.store.Peek(ctx, ip, rl.rate)
	if err != nil {
		// Fail closed: a budget that cannot be read is not a budget the request
		// has shown itself to be within. Allowing on a store error turns any
		// counter outage into an unlimited window for credential stuffing.
		slog.Error("rate limiter store read failed; denying the request", "key", ip, "err", err)
		return false
	}
	return limiterCtx.Remaining > 0
}

func (rl *rateLimiter) recordFailure(ip string) {
	ctx := context.Background()
	if _, err := rl.store.Increment(ctx, ip, 1, rl.rate); err != nil {
		slog.Warn("rate limiter increment failure", "ip", ip, "err", err)
	}
}

func (rl *rateLimiter) reset(ip string) {
	ctx := context.Background()
	if _, err := rl.store.Reset(ctx, ip, rl.rate); err != nil {
		slog.Warn("rate limiter reset failure", "ip", ip, "err", err)
	}
}

// accountLoginKey normalises a sign-in email into a rate-limit key, so case and
// padding variants of one address share one budget. An empty result is never
// used as a key: it would put every blank-email attempt in one shared bucket,
// which is a lockout primitive rather than a credential-stuffing defence.
func accountLoginKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// loginAllowed reports whether a sign-in attempt is within budget for both the
// client IP and the account. A blank account key is checked on the IP budget
// only. Charging the same failure to both budgets is what stops an attacker
// with a pool of addresses: the IP budget caps one address, and the account
// budget follows the target across all of them.
func (h *Handler) loginAllowed(ip, account string) bool {
	if !h.loginLimiter.allow(ip) {
		return false
	}
	return account == "" || h.loginAccountLimiter.allow(account)
}

// recordLoginFailure charges one failed sign-in to both budgets.
func (h *Handler) recordLoginFailure(ip, account string) {
	h.loginLimiter.recordFailure(ip)
	if account != "" {
		h.loginAccountLimiter.recordFailure(account)
	}
}

// resetLoginFailures clears both budgets after a successful sign-in, so the
// account's next attempt starts from zero.
func (h *Handler) resetLoginFailures(ip, account string) {
	h.loginLimiter.reset(ip)
	if account != "" {
		h.loginAccountLimiter.reset(account)
	}
}
