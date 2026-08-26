package links

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

// No checker registered (self-hosted) must behave exactly as before the seam
// existed: creation just works.
func TestCreateLinkNoQuotaCheckerCreates(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)

	out, err := p.createLink(context.Background(), &CreateLinkInput{
		Ctx:  mkCtx(httptest.NewRequest(http.MethodPost, "/api/links", nil)),
		Body: linkDTO{Slug: "no-quota", Target: "https://example.com"},
	})
	if err != nil {
		t.Fatalf("createLink without checker: %v", err)
	}
	if out == nil || out.Body.ID == 0 {
		t.Fatal("expected a created link, got zero output")
	}
	var n int64
	p.db.Model(&Link{}).Where("owner_id = ?", 1).Count(&n)
	if n != 1 {
		t.Errorf("expected 1 link row, got %d", n)
	}
}

// Exceeded quota → 429, and crucially no row is written (the refusal must not
// leave a half-created link behind).
func TestCreateLinkQuotaExceededBlocksAndWritesNothing(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaExceeded})

	_, err := p.createLink(context.Background(), &CreateLinkInput{
		Ctx:  mkCtx(httptest.NewRequest(http.MethodPost, "/api/links", nil)),
		Body: linkDTO{Slug: "blocked", Target: "https://example.com"},
	})
	if got := statusOf(t, err); got != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d (%v)", got, err)
	}
	var n int64
	p.db.Model(&Link{}).Where("owner_id = ?", 1).Count(&n)
	if n != 0 {
		t.Errorf("blocked createLink still wrote %d rows; want 0", n)
	}
}

// Unavailable (plan lacks the capability) → 402, not 429. The two must stay
// distinct: one is "used up", the other is "upgrade to get this".
func TestCreateLinkQuotaUnavailableIs402(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaUnavailable})

	_, err := p.createLink(context.Background(), &CreateLinkInput{
		Ctx:  mkCtx(httptest.NewRequest(http.MethodPost, "/api/links", nil)),
		Body: linkDTO{Slug: "upgrade-me", Target: "https://example.com"},
	})
	if got := statusOf(t, err); got != http.StatusPaymentRequired {
		t.Fatalf("want 402, got %d (%v)", got, err)
	}
	var n int64
	p.db.Model(&Link{}).Where("owner_id = ?", 1).Count(&n)
	if n != 0 {
		t.Errorf("402 refusal still wrote %d rows; want 0", n)
	}
}

// quickCreateLink is a second door into the same table; leaving it ungated
// would make the createLink gate cosmetic.
func TestQuickCreateLinkQuotaExceededBlocks(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaExceeded})

	_, err := p.quickCreateLink(context.Background(), &QuickCreateLinkInput{
		Ctx:  mkCtx(httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)),
		Body: QuickCreateLinkBody{URL: "https://example.com"},
	})
	if got := statusOf(t, err); got != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d (%v)", got, err)
	}
	var n int64
	p.db.Model(&Link{}).Where("owner_id = ?", 1).Count(&n)
	if n != 0 {
		t.Errorf("blocked quickCreateLink still wrote %d rows; want 0", n)
	}
}

func TestQuickCreateLinkQuotaUnavailableIs402(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	withQuotaChecker(p, fakeQuotaChecker{err: plugin.ErrQuotaUnavailable})

	_, err := p.quickCreateLink(context.Background(), &QuickCreateLinkInput{
		Ctx:  mkCtx(httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)),
		Body: QuickCreateLinkBody{URL: "https://example.com"},
	})
	if got := statusOf(t, err); got != http.StatusPaymentRequired {
		t.Fatalf("want 402, got %d (%v)", got, err)
	}
}

// A host whose Context.Lookup is nil (old host / MCP composition) must not
// panic — it must read as "no checker".
func TestCreateLinkNilLookupNoPanic(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	p.ctx = &plugin.Context{}

	out, err := p.createLink(context.Background(), &CreateLinkInput{
		Ctx:  mkCtx(httptest.NewRequest(http.MethodPost, "/api/links", nil)),
		Body: linkDTO{Slug: "nil-lookup", Target: "https://example.com"},
	})
	if err != nil {
		t.Fatalf("nil Lookup must pass, got %v", err)
	}
	if out == nil || out.Body.ID == 0 {
		t.Fatal("expected a created link")
	}
}
