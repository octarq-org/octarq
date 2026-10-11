package links

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
)

func TestTagsContain(t *testing.T) {
	if !tagsContain("a,b , c", "b") {
		t.Error("existing tag must match case-insensitively")
	}
	if tagsContain("a,b", "nope") {
		t.Error("absent tag must not match")
	}
	if !tagsContain("", "") {
		t.Error("empty tag filter matches everything")
	}
}

func TestTagBoundaryFilter(t *testing.T) {
	gdb := testDB(t)
	gdb.AutoMigrate(&Link{})
	links := []Link{
		{OrgID: 1, Slug: "exact", Tags: "test"},
		{OrgID: 1, Slug: "start", Tags: "test,beta"},
		{OrgID: 1, Slug: "end", Tags: "alpha,test"},
		{OrgID: 1, Slug: "mid", Tags: "alpha, test, beta"},
		{OrgID: 1, Slug: "spaces", Tags: "a,b , c"},
		{OrgID: 1, Slug: "case", Tags: "Promo"},
		{OrgID: 1, Slug: "wildcard_like", Tags: "te_t"},
		{OrgID: 1, Slug: "percent", Tags: "100%"},
		{OrgID: 1, Slug: "false_prefix", Tags: "testing,beta"},
		{OrgID: 1, Slug: "false_suffix", Tags: "alpha,attest"},
		{OrgID: 1, Slug: "false_mid", Tags: "alpha,detest,beta"},
	}
	for _, l := range links {
		gdb.Create(&l)
	}

	mustMatch := func(tag string, want ...string) {
		t.Helper()
		var matched []Link
		if err := filterByTag(gdb.Model(&Link{}).Where("owner_id = ?", 1), tag).Order("id ASC").Find(&matched).Error; err != nil {
			t.Fatalf("filterByTag(%q): %v", tag, err)
		}
		got := map[string]bool{}
		for _, m := range matched {
			got[m.Slug] = true
		}
		if len(matched) != len(want) {
			t.Fatalf("filterByTag(%q) matched %d %v, want %v", tag, len(matched), keys(got), want)
		}
		for _, slug := range want {
			if !got[slug] {
				t.Errorf("filterByTag(%q) missing %q (got %v)", tag, slug, keys(got))
			}
		}
	}

	mustMatch("test", "exact", "start", "end", "mid")
	mustMatch("TEST", "exact", "start", "end", "mid")
	mustMatch("b", "spaces")
	mustMatch("promo", "case")
	mustMatch("te_t", "wildcard_like")
	mustMatch("100%", "percent")
	mustMatch("_") // LIKE '_' must not match te_t
	mustMatch("%") // LIKE '%' must not match 100%
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSortStatKVOrdersByCountThenKey(t *testing.T) {
	rows := []models.StatKV{
		{Key: "b", Count: 2},
		{Key: "a", Count: 2},
		{Key: "c", Count: 5},
		{Key: "d", Count: 1},
	}
	sortStatKV(rows)
	var keys []string
	var counts []int64
	for _, r := range rows {
		keys = append(keys, r.Key)
		counts = append(counts, r.Count)
	}
	want := "c,a,b,d"
	if strings.Join(keys, ",") != want {
		t.Errorf("order = %s, want %s", strings.Join(keys, ","), want)
	}
	for i := 1; i < len(counts); i++ {
		if counts[i-1] < counts[i] {
			t.Errorf("counts not sorted descending: %v", counts)
		}
	}
}

// --- links.go helpers ---

func TestClassifyRefererExtraCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"not-a-url", "not-a-url"},
		{"://garbage", "Direct"},
		{"t.co", "Twitter"},
		{"https://mobile.twitter.com/u", "Twitter"},
		{"google.co.uk", "Google"},
		{"https://m.facebook.com/x", "Facebook"},
		{"https://instagram.com/p/1", "Facebook"},
		{"https://fb.me/x", "Facebook"},
		{"https://linkedin.com/in/x", "LinkedIn"},
		{"https://reddit.com/r/go", "Reddit"},
		{"https://github.com/octarq-org", "GitHub"},
		{"https://mp.weixin.qq.com/s/abc", "WeChat"},
		{"https://www.zhihu.com/question/1", "Zhihu"},
		{"u.zhihu.com", "Zhihu"},
		{"https://blog.example.com/path", "example.com"},
		{"example.com:8080/x", "example.com"},
		{"https://1.2.3.4/x", "1.2.3.4"},
	}
	for _, c := range cases {
		if got := classifyReferer(c.in); got != c.want {
			t.Errorf("classifyReferer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchRuleUnknownTypeIsFalse(t *testing.T) {
	if matchRule(RoutingRule{Type: "bogus", Match: "x"}, "x", "x", "x", "x") {
		t.Error("unknown rule type must never match")
	}
}

func TestTopChannels(t *testing.T) {
	t.Parallel()
	p, _ := setupFullLinksTestDB(t)
	wipeLinksTables(t, p)
	since := time.Now().Add(-time.Hour)

	p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: time.Now(), Referer: "https://google.com", Fingerprint: "fp-1"})
	p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: time.Now(), Referer: "https://google.com", Fingerprint: "fp-2"})
	p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: time.Now(), Referer: "https://facebook.com", Fingerprint: "fp-3"})
	p.db.Create(&LinkEvent{LinkID: 1, CreatedAt: time.Now().Add(-48 * time.Hour), Referer: "https://google.com", Fingerprint: "fp-4"})

	pv := p.topChannels(1, since, false)
	got := map[string]int64{}
	for _, r := range pv {
		got[r.Key] = r.Count
	}
	if got["Google"] != 2 || got["Facebook"] != 1 {
		t.Errorf("pv channel counts wrong: %v", got)
	}

	uv := p.topChannels(1, since, true)
	gotUV := map[string]int64{}
	for _, r := range uv {
		gotUV[r.Key] = r.Count
	}
	if gotUV["Google"] != 2 || gotUV["Facebook"] != 1 {
		t.Errorf("uv channel counts wrong: %v", gotUV)
	}

	empty := p.topChannels(999, since, false)
	if len(empty) != 0 {
		t.Errorf("expected empty result for unknown link, got %v", empty)
	}
}

