package links

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&Link{}, &LinkEvent{}); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}
	return db
}

func TestUTMCaptureAndTruncation(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db, nil)
	defer engine.Close()

	link := Link{
		OrgID:   1,
		Slug:    "utm-test",
		Target:  "https://example.com",
		Enabled: true,
	}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// 1. UTM capture
	req1 := httptest.NewRequest("GET", "/utm-test?utm_source=twitter&utm_medium=cpc&utm_campaign=summer_sale", nil)
	w1 := httptest.NewRecorder()
	engine.Handle(w1, req1, &link)

	// 2. Super long UTM values (500 chars)
	longSource := strings.Repeat("a", 500)
	req2 := httptest.NewRequest("GET", "/utm-test?utm_source="+longSource, nil)
	w2 := httptest.NewRecorder()
	engine.Handle(w2, req2, &link)

	// Flush queue
	engine.Close()

	var events []LinkEvent
	if err := db.Where("link_id = ?", link.ID).Order("id asc").Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify req1 UTM parameters
	if events[0].UTMSource != "twitter" || events[0].UTMMedium != "cpc" || events[0].UTMCampaign != "summer_sale" {
		t.Errorf("unexpected UTM values on event 0: source=%q, medium=%q, campaign=%q",
			events[0].UTMSource, events[0].UTMMedium, events[0].UTMCampaign)
	}

	// Verify req2 truncation to 128 chars
	if len(events[1].UTMSource) != 128 || events[1].UTMSource != strings.Repeat("a", 128) {
		t.Errorf("expected UTMSource to be truncated to 128 chars, got length %d", len(events[1].UTMSource))
	}
}

func TestClassifyReferer(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", "Direct"},
		{"   ", "Direct"},
		{"https://t.co/abc", "Twitter"},
		{"https://twitter.com/i/web/1", "Twitter"},
		{"https://x.com/status/123", "Twitter"},
		{"https://blog.example.com/p", "example.com"},
		{"https://shop.example.com/p", "example.com"},
		{"https://google.com/search?q=test", "Google"},
		{"https://www.google.co.jp", "Google"},
		{"https://news.ycombinator.com/", "Hacker News"},
	}

	for _, c := range cases {
		got := classifyReferer(c.raw)
		if got != c.want {
			t.Errorf("classifyReferer(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestUVExcludesEmptyFingerprint(t *testing.T) {
	db := setupTestDB(t)
	p := New()
	p.db = db
	p.auth.OrgID = func(r *http.Request) uint { return 1 }

	link := Link{
		ID:      101,
		OrgID:   1,
		Slug:    "uv-test",
		Target:  "https://example.com",
		Enabled: true,
	}
	db.Create(&link)

	now := time.Now()
	// 5 events with empty fingerprint
	for i := 0; i < 5; i++ {
		db.Create(&LinkEvent{
			LinkID:      link.ID,
			CreatedAt:   now,
			Device:      "Desktop",
			Browser:     "Chrome",
			Fingerprint: "",
		})
	}
	// 2 events with distinct fingerprints
	db.Create(&LinkEvent{
		LinkID:      link.ID,
		CreatedAt:   now,
		Device:      "Desktop",
		Browser:     "Chrome",
		Fingerprint: "fp_user_1",
	})
	db.Create(&LinkEvent{
		LinkID:      link.ID,
		CreatedAt:   now,
		Device:      "Desktop",
		Browser:     "Chrome",
		Fingerprint: "fp_user_2",
	})

	r := httptest.NewRequest("GET", "/api/links/101/stats?metric=uv", nil)
	r.Header.Set("X-Org-ID", "1")
	ctx := humago.NewContext(nil, r, httptest.NewRecorder())

	input := &LinkStatsInput{
		Ctx:    ctx,
		ID:     101,
		Days:   30,
		Metric: "uv",
	}

	out, err := p.linkStats(context.Background(), input)
	if err != nil {
		t.Fatalf("linkStats failed: %v", err)
	}

	devices, ok := out.Body["devices"].([]models.StatKV)
	if !ok || len(devices) == 0 {
		t.Fatalf("expected devices statkv slice in body")
	}

	// UV metric must exclude empty fingerprint, so count should be 2 (fp_user_1 and fp_user_2)
	if devices[0].Count != 2 {
		t.Fatalf("expected UV count = 2 excluding empty fingerprints, got %d", devices[0].Count)
	}
}

func TestPVVsUVMetrics(t *testing.T) {
	db := setupTestDB(t)
	p := New()
	p.db = db
	p.auth.OrgID = func(r *http.Request) uint { return 1 }

	link := Link{
		ID:      102,
		OrgID:   1,
		Slug:    "pv-uv-test",
		Target:  "https://example.com",
		Enabled: true,
	}
	db.Create(&link)

	now := time.Now()
	// 3 clicks from user 1, 2 clicks from user 2
	for i := 0; i < 3; i++ {
		db.Create(&LinkEvent{
			LinkID:      link.ID,
			CreatedAt:   now,
			Browser:     "Firefox",
			Fingerprint: "fp_user_1",
		})
	}
	for i := 0; i < 2; i++ {
		db.Create(&LinkEvent{
			LinkID:      link.ID,
			CreatedAt:   now,
			Browser:     "Firefox",
			Fingerprint: "fp_user_2",
		})
	}

	// Test PV
	rPV := httptest.NewRequest("GET", "/api/links/102/stats?metric=pv", nil)
	rPV.Header.Set("X-Org-ID", "1")
	ctxPV := humago.NewContext(nil, rPV, httptest.NewRecorder())
	outPV, err := p.linkStats(context.Background(), &LinkStatsInput{Ctx: ctxPV, ID: 102, Days: 30, Metric: "pv"})
	if err != nil {
		t.Fatalf("PV stats failed: %v", err)
	}
	browsersPV := outPV.Body["browsers"].([]models.StatKV)
	if len(browsersPV) == 0 || browsersPV[0].Count != 5 {
		t.Fatalf("expected PV count = 5, got %v", browsersPV)
	}

	// Test UV
	rUV := httptest.NewRequest("GET", "/api/links/102/stats?metric=uv", nil)
	rUV.Header.Set("X-Org-ID", "1")
	ctxUV := humago.NewContext(nil, rUV, httptest.NewRecorder())
	outUV, err := p.linkStats(context.Background(), &LinkStatsInput{Ctx: ctxUV, ID: 102, Days: 30, Metric: "uv"})
	if err != nil {
		t.Fatalf("UV stats failed: %v", err)
	}
	browsersUV := outUV.Body["browsers"].([]models.StatKV)
	if len(browsersUV) == 0 || browsersUV[0].Count != 2 {
		t.Fatalf("expected UV count = 2, got %v", browsersUV)
	}
}
