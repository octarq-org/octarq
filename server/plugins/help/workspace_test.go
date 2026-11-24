package help

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type helpTestSettings struct {
	workspace map[string]string
	global    map[string]string
}

func newHelpTestSettings() *helpTestSettings {
	return &helpTestSettings{
		workspace: make(map[string]string),
		global:    make(map[string]string),
	}
}

func (s *helpTestSettings) GetWorkspaceSetting(orgID uint, key string) string {
	if s == nil {
		return ""
	}
	return s.workspace[key]
}

func (s *helpTestSettings) SetWorkspaceSetting(orgID uint, key, value string) error {
	if s.workspace == nil {
		s.workspace = make(map[string]string)
	}
	s.workspace[key] = value
	return nil
}

func (s *helpTestSettings) GetGlobalSetting(key string) string {
	if s == nil {
		return ""
	}
	return s.global[key]
}

func (s *helpTestSettings) SetGlobalSetting(key, value string) error {
	if s.global == nil {
		s.global = make(map[string]string)
	}
	s.global[key] = value
	return nil
}

func newHelpTestHost(settings plugin.SettingsStore, orgID uint) *plugin.TestHost {
	return &plugin.TestHost{
		SessionMock: &plugin.TestSession{
			OrgIDFn: func(*http.Request) uint { return orgID },
		},
		SettingsMock: settings,
	}
}

func newHelpTestPlugin(t *testing.T, plugins []plugin.Plugin, host plugin.Host, db *gorm.DB) *Plugin {
	t.Helper()
	p := New()
	p.pctx = &plugin.Context{
		DB:            db,
		Host:          host,
		ActivePlugins: func() []plugin.Plugin { return plugins },
		PluginActive:  func(uint, plugin.Plugin) bool { return true },
		FeatureActive: func(uint, string) bool { return true },
	}
	p.SetHost(host)
	return p
}

func openHelpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Org{}); err != nil {
		t.Fatalf("migrate orgs: %v", err)
	}
	return db
}

