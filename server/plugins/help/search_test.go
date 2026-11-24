package help

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

func runHelpSearch(t *testing.T, p *Plugin, req *http.Request) (*HelpSearchOutput, error) {
	t.Helper()
	humaCtx := humago.NewContext(nil, req, httptest.NewRecorder())
	values := req.URL.Query()
	input := &HelpSearchInput{
		Q:        values.Get("q"),
		Lang:     values.Get("lang"),
		Category: values.Get("category"),
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("invalid limit %q: %v", raw, err)
		}
		input.Limit = limit
	}
	if errs := input.Resolve(humaCtx); len(errs) > 0 {
		t.Fatalf("Resolve failed: %v", errs)
	}
	return p.searchDocs(context.Background(), input)
}

func searchRequest(t *testing.T, query url.Values) *http.Request {
	t.Helper()
	target := "/api/help/search"
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return httptest.NewRequest("GET", target, nil)
}

func requireSearchStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with status %d, got nil", want)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma status error, got %v", err)
	}
	if statusErr.GetStatus() != want {
		t.Fatalf("status = %d, want %d (error: %v)", statusErr.GetStatus(), want, err)
	}
}

func findHelpSearchResult(results []HelpSearchResult, slug string) *HelpSearchResult {
	for i := range results {
		if results[i].Slug == slug {
			return &results[i]
		}
	}
	return nil
}

func searchHelpPlugin(t *testing.T, docs ...plugin.HelpDoc) *Plugin {
	t.Helper()
	host := newHelpTestHost(nil, 1)
	return newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: docs}}, host, nil)
}

func TestSearchHelpDocsMixedLanguageAndAnchor(t *testing.T) {
	cjkMarkdown := "## 登录配置\n\n使用 Octarq 登录账户并保存设置。"
	docs := []plugin.HelpDoc{
		{Slug: "zh-login", Title: "登录设置", Category: "settings", Markdown: cjkMarkdown},
		{Slug: "octarq-intro", Title: "Octarq overview", Category: "start", Markdown: "## Overview\n\nThe Octarq server stores your links."},
	}
	p := searchHelpPlugin(t, docs...)

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"登录 octarq"}, "lang": {"zh-CN"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if out.Body.Lang != "zh" {
		t.Errorf("response lang = %q, want zh", out.Body.Lang)
	}
	if len(out.Body.Results) == 0 {
		t.Fatal("mixed-language query returned no results")
	}

	hit := findHelpSearchResult(out.Body.Results, "zh-login")
	if hit == nil {
		t.Fatalf("zh-login missing from results: %+v", out.Body.Results)
	}
	if hit.Anchor != "heading" {
		t.Errorf("anchor = %q, want heading", hit.Anchor)
	}
	if hit.URL != "/help/settings/zh-login#heading" {
		t.Errorf("url = %q", hit.URL)
	}
	rendered, err := renderMarkdown(cjkMarkdown)
	if err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	if !strings.Contains(rendered, `id="heading"`) {
		t.Errorf("anchor does not match rendered heading id: %s", rendered)
	}
	if !strings.Contains(hit.Snippet.Text, "登录") {
		t.Errorf("snippet = %q, want the matched text", hit.Snippet.Text)
	}
	if len(hit.Snippet.Offsets) == 0 {
		t.Errorf("snippet offsets are empty")
	}
	runeLen := len([]rune(hit.Snippet.Text))
	for _, offset := range hit.Snippet.Offsets {
		if offset[0] < 0 || offset[1] > runeLen || offset[0] >= offset[1] {
			t.Errorf("offset %v is outside snippet rune length %d", offset, runeLen)
		}
	}

	fullWidth, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"ｏｃｔａｒｑ"}}))
	if err != nil {
		t.Fatalf("full-width search failed: %v", err)
	}
	if findHelpSearchResult(fullWidth.Body.Results, "octarq-intro") == nil {
		t.Errorf("NFKC-normalized query did not match: %+v", fullWidth.Body.Results)
	}
}

func TestSearchHelpDocsTitleBeatsBody(t *testing.T) {
	docs := []plugin.HelpDoc{
		{Slug: "body-hit", Title: "Account", Category: "start", Markdown: "## Overview\n\nbilling billing."},
		{Slug: "title-hit", Title: "Billing settings", Category: "commerce", Markdown: "## Overview\n\nNothing about money here."},
	}
	p := searchHelpPlugin(t, docs...)

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"billing"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(out.Body.Results) != 2 {
		t.Fatalf("expected 2 results, got %+v", out.Body.Results)
	}
	if out.Body.Results[0].Slug != "title-hit" {
		t.Errorf("title match should outrank body match, got %+v", out.Body.Results)
	}
}

