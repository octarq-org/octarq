package plugin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/octarq-org/octarq/plugin"
)

type mockDescriberPlugin struct {
	name string
	info plugin.Info
}

func (m *mockDescriberPlugin) Name() string                              { return m.name }
func (m *mockDescriberPlugin) Models() []any                             { return nil }
func (m *mockDescriberPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {}
func (m *mockDescriberPlugin) Describe() plugin.Info                     { return m.info }

type mockPlainPlugin struct {
	name string
}

func (m *mockPlainPlugin) Name() string                              { return m.name }
func (m *mockPlainPlugin) Models() []any                             { return nil }
func (m *mockPlainPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {}

func TestNormalizeIssuer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://accounts.google.com/", "https://accounts.google.com"},
		{" https://idp.example.com/// ", "https://idp.example.com"},
		{"https://auth.company.org", "https://auth.company.org"},
		{"", ""},
	}
	for _, tc := range cases {
		got := plugin.NormalizeIssuer(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeIssuer(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMemberRemovedHooks(t *testing.T) {
	plugin.ResetMemberRemovedHooks()
	defer plugin.ResetMemberRemovedHooks()

	var called1, called2 bool
	plugin.RegisterMemberRemovedHook("hook1", func(orgID, userID uint) {
		if orgID == 10 && userID == 20 {
			called1 = true
		}
	})
	plugin.RegisterMemberRemovedHook("hook2", func(orgID, userID uint) {
		if orgID == 10 && userID == 20 {
			called2 = true
		}
	})
	// Panicking hook should be caught and not abort other hooks
	plugin.RegisterMemberRemovedHook("hook_panic", func(orgID, userID uint) {
		panic("boom")
	})
	// Nil hook should be ignored
	plugin.RegisterMemberRemovedHook("hook_nil", nil)

	plugin.NotifyMemberRemoved(10, 20)
	if !called1 || !called2 {
		t.Errorf("expected both hooks to be called, got called1=%v called2=%v", called1, called2)
	}

	// Reset hooks
	plugin.ResetMemberRemovedHooks()
	called1 = false
	called2 = false
	plugin.NotifyMemberRemoved(10, 20)
	if called1 || called2 {
		t.Error("expected no hooks to run after reset")
	}
}

func TestPermResolverAndHasPerm(t *testing.T) {
	plugin.ResetPermRegistry()
	defer plugin.ResetPermRegistry()

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// No resolver set
	allow, decided := plugin.ResolvePerm(req, "test.perm")
	if allow || decided {
		t.Errorf("expected false, false when no resolver, got allow=%v decided=%v", allow, decided)
	}

	// Set resolver
	plugin.SetPermResolver(func(r *http.Request, permKey string) (bool, bool) {
		if permKey == "allowed.perm" {
			return true, true
		}
		if permKey == "denied.perm" {
			return false, true
		}
		return false, false
	})

	allow, decided = plugin.ResolvePerm(req, "allowed.perm")
	if !allow || !decided {
		t.Errorf("expected true, true for allowed.perm, got %v, %v", allow, decided)
	}

	allow, decided = plugin.ResolvePerm(req, "denied.perm")
	if allow || !decided {
		t.Errorf("expected false, true for denied.perm, got %v, %v", allow, decided)
	}

	// Test Context.HasPerm
	var nilCtx *plugin.Context
	if nilCtx.HasPerm(req, "any", "admin") {
		t.Error("nil Context.HasPerm should return false")
	}

	ctxNoPerm := &plugin.Context{}
	if ctxNoPerm.HasPerm(req, "any", "admin") {
		t.Error("Context with nil RequirePerm should return false")
	}

	ctxWithPerm := &plugin.Context{
		RequirePerm: func(r *http.Request, permKey, minRole string) bool {
			return permKey == "ok.perm" && minRole == "member"
		},
	}
	if !ctxWithPerm.HasPerm(req, "ok.perm", "member") {
		t.Error("expected HasPerm to return true for matching call")
	}
	if ctxWithPerm.HasPerm(req, "bad.perm", "member") {
		t.Error("expected HasPerm to return false for mismatching perm")
	}
}

func TestServiceNames(t *testing.T) {
	if got := plugin.CleanupServiceName("mail"); got != "mail.cleanup" {
		t.Errorf("CleanupServiceName = %q, want mail.cleanup", got)
	}
	if got := plugin.ExportServiceName("links"); got != "links.export" {
		t.Errorf("ExportServiceName = %q, want links.export", got)
	}
	if got := plugin.PurgeServiceName("dns"); got != "dns.purge" {
		t.Errorf("PurgeServiceName = %q, want dns.purge", got)
	}
	if got := plugin.MCPExportServiceName("emails"); got != "emails.mcp_export" {
		t.Errorf("MCPExportServiceName = %q, want emails.mcp_export", got)
	}
	if got := plugin.OverviewServiceName("analytics"); got != "analytics.overview" {
		t.Errorf("OverviewServiceName = %q, want analytics.overview", got)
	}
}

func TestLookupAsTypeSafety(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.Provide("str.service", "hello world")
	reg.Provide("num.service", 42)

	ctx := &plugin.Context{
		Lookup: reg.Lookup,
	}

	// 1. Nil context / nil lookup
	if val, ok := plugin.LookupAs[string](nil, "str.service"); ok || val != "" {
		t.Errorf("LookupAs with nil context should return zero, false")
	}
	if val, ok := plugin.LookupServiceAs[string](nil, "str.service"); ok || val != "" {
		t.Errorf("LookupServiceAs with nil lookup should return zero, false")
	}

	// 2. Non-existent service
	if val, ok := plugin.LookupAs[string](ctx, "nonexistent"); ok || val != "" {
		t.Errorf("LookupAs nonexistent should return zero, false")
	}

	// 3. Matching type assertion
	if val, ok := plugin.LookupAs[string](ctx, "str.service"); !ok || val != "hello world" {
		t.Errorf("LookupAs matching string failed: got %q, %v", val, ok)
	}
	if val, ok := plugin.LookupAs[int](ctx, "num.service"); !ok || val != 42 {
		t.Errorf("LookupAs matching int failed: got %d, %v", val, ok)
	}

	// 4. Type mismatch: MUST return zero, false WITHOUT PANIC
	if val, ok := plugin.LookupAs[int](ctx, "str.service"); ok || val != 0 {
		t.Errorf("LookupAs with mismatched type should return zero, false without panic, got %v, %v", val, ok)
	}
	if val, ok := plugin.LookupAs[bool](ctx, "num.service"); ok || val != false {
		t.Errorf("LookupAs with mismatched bool type should return zero, false without panic, got %v, %v", val, ok)
	}
}

func TestPluginMetadataAndCore(t *testing.T) {
	p1 := &mockDescriberPlugin{
		name: "p1",
		info: plugin.Info{
			Group: "feature-a",
			Core:  true,
		},
	}
	p2 := &mockDescriberPlugin{
		name: "p2",
		info: plugin.Info{
			Group: "feature-a",
			Core:  false,
		},
	}
	p3 := &mockPlainPlugin{name: "plain"}

	// Describe
	info1 := plugin.Describe(p1)
	if info1.Group != "feature-a" || !info1.Core {
		t.Errorf("Describe(p1) unexpected: %+v", info1)
	}
	info3 := plugin.Describe(p3)
	if info3.Group != "" || info3.Core {
		t.Errorf("Describe(p3) should be empty: %+v", info3)
	}

	// FeatureKey
	if k := plugin.FeatureKey(p1); k != "feature-a" {
		t.Errorf("FeatureKey(p1) = %q, want feature-a", k)
	}
	if k := plugin.FeatureKey(p3); k != "plain" {
		t.Errorf("FeatureKey(p3) = %q, want plain", k)
	}

	// FeatureIsCore
	plugins := []plugin.Plugin{p1, p2, p3}
	if !plugin.FeatureIsCore(plugins, "feature-a") {
		t.Error("expected feature-a to be core because p1 is core")
	}
	if plugin.FeatureIsCore(plugins, "plain") {
		t.Error("expected plain not to be core")
	}
	if plugin.FeatureIsCore(plugins, "nonexistent") {
		t.Error("expected nonexistent not to be core")
	}
}

func TestHelpDocFillDefaultsAndSorting(t *testing.T) {
	doc1 := plugin.HelpDoc{
		Slug:     "doc1",
		Title:    "Alpha",
		Category: "START",
		Order:    10,
	}
	doc1.FillDefaults("plugin1", "")
	if doc1.Category != "start" {
		t.Errorf("Category = %q, want start", doc1.Category)
	}

	doc2 := plugin.HelpDoc{
		Slug:     "doc2",
		Title:    "Beta",
		Category: "",
	}
	doc2.FillDefaults("myplugin", "messaging")
	if doc2.Category != "services" { // messaging remaps to services
		t.Errorf("Category = %q, want services", doc2.Category)
	}

	docUnknown := plugin.HelpDoc{
		Slug:     "doc3",
		Title:    "Gamma",
		Category: "totally-unknown-category",
	}
	docUnknown.FillDefaults("myplugin", "")
	if docUnknown.Category != "services" {
		t.Errorf("Category for unknown = %q, want services", docUnknown.Category)
	}

	// CompareHelpDocs
	docA := plugin.HelpDoc{Category: "start", Order: 1, Title: "A"}
	docB := plugin.HelpDoc{Category: "start", Order: 2, Title: "B"}
	docC := plugin.HelpDoc{Category: "licensing", Order: 1, Title: "C"}

	if !plugin.CompareHelpDocs(docA, docB) {
		t.Error("docA should come before docB (order)")
	}
	if !plugin.CompareHelpDocs(docA, docC) {
		t.Error("start category should come before licensing")
	}
}

func TestMustParseHelpDocPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseHelpDoc with invalid yaml should panic")
		}
	}()
	invalidYAML := "---\n: [invalid yaml\n---\nBody"
	_ = plugin.MustParseHelpDoc(invalidYAML)
}

func TestLoadHelpDocsEdgeCases(t *testing.T) {
	if docs := plugin.LoadHelpDocs(nil); docs != nil {
		t.Errorf("LoadHelpDocs(nil) expected nil, got %v", docs)
	}

	mockFS := fstest.MapFS{
		"notitle.md":    &fstest.MapFile{Data: []byte("Just markdown body without frontmatter")},
		"zh_only.zh.md": &fstest.MapFile{Data: []byte("中文内容")},
	}
	docs := plugin.LoadHelpDocs(mockFS)
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc (English only as root), got %d", len(docs))
	}
	if docs[0].Slug != "notitle" || docs[0].Title != "notitle" {
		t.Errorf("fallback title/slug mismatch: slug=%q title=%q", docs[0].Slug, docs[0].Title)
	}
}