func renderHelpDoc(t *testing.T, p *Plugin, slug, lang, ifNoneMatch string) *GetDocOutput {
	t.Helper()
	target := "/api/help/docs/" + slug
	if lang != "" {
		target += "?lang=" + lang
	}
	req := httptest.NewRequest("GET", target, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	humaCtx := humago.NewContext(nil, req, httptest.NewRecorder())
	input := &GetDocInput{Slug: slug}
	if errs := input.Resolve(humaCtx); len(errs) > 0 {
		t.Fatalf("Resolve failed: %v", errs)
	}
	out, err := p.getDoc(context.Background(), input)
	if err != nil {
		t.Fatalf("getDoc(%q) failed: %v", slug, err)
	}
	return out
}

func TestSubstituteHelpPlaceholdersEscapesValues(t *testing.T) {
	values := map[string]string{
		"app_name": "Acme*Corp",
		"domain":   "example.com",
		"org_slug": "acme",
	}
	got := substituteHelpPlaceholders("{{app_name}} on {{domain}} / {{org_slug}}", "en", values)
	want := `Acme\*Corp on example.com / acme`
	if got != want {
		t.Fatalf("substitution = %q, want %q", got, want)
	}

	htmlValues := map[string]string{"app_name": "<b>Acme & Co</b>"}
	got = substituteHelpPlaceholders("{{app_name}}", "en", htmlValues)
	want = "&lt;b&gt;Acme &amp; Co&lt;/b&gt;"
	if got != want {
		t.Fatalf("html substitution = %q, want %q", got, want)
	}
}

func TestSubstituteHelpPlaceholdersUnknownUntouched(t *testing.T) {
	got := substituteHelpPlaceholders("{{unknown_key}} {{app_name}}", "en", map[string]string{"app_name": "Acme"})
	want := "{{unknown_key}} Acme"
	if got != want {
		t.Fatalf("substitution = %q, want %q", got, want)
	}
}

func TestSubstituteHelpPlaceholdersMissingMarkers(t *testing.T) {
	cases := map[string]string{
		"en":    "(not configured)",
		"zh":    "（未配置）",
		"zh-CN": "（未配置）",
		"es":    "(sin configurar)",
		"pt":    "(não configurado)",
		"ja":    "（未設定）",
		"fr":    "(not configured)",
	}
	for lang, want := range cases {
		got := substituteHelpPlaceholders("{{app_name}}", lang, map[string]string{})
		if got != want {
			t.Errorf("lang %q marker = %q, want %q", lang, got, want)
		}
	}
}

func TestValidateHelpSubstitutionKeyRejectsSensitiveKeys(t *testing.T) {
	sensitive := []string{
		"api_key",
		"access_token",
		"user_password",
		"webhook_secret",
		"vault_value",
		"credential_id",
	}
	for _, key := range sensitive {
		if err := validateHelpSubstitutionKey(key); err == nil {
			t.Errorf("key %q was accepted, want rejection", key)
		}
	}
	if len(helpSubstitutionKeys) == 0 {
		t.Fatal("substitution whitelist is empty")
	}
	for _, key := range helpSubstitutionKeys {
		if err := validateHelpSubstitutionKey(key); err != nil {
			t.Errorf("whitelisted key %q was rejected: %v", key, err)
		}
	}
}

func TestGetDocSubstitutesWorkspaceValues(t *testing.T) {
	settings := newHelpTestSettings()
	settings.workspace["app_name"] = "Acme_Prod"
	settings.workspace["base_domain"] = "example.com"
	db := openHelpTestDB(t)
	if err := db.Create(&models.Org{Name: "Acme", Slug: "acme-org", Kind: models.OrgKindTeam}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	doc := plugin.HelpDoc{
		Slug:     "welcome",
		Title:    "Welcome",
		Category: "start",
		Markdown: "Welcome to {{app_name}} at {{domain}}. Org {{org_slug}}. Unknown {{missing_key}}.",
	}
	host := newHelpTestHost(settings, 1)
	p := newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: []plugin.HelpDoc{doc}}}, host, db)

	out := renderHelpDoc(t, p, "welcome", "", "")
	for _, want := range []string{"Acme_Prod", "example.com", "acme-org", "{{missing_key}}"} {
		if !strings.Contains(out.Body.HTML, want) {
			t.Errorf("rendered HTML missing %q: %s", want, out.Body.HTML)
		}
	}
	if strings.Contains(out.Body.HTML, "{{app_name}}") || strings.Contains(out.Body.HTML, "{{domain}}") {
		t.Errorf("known placeholder was not substituted: %s", out.Body.HTML)
	}
	if strings.Contains(out.Body.HTML, "<em>") {
		t.Errorf("placeholder value introduced markdown emphasis: %s", out.Body.HTML)
	}
}

func TestGetDocSubstitutionUsesTranslatedMarkdown(t *testing.T) {
	settings := newHelpTestSettings()
	settings.workspace["app_name"] = "Acme"
	doc := plugin.HelpDoc{
		Slug:     "welcome",
		Title:    "Welcome",
		Category: "start",
		Markdown: "Base {{app_name}}",
		Translations: map[string]plugin.HelpDocTranslation{
			"zh": {Title: "欢迎", Markdown: "中文 {{app_name}}"},
		},
	}
	host := newHelpTestHost(settings, 1)
	p := newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: []plugin.HelpDoc{doc}}}, host, nil)

	out := renderHelpDoc(t, p, "welcome", "zh", "")
	if !strings.Contains(out.Body.HTML, "中文 Acme") {
		t.Errorf("translated markdown was not substituted: %s", out.Body.HTML)
	}
	if strings.Contains(out.Body.HTML, "Base") {
		t.Errorf("base markdown leaked into the translated response: %s", out.Body.HTML)
	}
}