func TestSearchHelpDocsAnchorMatchesRenderedHeading(t *testing.T) {
	markdown := "## Install steps\n\nInstall now.\n\n## 中文标题\n\n内容在这里。"
	p := searchHelpPlugin(t, plugin.HelpDoc{
		Slug:     "install",
		Title:    "Install",
		Category: "start",
		Markdown: markdown,
	})

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"install"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	hit := findHelpSearchResult(out.Body.Results, "install")
	if hit == nil {
		t.Fatal("install doc missing")
	}
	if hit.Anchor != "install-steps" {
		t.Errorf("anchor = %q, want install-steps", hit.Anchor)
	}
	if hit.AnchorTitle != "Install steps" {
		t.Errorf("anchor title = %q", hit.AnchorTitle)
	}

	cjkOut, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"内容"}}))
	if err != nil {
		t.Fatalf("cjk search failed: %v", err)
	}
	cjkHit := findHelpSearchResult(cjkOut.Body.Results, "install")
	if cjkHit == nil {
		t.Fatal("install doc missing for cjk query")
	}
	rendered, err := renderMarkdown(markdown)
	if err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	if cjkHit.Anchor != "heading" {
		t.Errorf("cjk anchor = %q, want heading", cjkHit.Anchor)
	}
	if !strings.Contains(rendered, `id="`+cjkHit.Anchor+`"`) {
		t.Errorf("anchor %q is absent from rendered HTML: %s", cjkHit.Anchor, rendered)
	}
}

func TestSearchHelpDocsCategoryFilter(t *testing.T) {
	docs := []plugin.HelpDoc{
		{Slug: "octarq-start", Title: "Octarq start", Category: "start", Markdown: "Octarq basics."},
		{Slug: "octarq-settings", Title: "Octarq settings", Category: "settings", Markdown: "Octarq settings."},
	}
	p := searchHelpPlugin(t, docs...)

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}, "category": {"settings"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(out.Body.Results) != 1 || out.Body.Results[0].Slug != "octarq-settings" {
		t.Errorf("category filter failed: %+v", out.Body.Results)
	}

	empty, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}, "category": {"operations"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if empty.Body.Results == nil || len(empty.Body.Results) != 0 {
		t.Errorf("expected empty results, got %+v", empty.Body.Results)
	}

	_, err = runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}, "category": {"bogus"}}))
	requireSearchStatus(t, err, http.StatusBadRequest)
}

func TestSearchHelpDocsOrgIsolation(t *testing.T) {
	docs := []plugin.HelpDoc{
		{Slug: "octarq-doc", Title: "Octarq doc", Category: "start", Markdown: "Octarq body."},
	}
	host := &plugin.TestHost{
		SessionMock: &plugin.TestSession{
			OrgIDFn: func(r *http.Request) uint {
				if r.Header.Get("X-Org") == "2" {
					return 2
				}
				return 1
			},
		},
	}
	p := newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: docs}}, host, nil)
	p.pctx.PluginActive = func(orgID uint, _ plugin.Plugin) bool { return orgID == 1 }

	req1 := searchRequest(t, url.Values{"q": {"octarq"}})
	req1.Header.Set("X-Org", "1")
	org1, err := runHelpSearch(t, p, req1)
	if err != nil {
		t.Fatalf("org1 search failed: %v", err)
	}
	if len(org1.Body.Results) != 1 {
		t.Errorf("org1 results = %+v", org1.Body.Results)
	}

	req2 := searchRequest(t, url.Values{"q": {"octarq"}})
	req2.Header.Set("X-Org", "2")
	org2, err := runHelpSearch(t, p, req2)
	if err != nil {
		t.Fatalf("org2 search failed: %v", err)
	}
	if len(org2.Body.Results) != 0 {
		t.Errorf("org2 must not see org1 docs, got %+v", org2.Body.Results)
	}
}

func TestSearchHelpDocsFeatureGating(t *testing.T) {
	docs := []plugin.HelpDoc{
		{Slug: "beta-doc", Title: "Beta Octarq", Category: "start", Feature: "beta", Markdown: "Octarq beta."},
	}
	enabled := false
	host := newHelpTestHost(nil, 1)
	p := newHelpTestPlugin(t, []plugin.Plugin{&mockPlugin{name: "core", helpDocs: docs}}, host, nil)
	p.pctx.FeatureActive = func(uint, string) bool { return enabled }

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(out.Body.Results) != 0 {
		t.Errorf("disabled feature doc must be absent, got %+v", out.Body.Results)
	}

	enabled = true
	out, err = runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(out.Body.Results) != 1 || out.Body.Results[0].Slug != "beta-doc" {
		t.Errorf("enabled feature doc missing: %+v", out.Body.Results)
	}
}

