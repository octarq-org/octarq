package links

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// perOrgQuotaChecker is a QuotaChecker that refuses only the orgs listed in
// over, letting every other org through — the shape needed to prove that one
// over-quota org in a batch cannot drag its batch-mates down.
type perOrgQuotaChecker struct {
	over map[uint]bool
}

func (c perOrgQuotaChecker) Check(_ context.Context, orgID uint, _ string, _ int64) error {
	if c.over[orgID] {
		return plugin.ErrQuotaExceeded
	}
	return nil
}

// usageRecorder captures RecordUsage calls so tests can assert both the metric
// name and that suppressed orgs are never metered.
type usageRecorder struct {
	calls []struct {
		orgID  uint
		metric string
		n      int64
	}
}

func (r *usageRecorder) record(orgID uint, metric string, n int64) {
	r.calls = append(r.calls, struct {
		orgID  uint
		metric string
		n      int64
	}{orgID, metric, n})
}

// newFlushEngine builds an Engine on an isolated DB whose ctx registers checker
// under plugin.ServiceQuotaChecker (pass nil for the no-checker / self-hosted
// shape) and captures RecordUsage. flushBatch is invoked directly, not over
// HTTP, so a test controls exactly which orgs and links a batch contains.
func newFlushEngine(t *testing.T, checker plugin.QuotaChecker) (*Engine, *usageRecorder) {
	t.Helper()
	p, _ := setupFullLinksTestDB(t)
	rec := &usageRecorder{}
	ctx := mockCtx()
	ctx.Lookup = func(name string) (any, bool) {
		if name == plugin.ServiceQuotaChecker && checker != nil {
			return checker, true
		}
		return nil, false
	}
	ctx.RecordUsage = rec.record
	return &Engine{db: p.db, ctx: ctx}, rec
}

// Self-hosted installs register no checker, so CheckQuota answers nil for every
// org. This must behave exactly as before the seam existed: everything counts.
func TestFlushBatchNoCheckerCountsEverything(t *testing.T) {
	eng, _ := newFlushEngine(t, nil)
	link := &Link{OrgID: 7, Slug: "selfhosted", Target: "https://example.com", Enabled: true}
	if err := eng.db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}
	now := time.Now()

	eng.flushBatch([]clickItem{
		{orgID: 7, linkID: link.ID, createdAt: now},
		{orgID: 7, linkID: link.ID, createdAt: now},
	})

	var events int64
	eng.db.Model(&LinkEvent{}).Where("link_id = ?", link.ID).Count(&events)
	if events != 2 {
		t.Errorf("events = %d, want 2 (no checker must keep counting)", events)
	}
	var l Link
	eng.db.First(&l, link.ID)
	if l.Clicks != 2 {
		t.Errorf("clicks = %d, want 2", l.Clicks)
	}
}

// An org whose checker says ErrQuotaExceeded must write nothing: no LinkEvent
// rows and no Link.clicks bump.
func TestFlushBatchQuotaExceededWritesNothing(t *testing.T) {
	eng, _ := newFlushEngine(t, fakeQuotaChecker{err: plugin.ErrQuotaExceeded})
	link := &Link{OrgID: 7, Slug: "capped", Target: "https://example.com", Enabled: true}
	if err := eng.db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	eng.flushBatch([]clickItem{
		{orgID: 7, linkID: link.ID, createdAt: time.Now()},
		{orgID: 7, linkID: link.ID, createdAt: time.Now()},
	})

	var events int64
	eng.db.Model(&LinkEvent{}).Where("link_id = ?", link.ID).Count(&events)
	if events != 0 {
		t.Errorf("events = %d, want 0 (over-quota org must not be counted)", events)
	}
	var l Link
	eng.db.First(&l, link.ID)
	if l.Clicks != 0 {
		t.Errorf("clicks = %d, want 0", l.Clicks)
	}
}