// --- shortlink.go ---

func TestLookupCachePaths(t *testing.T) {
	db := newTestEngine(t).db

	cachedLink := &Link{ID: 9, Slug: "cached", Target: "https://cached.example", Enabled: true}
	ctx := &plugin.Context{
		CacheGet: func(_ context.Context, key string, val any) bool {
			if v, ok := val.(*Link); ok {
				*v = *cachedLink
			}
			return true
		},
		CacheSet: func(context.Context, string, any, time.Duration) error { return nil },
	}
	eng := NewEngine(db, ctx)
	defer eng.Close()

	got, ok := eng.Lookup("h", "cached")
	if !ok || got.Slug != "cached" {
		t.Errorf("cache hit did not resolve: ok=%v got=%+v", ok, got)
	}

	ctx.CacheGet = func(_ context.Context, key string, val any) bool {
		return true // leave val zero: the cached answer is "no such link"
	}
	if _, ok := eng.Lookup("h", "missing"); ok {
		t.Error("cached negative result must resolve to not-found")
	}
}

func TestHandleAttributeRoutingTakesPriorityOverSplit(t *testing.T) {
	eng := newTestEngine(t)
	t.Cleanup(eng.Close)

	link := &Link{
		OrgID:   1,
		Slug:    "route-priority",
		Target:  "https://default",
		Enabled: true,
		RoutingRules: RoutingRules{
			{Type: "geo", Match: "CN", Target: "https://china.example"},
			{Type: "split", Weight: 100, Target: "https://split.example"},
		},
	}
	if err := eng.db.Create(link).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	eng.ctx.GeoLookup = func(string) (string, string, string) { return "CN", "Beijing", "Beijing" }

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/route-priority", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	eng.Handle(rec, req, link)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://china.example" {
		t.Errorf("geo rule must beat split rule: got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestHandleSplitRoutingServesSplitTarget(t *testing.T) {
	eng := newTestEngine(t)
	t.Cleanup(eng.Close)

	link := &Link{
		OrgID:   1,
		Slug:    "split-only",
		Target:  "https://default",
		Enabled: true,
		RoutingRules: RoutingRules{
			{Type: "split", Weight: 100, Target: "https://variant.example"},
		},
	}
	if err := eng.db.Create(link).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/split-only", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	eng.Handle(rec, req, link)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://variant.example" {
		t.Errorf("split rule should redirect to its target: got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	eng.Close()
	var ev LinkEvent
	if err := eng.db.Where("link_id = ?", link.ID).Last(&ev).Error; err != nil {
		t.Fatalf("no click event recorded: %v", err)
	}
	if ev.Variant != "https://variant.example" {
		t.Errorf("variant not recorded: %q", ev.Variant)
	}
}

var errCtxNotSet = errors.New("huma context not stored on input")

func TestResolveMethodsSetContext(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	hctx := humago.NewContext(nil, r, httptest.NewRecorder())

	inputs := []struct {
		name string
		call func(huma.Context) error
	}{
		{"ListLinksInput", func(c huma.Context) error {
			i := &ListLinksInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"LinkMetadataInput", func(c huma.Context) error {
			i := &LinkMetadataInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"GetLinkInput", func(c huma.Context) error {
			i := &GetLinkInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"CreateLinkInput", func(c huma.Context) error {
			i := &CreateLinkInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"QuickCreateLinkInput", func(c huma.Context) error {
			i := &QuickCreateLinkInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"ExportLinksCSVInput", func(c huma.Context) error {
			i := &ExportLinksCSVInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"UpdateLinkInput", func(c huma.Context) error {
			i := &UpdateLinkInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"DeleteLinkInput", func(c huma.Context) error {
			i := &DeleteLinkInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"LinkStatsInput", func(c huma.Context) error {
			i := &LinkStatsInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
		{"LinkQRInput", func(c huma.Context) error {
			i := &LinkQRInput{}
			i.Resolve(c)
			if i.Ctx != c {
				return errCtxNotSet
			}
			return nil
		}},
	}
	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			if err := in.call(hctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// wipeLinksTables clears rows a previous run of the same test (SQLite in-memory
// DBs with cache=shared persist across -count=2 reruns) may have left behind.
func wipeLinksTables(t *testing.T, p *Plugin) {
	t.Helper()
	p.db.Where("1 = 1").Delete(&Link{})
	p.db.Where("1 = 1").Delete(&LinkEvent{})
	p.db.Where("1 = 1").Delete(&dns.Domain{})
}
