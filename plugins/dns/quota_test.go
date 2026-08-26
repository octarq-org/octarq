package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/plugin"
)

type fakeQuotaChecker struct {
	err error
}

func (f fakeQuotaChecker) Check(_ context.Context, _ uint, _ string, _ int64) error {
	return f.err
}

// withQuotaChecker swaps in a plugin.Context whose Lookup serves a QuotaChecker
// always returning err; pass nil as checker to register no checker at all.
func withQuotaChecker(p *Plugin, checker plugin.QuotaChecker) {
	p.ctx = &plugin.Context{
		Lookup: func(name string) (any, bool) {
			if name == plugin.ServiceQuotaChecker && checker != nil {
				return checker, true
			}
			return nil, false
		},
	}
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
		return
	}
	se, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	return se.GetStatus()
}

func seedProviderAccount(t *testing.T, p *Plugin) uint {
	t.Helper()
	acc := ProviderAccount{OrgID: 1, Name: "Test Provider", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}
	return acc.ID
}

// No checker registered (self-hosted) must behave exactly as before the seam
// existed: domain creation just works.
func TestCreateDomainNoQuotaCheckerCreates(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullDNSTestDB(t)
	accID := seedProviderAccount(t, p)

	out, err := p.createDomain(context.Background(), &CreateDomainInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/domains", nil)),
		Body: domainDTO{
			Name:              "no-quota.example",
			ProviderAccountID: accID,
		},
	})
	if err != nil {
		t.Fatalf("createDomain without checker: %v", err)
	}
	if out == nil || out.Body.ID == 0 {
		t.Fatal("expected a created domain, got zero output")
	}
	var n int64
	p.db.Model(&Domain{}).Where("owner_id = ?", 1).Count(&n)
	if n != 1 {
		t.Errorf("expected 1 domain row, got %d", n)
	}
}

// Exceeded quota → 429, and crucially no row is written (the refusal must not
// leave a half-created domain behind).
func TestCreateDomainQuotaExceededBlocksAndWritesNothing(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullDNSTestDB(t)
	accID := seedProviderAccount(t, p)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaExceeded})

	_, err := p.createDomain(context.Background(), &CreateDomainInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/domains", nil)),
		Body: domainDTO{
			Name:              "blocked.example",
			ProviderAccountID: accID,
		},
	})
	if got := statusOf(t, err); got != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d (%v)", got, err)
	}
	var n int64
	p.db.Model(&Domain{}).Where("owner_id = ?", 1).Count(&n)
	if n != 0 {
		t.Errorf("blocked createDomain still wrote %d rows; want 0", n)
	}
}

// Unavailable (plan lacks the capability) → 402, not 429. The two must stay
// distinct: one is "used up", the other is "upgrade to get this".
func TestCreateDomainQuotaUnavailableIs402(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullDNSTestDB(t)
	accID := seedProviderAccount(t, p)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaUnavailable})

	_, err := p.createDomain(context.Background(), &CreateDomainInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/domains", nil)),
		Body: domainDTO{
			Name:              "upgrade-me.example",
			ProviderAccountID: accID,
		},
	})
	if got := statusOf(t, err); got != http.StatusPaymentRequired {
		t.Fatalf("want 402, got %d (%v)", got, err)
	}
	var n int64
	p.db.Model(&Domain{}).Where("owner_id = ?", 1).Count(&n)
	if n != 0 {
		t.Errorf("402 refusal still wrote %d rows; want 0", n)
	}
}

// A host whose Context.Lookup is nil (old host / MCP composition) must not
// panic — it must read as "no checker".
func TestCreateDomainNilLookupNoPanic(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullDNSTestDB(t)
	accID := seedProviderAccount(t, p)
	p.ctx = &plugin.Context{}

	out, err := p.createDomain(context.Background(), &CreateDomainInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/domains", nil)),
		Body: domainDTO{
			Name:              "nil-lookup.example",
			ProviderAccountID: accID,
		},
	})
	if err != nil {
		t.Fatalf("nil Lookup must pass, got %v", err)
	}
	if out == nil || out.Body.ID == 0 {
		t.Fatal("expected a created domain")
	}
}