// The one case most worth getting right: two orgs share a batch and only one is
// over quota. The over-quota org must write nothing; the fine org's events,
// clicks, and metering must be completely unaffected.
func TestFlushBatchMixedOrgsIsolateSuppression(t *testing.T) {
	eng, rec := newFlushEngine(t, perOrgQuotaChecker{over: map[uint]bool{1: true}})
	overLink := &Link{OrgID: 1, Slug: "over", Target: "https://a", Enabled: true}
	okLink := &Link{OrgID: 2, Slug: "ok", Target: "https://b", Enabled: true}
	if err := eng.db.Create(&overLink).Error; err != nil {
		t.Fatalf("create over-quota link: %v", err)
	}
	if err := eng.db.Create(&okLink).Error; err != nil {
		t.Fatalf("create fine link: %v", err)
	}
	now := time.Now()

	eng.flushBatch([]clickItem{
		{orgID: 1, linkID: overLink.ID, createdAt: now},
		{orgID: 1, linkID: overLink.ID, createdAt: now},
		{orgID: 2, linkID: okLink.ID, createdAt: now},
		{orgID: 2, linkID: okLink.ID, createdAt: now},
	})

	var overEvents int64
	eng.db.Model(&LinkEvent{}).Where("link_id = ?", overLink.ID).Count(&overEvents)
	if overEvents != 0 {
		t.Errorf("over-quota org events = %d, want 0", overEvents)
	}
	var l1 Link
	eng.db.First(&l1, overLink.ID)
	if l1.Clicks != 0 {
		t.Errorf("over-quota org clicks = %d, want 0", l1.Clicks)
	}

	var okEvents int64
	eng.db.Model(&LinkEvent{}).Where("link_id = ?", okLink.ID).Count(&okEvents)
	if okEvents != 2 {
		t.Errorf("fine org events = %d, want 2 (suppression must not leak across orgs)", okEvents)
	}
	var l2 Link
	eng.db.First(&l2, okLink.ID)
	if l2.Clicks != 2 {
		t.Errorf("fine org clicks = %d, want 2", l2.Clicks)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("RecordUsage calls = %d, want 1 (only the fine org is metered)", len(rec.calls))
	}
	if rec.calls[0].orgID != 2 || rec.calls[0].metric != "clicks" || rec.calls[0].n != 2 {
		t.Errorf("unexpected RecordUsage call: %+v", rec.calls[0])
	}
}

// A checker that fails for an unrelated reason (billing backend down, unknown
// error) must read as "allowed": a metering outage can not become silent data
// loss. Only the explicit ErrQuotaExceeded suppresses.
func TestFlushBatchCheckerOtherErrorCountsEverything(t *testing.T) {
	eng, _ := newFlushEngine(t, fakeQuotaChecker{err: errors.New("billing backend down")})
	link := &Link{OrgID: 7, Slug: "still-counted", Target: "https://example.com", Enabled: true}
	if err := eng.db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	eng.flushBatch([]clickItem{
		{orgID: 7, linkID: link.ID, createdAt: time.Now()},
	})

	var events int64
	eng.db.Model(&LinkEvent{}).Where("link_id = ?", link.ID).Count(&events)
	if events != 1 {
		t.Errorf("events = %d, want 1 (non-quota error must pass through)", events)
	}
	var l Link
	eng.db.First(&l, link.ID)
	if l.Clicks != 1 {
		t.Errorf("clicks = %d, want 1", l.Clicks)
	}
}

// Click counting is metered as "clicks" — not "links", which is the stock-quota
// key for how many short links an org may hold.
func TestFlushBatchRecordsUsageAsClicks(t *testing.T) {
	eng, rec := newFlushEngine(t, nil)
	link := &Link{OrgID: 42, Slug: "usage", Target: "https://example.com", Enabled: true}
	if err := eng.db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	eng.flushBatch([]clickItem{
		{orgID: 42, linkID: link.ID, createdAt: time.Now()},
		{orgID: 42, linkID: link.ID, createdAt: time.Now()},
	})

	if len(rec.calls) != 1 {
		t.Fatalf("RecordUsage calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].orgID != 42 || rec.calls[0].metric != "clicks" || rec.calls[0].n != 2 {
		t.Errorf("unexpected RecordUsage call: %+v", rec.calls[0])
	}
}
