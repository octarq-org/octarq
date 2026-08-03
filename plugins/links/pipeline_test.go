package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	dns "github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func newTestPipelineEngine(t *testing.T) (*Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&Link{})
	db.Where("1 = 1").Delete(&LinkEvent{})

	eng := NewEngine(db, mockCtx())
	return eng, db
}

// 1. Batching Test: N clicks produce write transactions far less than N.
func TestClickPipelineBatching(t *testing.T) {
	eng, db := newTestPipelineEngine(t)
	defer eng.Close()

	link := &Link{Slug: "batchtest", Target: "https://example.com", Enabled: true, OrgID: 1}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	const nClicks = 50
	var wg sync.WaitGroup
	wg.Add(nClicks)

	for i := 0; i < nClicks; i++ {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/batchtest", nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
			eng.Handle(rec, req, link)
			if rec.Code != http.StatusFound {
				t.Errorf("expected 302 redirect, got %d", rec.Code)
			}
		}()
	}

	wg.Wait()
	eng.Close()

	txCount := eng.TxCount()
	if txCount >= nClicks {
		t.Fatalf("expected write transactions (%d) to be far less than N (%d)", txCount, nClicks)
	}
	if txCount == 0 {
		t.Fatalf("expected at least 1 write transaction, got 0")
	}

	var totalEvents int64
	db.Model(&LinkEvent{}).Where("link_id = ?", link.ID).Count(&totalEvents)
	if totalEvents != nClicks {
		t.Errorf("expected %d total events in DB, got %d", nClicks, totalEvents)
	}

	var updatedLink Link
	db.First(&updatedLink, link.ID)
	if updatedLink.Clicks != nClicks {
		t.Errorf("expected link.clicks to be %d, got %d", nClicks, updatedLink.Clicks)
	}
}

// 2. Bounded Queue Dropping Test: filling queue increments drop counter without blocking.
func TestClickPipelineBoundedQueueDropping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create engine with tiny queue capacity for test
	eng := &Engine{
		db:    db,
		ctx:   mockCtx(),
		queue: make(chan clickItem, 5),
	}
	// Do NOT start worker so queue stays full
	// Push 10 events into queue of size 5
	start := time.Now()
	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/droptest", nil)
		eng.record(req, 1, "droptest", 1, "1.2.3.4", "", "", "", "UA", "", "", "", false)
		if rec.Code != http.StatusOK && rec.Code != 0 {
			t.Errorf("unexpected status: %d", rec.Code)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("record calls blocked when queue was full, took %v", elapsed)
	}

	if eng.DropCount() == 0 {
		t.Errorf("expected drop count > 0 when queue overflowed, got %d", eng.DropCount())
	}
}

// 3. Shutdown Flush Test: triggering Close flushes all queued items.
func TestClickPipelineShutdownFlush(t *testing.T) {
	eng, db := newTestPipelineEngine(t)

	link := &Link{Slug: "flushtest", Target: "https://example.com", Enabled: true, OrgID: 1}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	const nEvents = 25
	for i := 0; i < nEvents; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/flushtest", nil)
		eng.Handle(rec, req, link)
	}

	// Trigger shutdown flush
	eng.Close()

	var totalEvents int64
	db.Model(&LinkEvent{}).Where("link_id = ?", link.ID).Count(&totalEvents)
	if totalEvents != nEvents {
		t.Errorf("expected all %d events to be flushed to DB on close, got %d", nEvents, totalEvents)
	}
}

// 4. Rate Limit Test: exceeding threshold still returns 302 redirect but drops analytics event.
func TestClickPipelineRateLimiting(t *testing.T) {
	eng, db := newTestPipelineEngine(t)
	defer eng.Close()

	eng.SetRateLimit(3, time.Minute)

	link := &Link{Slug: "ratetest", Target: "https://example.com", Enabled: true, OrgID: 1}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	const totalRequests = 8
	for i := 0; i < totalRequests; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ratetest", nil)
		req.RemoteAddr = "203.0.113.195:12345"
		eng.Handle(rec, req, link)

		if rec.Code != http.StatusFound {
			t.Errorf("request %d: expected status 302 Found even when rate limited, got %d", i+1, rec.Code)
		}
		if rec.Header().Get("Location") != "https://example.com" {
			t.Errorf("request %d: expected Location https://example.com, got %q", i+1, rec.Header().Get("Location"))
		}
	}

	eng.Close()

	var totalEvents int64
	db.Model(&LinkEvent{}).Where("link_id = ?", link.ID).Count(&totalEvents)
	if totalEvents != 3 {
		t.Errorf("expected exactly 3 events recorded up to rate limit, got %d", totalEvents)
	}
}

// 5. Minimal Endpoint Ownership Validation Test: host belonging to another org returns 403.
func TestQuickCreateLinkHostOwnership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create domain owned by Org 2
	dom2 := dns.Domain{
		OrgID:   2,
		Name:    "org2domain.com",
		ForLink: true,
	}
	db.Create(&dom2)

	p := New()
	p.db = db
	p.auth.OrgID = func(r *http.Request) uint { return 1 } // Context belongs to Org 1

	// Case A: Org 1 tries to create link targeting Org 2's host -> MUST BE 403 Forbidden
	reqA := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
	inputA := &QuickCreateLinkInput{
		Ctx: humago.NewContext(nil, reqA, httptest.NewRecorder()),
		Body: QuickCreateLinkBody{
			URL:  "https://example.com/target",
			Host: "org2domain.com",
		},
	}
	_, errA := p.quickCreateLink(context.Background(), inputA)
	if errA == nil {
		t.Fatalf("unowned host creation: expected 403 error, got nil")
	}
	if statusErr, ok := errA.(huma.StatusError); !ok || statusErr.GetStatus() != http.StatusForbidden {
		t.Errorf("unowned host creation: expected 403 Forbidden, got %v", errA)
	}

	// Case B: Org 1 creates valid link with default/owned host -> 201 Created
	reqB := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
	inputB := &QuickCreateLinkInput{
		Ctx: humago.NewContext(nil, reqB, httptest.NewRecorder()),
		Body: QuickCreateLinkBody{
			URL: "https://example.com/target",
		},
	}
	outB, errB := p.quickCreateLink(context.Background(), inputB)
	if errB != nil {
		t.Fatalf("valid quick link creation: expected success, got %v", errB)
	}
	if outB.Body.Target != "https://example.com/target" {
		t.Errorf("expected target https://example.com/target, got %q", outB.Body.Target)
	}
}