func TestGetDocMissingKnownPlaceholderUsesLocalizedMarker(t *testing.T) {
	doc := plugin.HelpDoc{
		Slug:     "welcome",
		Title:    "Welcome",
		Category: "start",
		Markdown: "Org {{org_slug}}",
		Translations: map[string]plugin.HelpDocTranslation{
			"zh": {Title: "欢迎", Markdown: "组织 {{org_slug}}"},
		},
	}
	host := newHelpTestHost(nil, 1)
	p := newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: []plugin.HelpDoc{doc}}}, host, nil)

	out := renderHelpDoc(t, p, "welcome", "zh", "")
	if !strings.Contains(out.Body.HTML, "（未配置）") {
		t.Errorf("expected localized missing marker, got %s", out.Body.HTML)
	}
}

func TestGetDocETagAndRenderCache(t *testing.T) {
	settings := newHelpTestSettings()
	settings.workspace["app_name"] = "Acme"
	doc := plugin.HelpDoc{
		Slug:     "welcome",
		Title:    "Welcome",
		Category: "start",
		Markdown: "Welcome to {{app_name}}",
	}
	host := newHelpTestHost(settings, 1)
	p := newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: []plugin.HelpDoc{doc}}}, host, nil)

	first := renderHelpDoc(t, p, "welcome", "", "")
	if first.ETag == "" || first.Status != http.StatusOK {
		t.Fatalf("unexpected first response: status=%d etag=%q", first.Status, first.ETag)
	}
	if !strings.Contains(first.Body.HTML, "Acme") {
		t.Fatalf("first response missing substituted value: %s", first.Body.HTML)
	}

	cached := renderHelpDoc(t, p, "welcome", "", "")
	if cached.ETag != first.ETag || cached.Body.HTML != first.Body.HTML {
		t.Fatalf("cached response drifted: %+v vs %+v", cached, first)
	}

	notModified := renderHelpDoc(t, p, "welcome", "", first.ETag)
	if notModified.Status != http.StatusNotModified || notModified.Body.HTML != "" {
		t.Fatalf("expected 304 with empty body, got status=%d html=%q", notModified.Status, notModified.Body.HTML)
	}

	settings.workspace["app_name"] = "Beta Inc"
	updated := renderHelpDoc(t, p, "welcome", "", first.ETag)
	if updated.Status != http.StatusOK {
		t.Fatalf("changed settings should bypass the cached render, got status=%d", updated.Status)
	}
	if updated.ETag == first.ETag {
		t.Errorf("ETag did not change after the substituted content changed")
	}
	if !strings.Contains(updated.Body.HTML, "Beta Inc") {
		t.Errorf("updated response did not reflect the new value: %s", updated.Body.HTML)
	}
}

func TestResolveHelpLang(t *testing.T) {
	cases := map[string]string{
		"":               "en",
		"*":              "en",
		"en-US":          "en",
		"zh-CN,zh;q=0.9": "zh",
		"pt-BR":          "pt",
		"ja":             "ja",
		"fr-FR":          "en",
	}
	for input, want := range cases {
		if got := resolveHelpLang(input); got != want {
			t.Errorf("resolveHelpLang(%q) = %q, want %q", input, got, want)
		}
	}
	if got := helpRequestLang(nil); got != "en" {
		t.Errorf("helpRequestLang(nil) = %q, want en", got)
	}
}

func TestValidateHelpSubstitutionKeyRejectsUnknownPlaceholder(t *testing.T) {
	if err := validateHelpSubstitutionKey("not_a_placeholder"); err == nil {
		t.Error("unknown placeholder key was accepted")
	}
}

func TestETagMatches(t *testing.T) {
	etag := `"abc123"`
	cases := map[string]bool{
		"":                  false,
		`"abc123"`:          true,
		`W/"abc123"`:        true,
		`"other", "abc123"`: true,
		`*`:                 true,
		`"other"`:           false,
	}
	for header, want := range cases {
		if got := etagMatches(header, etag); got != want {
			t.Errorf("etagMatches(%q) = %v, want %v", header, got, want)
		}
	}
}
