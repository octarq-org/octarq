package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/ulule/limiter/v3"
)

// postLoginFrom sends a sign-in attempt from one client address. The proxy
// header is what the limiter reads because the test config trusts proxies, so
// each call can present a different address without a real socket.
func postLoginFrom(t *testing.T, srv http.Handler, ip, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// An attacker with a pool of addresses spends a fresh IP budget on every
// account guess, so the per-IP budget alone never stops a run against one
// account. The account budget is what ends it.
func TestLoginLimiterCountsFailuresForTheAccountAcrossAddresses(t *testing.T) {
	_, srv, _ := newTestHandlerRawCfg(t, &config.Config{
		AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret", TrustProxy: true,
	})

	victim := "victim@example.com"
	// Two addresses that each stay inside the 5-failures-per-IP budget fill the
	// 10-failures-per-account one.
	for _, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		for n := 1; n <= 5; n++ {
			rec := postLoginFrom(t, srv, ip, victim, "wrong-password")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("attempt %d from %s: want 401, got %d: %s", n, ip, rec.Code, rec.Body.String())
			}
		}
	}

	// A third address has spent none of its own budget and is refused only
	// because the account's budget is exhausted.
	rec := postLoginFrom(t, srv, "198.51.100.3", victim, "wrong-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 for the exhausted account, got %d: %s", rec.Code, rec.Body.String())
	}

	// The refusal follows the account, not the address: another account from
	// the same third address is still served.
	rec = postLoginFrom(t, srv, "198.51.100.3", "other@example.com", "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for a different account, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A blank email must not become a shared bucket: every blank-email attempt
// would then lock out every other one, which is a lockout primitive rather
// than a credential-stuffing defence.
func TestLoginAllowedIgnoresABlankAccountKey(t *testing.T) {
	h := &Handler{
		loginLimiter:        mustRateLimiter(t, "", "test-ip", 5, time.Minute),
		loginAccountLimiter: &rateLimiter{store: failingStore{}, rate: limiter.Rate{Limit: 10, Period: time.Minute}},
	}
	if !h.loginAllowed("198.51.100.7", "") {
		t.Fatal("a blank account key must be checked on the IP budget only")
	}
}

// failingStore stands in for a counter store that is configured but cannot be
// read, the state a broken Redis leaves behind.
type failingStore struct{}

var errStoreDown = errors.New("counter store is down")

func (failingStore) Get(context.Context, string, limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errStoreDown
}
func (failingStore) Peek(context.Context, string, limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errStoreDown
}
func (failingStore) Reset(context.Context, string, limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errStoreDown
}
func (failingStore) Increment(context.Context, string, int64, limiter.Rate) (limiter.Context, error) {
	return limiter.Context{}, errStoreDown
}

// A budget that cannot be read is not a budget the request has proven itself
// to be inside. Allowing on a store error would turn a counter outage into an
// unlimited window for credential stuffing.
func TestLoginLimiterRefusesWhenTheCounterStoreFails(t *testing.T) {
	rl := &rateLimiter{store: failingStore{}, rate: limiter.Rate{Limit: 5, Period: time.Minute}}
	if rl.allow("198.51.100.9") {
		t.Fatal("a store error must refuse the request, not allow it")
	}
	h := &Handler{loginLimiter: rl, loginAccountLimiter: rl}
	if h.loginAllowed("198.51.100.9", "victim@example.com") {
		t.Fatal("loginAllowed must refuse when the budget cannot be read")
	}
	rl.recordFailure("198.51.100.9") // must not panic on the way out
}