func TestSearchHelpDocsValidationAndClamp(t *testing.T) {
	docs := []plugin.HelpDoc{
		{Slug: "octarq-a", Title: "Octarq A", Category: "start", Markdown: "Octarq one."},
		{Slug: "octarq-b", Title: "Octarq B", Category: "start", Markdown: "Octarq two."},
		{Slug: "octarq-c", Title: "Octarq C", Category: "start", Markdown: "Octarq three."},
	}
	p := searchHelpPlugin(t, docs...)

	_, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {""}}))
	requireSearchStatus(t, err, http.StatusBadRequest)

	_, err = runHelpSearch(t, p, searchRequest(t, url.Values{"q": {strings.Repeat("a", 201)}}))
	requireSearchStatus(t, err, http.StatusBadRequest)

	_, err = runHelpSearch(t, p, searchRequest(t, url.Values{"q": {strings.Repeat("登", 201)}}))
	requireSearchStatus(t, err, http.StatusBadRequest)

	if _, err = runHelpSearch(t, p, searchRequest(t, url.Values{"q": {strings.Repeat("a", 200)}})); err != nil {
		t.Errorf("200-character query should be accepted: %v", err)
	}

	clampedLow, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}, "limit": {"-5"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(clampedLow.Body.Results) != 1 {
		t.Errorf("negative limit should clamp to 1, got %d", len(clampedLow.Body.Results))
	}

	clampedHigh, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}, "limit": {"999"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(clampedHigh.Body.Results) != 3 {
		t.Errorf("high limit should cap at 50 and keep all 3 docs, got %d", len(clampedHigh.Body.Results))
	}

	none, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"zzzznotfound"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if none.Body.Results == nil || len(none.Body.Results) != 0 {
		t.Errorf("empty query result should be an empty array, got %+v", none.Body.Results)
	}
}

func TestSearchHelpDocsNilHost(t *testing.T) {
	p := newHelpTestPlugin(t, []plugin.Plugin{
		&mockPlugin{name: "core", helpDocs: []plugin.HelpDoc{
			{Slug: "octarq-doc", Title: "Octarq doc", Category: "start", Markdown: "Octarq body."},
		}},
	}, nil, nil)
	p.SetHost(nil)

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}}))
	if err != nil {
		t.Fatalf("search with nil host failed: %v", err)
	}
	if len(out.Body.Results) != 1 {
		t.Errorf("expected one result with nil host, got %+v", out.Body.Results)
	}
}

func TestSearchHelpDocsRebuildsIndexWhenDocsChange(t *testing.T) {
	mock := &mockPlugin{
		name: "core",
		helpDocs: []plugin.HelpDoc{
			{Slug: "first-doc", Title: "First", Category: "start", Markdown: "First body."},
		},
	}
	host := newHelpTestHost(nil, 1)
	p := newHelpTestPlugin(t, []plugin.Plugin{mock}, host, nil)

	before, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"second"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(before.Body.Results) != 0 {
		t.Fatalf("unexpected initial results: %+v", before.Body.Results)
	}

	mock.helpDocs = append(mock.helpDocs, plugin.HelpDoc{
		Slug: "second-doc", Title: "Second", Category: "start", Markdown: "Second body.",
	})
	after, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"second"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(after.Body.Results) != 1 || after.Body.Results[0].Slug != "second-doc" {
		t.Errorf("index did not rebuild after docs changed: %+v", after.Body.Results)
	}
}

func TestHelpHTTPEndpoints(t *testing.T) {
	p := searchHelpPlugin(t, plugin.HelpDoc{
		Slug:     "octarq-doc",
		Title:    "Octarq doc",
		Category: "start",
		Markdown: "## Install\n\nInstall Octarq now.",
	})
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Help API", "1.0.0"))
	p.pctx.Huma = api
	p.Mount(nil, p.pctx)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/help/search?q=octarq")
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d", resp.StatusCode)
	}
	var body HelpSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(body.Results) != 1 || body.Results[0].Slug != "octarq-doc" {
		t.Fatalf("unexpected search body: %+v", body)
	}
	if body.Results[0].URL != "/help/start/octarq-doc#install" {
		t.Errorf("url = %q", body.Results[0].URL)
	}

	docResp, err := http.Get(server.URL + "/api/help/docs/octarq-doc")
	if err != nil {
		t.Fatalf("doc request failed: %v", err)
	}
	docResp.Body.Close()
	if docResp.StatusCode != http.StatusOK {
		t.Fatalf("doc status = %d", docResp.StatusCode)
	}
	etag := docResp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("doc response has no ETag header")
	}

	req, err := http.NewRequest("GET", server.URL+"/api/help/docs/octarq-doc", nil)
	if err != nil {
		t.Fatalf("build conditional request: %v", err)
	}
	req.Header.Set("If-None-Match", etag)
	conditional, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("conditional request failed: %v", err)
	}
	defer conditional.Body.Close()
	if conditional.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", conditional.StatusCode)
	}
	payload, err := io.ReadAll(conditional.Body)
	if err != nil {
		t.Fatalf("read 304 body: %v", err)
	}
	if len(payload) != 0 {
		t.Errorf("304 response carried a body: %q", payload)
	}
}

