package links

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/octarq-org/octarq/plugin"
)

// Mount with a fully wired plugin.Context: every seam the plugin consumes is
// present, the huma routes are registered into a live test API, and the root
// short-link handler is served end to end.
func TestMountWiresEverythingAndServesRoot(t *testing.T) {
	seed, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, seed)
	db := seed.db

	p := New()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("links test", "1.0.0"))

	var webhooks, tasks, provides []string
	var root http.Handler
	ctx := &plugin.Context{
		DB:     db,
		UserID: func(*http.Request) uint { return 1 },
		OrgID: func(r *http.Request) uint {
			if v := r.Header.Get("X-Org-ID"); v != "" {
				var id uint
				fmt.Sscanf(v, "%d", &id)
				return id
			}
			return 1
		},
		Audit:                func(*http.Request, string, string, uint, map[string]any) {},
		GetGlobalSetting:     func(string) string { return "" },
		GetWorkspaceSetting:  func(uint, string) string { return "" },
		Enqueue:              func(context.Context, string, []byte) error { return nil },
		DeleteCache:          func(context.Context, string) error { return nil },
		PublishEvent:         func(uint, string, any) {},
		RequireRole:          func(*http.Request, string) bool { return true },
		RegisterWebhookEvent: func(def plugin.WebhookEventDef) { webhooks = append(webhooks, def.Key) },
		RegisterTask:         func(name string, _ func(context.Context, []byte) error) { tasks = append(tasks, name) },
		Huma:                 api,
		Provide:              func(name string, _ any) { provides = append(provides, name) },
		HandleRoot:           func(h http.Handler) { root = h },
	}
	p.Mount(nil, ctx)

	for _, want := range []string{"link.create", "link.click", "link.delete"} {
		if !slicesContains(webhooks, want) {
			t.Errorf("webhook event %s not registered; got %v", want, webhooks)
		}
	}
	if len(tasks) != 1 || tasks[0] != "link.crawl" {
		t.Errorf("tasks = %v, want [link.crawl]", tasks)
	}
	for _, want := range []string{"links.overview", "links.purge", "links.export", "links.resolve", "links.create", "links.cleanup", "links.mcp_export", "links.trust_proxy"} {
		if !slicesContains(provides, want) {
			t.Errorf("service %s not provided; got %v", want, provides)
		}
	}

	// The registered API routes dispatch through the plugin's handlers.
	if err := db.Create(&Link{OrgID: 1, Slug: "e2e", Target: "https://e2e.example"}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.Header.Set("X-Org-ID", "1")
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /api/links -> %d, body %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "e2e") {
		t.Errorf("list response missing seeded link: %s", res.Body.String())
	}

	// Root handler: empty path 404s, unknown slug 404s, a real slug redirects.
	if root == nil {
		t.Fatal("root handler was not registered")
	}
	rec4 := httptest.NewRecorder()
	root.ServeHTTP(rec4, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec4.Code != http.StatusNotFound {
		t.Errorf("root / -> %d, want 404", rec4.Code)
	}
	rec2 := httptest.NewRecorder()
	root.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("root /missing -> %d, want 404", rec2.Code)
	}
	rec3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/e2e", nil)
	r3.Host = "app.example"
	root.ServeHTTP(rec3, r3)
	if rec3.Code != http.StatusFound || rec3.Header().Get("Location") != "https://e2e.example" {
		t.Errorf("root /e2e -> %d %q, want 302 https://e2e.example", rec3.Code, rec3.Header().Get("Location"))
	}
}

