package api

import (
	"context"
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