func TestSearchHelpDocsSnippetWindowAroundLateMatch(t *testing.T) {
	prefix := strings.Repeat("alpha ", 60)
	p := searchHelpPlugin(t, plugin.HelpDoc{
		Slug:     "long-doc",
		Title:    "Long document",
		Category: "start",
		Markdown: "## Details\n\n" + prefix + "octarq target near the end.",
	})

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"target"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	hit := findHelpSearchResult(out.Body.Results, "long-doc")
	if hit == nil {
		t.Fatal("long doc missing")
	}
	if len([]rune(hit.Snippet.Text)) > helpSnippetRunes {
		t.Errorf("snippet is %d runes, want at most %d", len([]rune(hit.Snippet.Text)), helpSnippetRunes)
	}
	if !strings.Contains(hit.Snippet.Text, "target") {
		t.Errorf("snippet does not contain the match: %q", hit.Snippet.Text)
	}
	if len(hit.Snippet.Offsets) == 0 {
		t.Error("snippet offsets are empty for a body match")
	}
}

func TestSearchHelpDocsTitleOnlySnippet(t *testing.T) {
	prefix := strings.Repeat("alpha ", 60)
	p := searchHelpPlugin(t, plugin.HelpDoc{
		Slug:     "long-title-doc",
		Title:    "Octarq reference",
		Category: "start",
		Markdown: "## Details\n\n" + prefix,
	})

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarq"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	hit := findHelpSearchResult(out.Body.Results, "long-title-doc")
	if hit == nil {
		t.Fatal("title-only doc missing")
	}
	if hit.Anchor != "" {
		t.Errorf("title-only match should use the document anchor, got %q", hit.Anchor)
	}
	if len([]rune(hit.Snippet.Text)) > helpSnippetRunes {
		t.Errorf("snippet is %d runes, want at most %d", len([]rune(hit.Snippet.Text)), helpSnippetRunes)
	}
	if len(hit.Snippet.Offsets) != 0 {
		t.Errorf("title-only snippet should have no body offsets, got %v", hit.Snippet.Offsets)
	}
}

func TestSearchHelpDocsHeadingOnlyMatchUsesHeadingAnchor(t *testing.T) {
	p := searchHelpPlugin(t, plugin.HelpDoc{
		Slug:     "billing-faq",
		Title:    "Reference",
		Category: "commerce",
		Markdown: "## Billing FAQ\n\nRead this section first.\n\n## Other\n\nDifferent material.",
	})

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"billing"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	hit := findHelpSearchResult(out.Body.Results, "billing-faq")
	if hit == nil {
		t.Fatal("billing doc missing")
	}
	if hit.Anchor != "billing-faq" {
		t.Errorf("anchor = %q, want billing-faq", hit.Anchor)
	}
	if hit.URL != "/help/commerce/billing-faq#billing-faq" {
		t.Errorf("url = %q", hit.URL)
	}
}

func TestSearchHelpDocsIndexesFencedCode(t *testing.T) {
	p := searchHelpPlugin(t, plugin.HelpDoc{
		Slug:     "code-doc",
		Title:    "Code",
		Category: "start",
		Markdown: "## Example\n\n```go\nfunc OctarqHelper() {}\n```\n",
	})

	out, err := runHelpSearch(t, p, searchRequest(t, url.Values{"q": {"octarqhelper"}}))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(out.Body.Results) != 1 || out.Body.Results[0].Slug != "code-doc" {
		t.Errorf("fenced code text was not indexed: %+v", out.Body.Results)
	}
}

func TestGetDocsWithoutContextFailsClosed(t *testing.T) {
	p := New()
	if docs := p.getDocs(1, "en"); docs != nil {
		t.Errorf("getDocs without a context returned %d docs", len(docs))
	}
	p.pctx = &plugin.Context{}
	if docs := p.getDocs(1, "en"); docs != nil {
		t.Errorf("getDocs without plugin callbacks returned %d docs", len(docs))
	}
}