func TestDeleteLinkPublishesEvent(t *testing.T) {
	t.Parallel()
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	seedLink := &Link{OrgID: 1, Slug: "delpub", Target: "https://x"}
	if err := p.db.Create(seedLink).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	var gotEvent string
	var gotOrg uint
	p.publishEvent = func(orgID uint, event string, data any) {
		gotOrg = orgID
		gotEvent = event
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/links/1", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	rec := httptest.NewRecorder()
	hctx := humago.NewContext(nil, req, rec)
	out, err := p.deleteLink(ctx, &DeleteLinkInput{Ctx: hctx, ID: seedLink.ID})
	if err != nil || !out.Body["ok"] {
		t.Fatalf("deleteLink: %v", err)
	}
	if gotEvent != "link.delete" || gotOrg != 1 {
		t.Errorf("delete event = %q org %d, want link.delete org 1", gotEvent, gotOrg)
	}
}

func TestQuickCreateLinkHooks(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	ctx := context.Background()

	audited, published, enqueued, invalidated := 0, 0, 0, 0
	p.audit = func(*http.Request, string, string, uint, map[string]any) { audited++ }
	p.publishEvent = func(uint, string, any) { published++ }
	p.enqueue = func(context.Context, string, []byte) error { enqueued++; return nil }
	p.deleteCache = func(context.Context, string) error { invalidated++; return nil }

	req := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
	req.Header.Set("X-Org-ID", "1")
	out, err := p.quickCreateLink(ctx, &QuickCreateLinkInput{
		Ctx:  mkCtx(req),
		Body: QuickCreateLinkBody{URL: "https://hooked.example"},
	})
	if err != nil {
		t.Fatalf("quickCreateLink: %v", err)
	}
	if out.Body.Slug == "" {
		t.Error("expected a generated slug")
	}
	if audited != 1 || published != 1 || enqueued != 1 || invalidated != 1 {
		t.Errorf("hooks: audit=%d publish=%d enqueue=%d cache=%d, want all 1", audited, published, enqueued, invalidated)
	}
}

func TestRoutingRulesScan(t *testing.T) {
	var r RoutingRules
	if err := r.Scan(nil); err != nil || r != nil {
		t.Errorf("Scan(nil) = %v, %v; want nil, nil", r, err)
	}
	var fromBytes RoutingRules
	if err := fromBytes.Scan([]byte(`[{"type":"geo","match":"US","target":"https://us.example"}]`)); err != nil {
		t.Fatalf("Scan []byte: %v", err)
	}
	if len(fromBytes) != 1 || fromBytes[0].Type != "geo" || fromBytes[0].Target != "https://us.example" {
		t.Errorf("Scan []byte: %+v", fromBytes)
	}
	var fromString RoutingRules
	if err := fromString.Scan(`[]`); err != nil || len(fromString) != 0 {
		t.Errorf("Scan string '[]': %v %+v", err, fromString)
	}
	var fromEmpty RoutingRules
	if err := fromEmpty.Scan([]byte("")); err != nil || fromEmpty != nil {
		t.Errorf("Scan empty bytes: %v %+v", err, fromEmpty)
	}
	var fromInt RoutingRules
	if err := fromInt.Scan(42); err == nil {
		t.Error("Scan of an unsupported type must fail")
	}
}

func TestSplitAssignSkipsNonPositiveAndHaltsAtFullWeight(t *testing.T) {
	rules := RoutingRules{
		{Type: "split", Weight: -1, Target: "A"},
		{Type: "split", Weight: 200, Target: "B"},
	}
	for i := 0; i < 50; i++ {
		target, _, ok := splitAssign(rules, fmt.Sprintf("fp-%d", i), 1)
		if !ok || target != "B" {
			t.Fatalf("iteration %d: got %q ok=%v, want B (all buckets < 100, B owns the whole distribution)", i, target, ok)
		}
	}
}

func TestTruncateStringBranches(t *testing.T) {
	if got := truncateString("short", 10); got != "short" {
		t.Errorf("short string must pass through unchanged, got %q", got)
	}
	long := "héllo wörld"
	if got := truncateString(long, 6); got != "héllo " {
		t.Errorf("multi-byte truncation: got %q, want %q", got, "héllo ")
	}
	// len > max but rune count <= max: byte length alone is a poor measure.
	if got := truncateString(long, 20); got != long {
		t.Errorf("string with len>max but few runes must pass through, got %q", got)
	}
}

func TestClientIPFallsBackToXRealIPWhenXFFBlank(t *testing.T) {
	SetTrustProxy(true)
	defer SetTrustProxy(false)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.9:1234"
	r.Header.Set("X-Forwarded-For", "   ")
	r.Header.Set("X-Real-IP", "8.8.4.4")
	if got := clientIP(r); got != "8.8.4.4" {
		t.Errorf("blank XFF must fall through to X-Real-IP, got %q", got)
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	l := newIPRateLimiter(1, time.Minute)
	if !l.Allow("1.1.1.1") {
		t.Fatal("first request must be allowed")
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("second request must be rate limited")
	}
	// Force the window to have lapsed; the counter must reset.
	l.mu.Lock()
	l.resets["1.1.1.1"] = time.Now().Add(-time.Minute)
	l.mu.Unlock()
	if !l.Allow("1.1.1.1") {
		t.Error("request after the window must be allowed again")
	}
	if l.Allow("1.1.1.1") {
		t.Error("request right after the reset must be rate limited again")
	}
}

func TestOwnsHostAndLinkHostRequiredZeroOrg(t *testing.T) {
	p := &Plugin{}
	if p.ownsHost(0, "any.example") {
		t.Error("ownsHost with org 0 must be false")
	}
	if p.linkHostRequired(0) {
		t.Error("linkHostRequired with org 0 must be false")
	}

}
