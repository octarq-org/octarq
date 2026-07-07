package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/led/internal/models"
	"github.com/pquerna/otp/totp"
)

// TestLogoutClearsCookie covers the plain single-device logout.
func TestLogoutClearsCookie(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)
	rec := do(srv, "POST", "/api/auth/logout", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: got %d", rec.Code)
	}
	// The Set-Cookie clears the session (MaxAge<0 / empty value).
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "led_session" && (c.MaxAge < 0 || c.Value == "") {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout did not clear the session cookie")
	}
}

// TestOrgSwitchAndMembers covers createOrg, switchOrg (member + non-member), and
// listOrgMembers — the multi-org navigation surface.
func TestOrgSwitchAndMembers(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Create a second org — the caller is added as owner.
	rec := do(srv, "POST", "/api/orgs", cookies, `{"name":"Second Org"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org: got %d (%s)", rec.Code, rec.Body.String())
	}
	var org struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &org)

	// Switch to the org the caller belongs to → 200.
	if rec := do(srv, "POST", "/api/auth/switch-org", cookies, `{"orgId":`+strconv.FormatUint(uint64(org.ID), 10)+`}`); rec.Code != http.StatusOK {
		t.Errorf("switch to own org: got %d", rec.Code)
	}
	// Switch to an org the caller is NOT a member of → 403.
	if rec := do(srv, "POST", "/api/auth/switch-org", cookies, `{"orgId":99999}`); rec.Code != http.StatusForbidden {
		t.Errorf("switch to foreign org: got %d, want 403", rec.Code)
	}

	// List members of the current org → 200, includes the admin.
	rec = do(srv, "GET", "/api/org/members", cookies, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "admin") {
		t.Errorf("list members: got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestExportLinksCSV covers the CSV export path.
func TestExportLinksCSV(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	if rec := do(srv, "POST", "/api/links", cookies, `{"slug":"promo","target":"https://example.com"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create link: got %d (%s)", rec.Code, rec.Body.String())
	}
	rec := do(srv, "GET", "/api/links/export.csv", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export csv: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "csv") {
		t.Errorf("content-type: got %q, want csv", ct)
	}
	if !strings.Contains(rec.Body.String(), "promo") {
		t.Errorf("csv missing the link slug: %s", rec.Body.String())
	}
}

// TestTwoFADisableAndStatus covers 2FA status + disable (enable is covered
// elsewhere), including the wrong-code rejection on disable.
func TestTwoFADisableAndStatus(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Initially disabled.
	if rec := do(srv, "GET", "/api/auth/2fa/status", cookies, ""); !strings.Contains(rec.Body.String(), "false") {
		t.Fatalf("status before: %s", rec.Body.String())
	}

	// Enroll: setup → enable with a valid TOTP code.
	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: got %d", rec.Code)
	}
	var setup struct {
		Secret    string `json:"secret"`
		QRDataURI string `json:"qrDataUri"`
	}
	json.Unmarshal(rec.Body.Bytes(), &setup)
	if setup.Secret == "" {
		t.Fatal("setup returned no secret")
	}
	if !strings.HasPrefix(setup.QRDataURI, "data:image/png;base64,") {
		t.Errorf("qr not a server-side data URI: %.30s", setup.QRDataURI)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/enable", cookies, `{"code":"`+code+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "GET", "/api/auth/2fa/status", cookies, ""); !strings.Contains(rec.Body.String(), "true") {
		t.Fatalf("status after enable: %s", rec.Body.String())
	}

	// Disable with a wrong code → rejected; 2FA stays on.
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"000000"}`); rec.Code == http.StatusOK {
		t.Error("disable accepted a wrong code")
	}
	// Disable with a valid code → off.
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"`+code2+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "GET", "/api/auth/2fa/status", cookies, ""); !strings.Contains(rec.Body.String(), "false") {
		t.Errorf("status after disable: %s", rec.Body.String())
	}
}

// TestUpdateOrgRename covers the workspace-rename endpoint + its role guard.
func TestUpdateOrgRename(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	if rec := do(srv, "PUT", "/api/org", cookies, `{"name":"Renamed Workspace"}`); rec.Code != http.StatusOK {
		t.Fatalf("rename: got %d (%s)", rec.Code, rec.Body.String())
	}
	var name string
	db.Model(&models.Org{}).Where("name = ?", "Renamed Workspace").Pluck("name", &name)
	if name != "Renamed Workspace" {
		t.Errorf("org not renamed, got %q", name)
	}
	// Empty name → 400.
	if rec := do(srv, "PUT", "/api/org", cookies, `{"name":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want 400", rec.Code)
	}
}

// TestInboundWebhookAuth covers the per-org inbound email webhook: bad token →
// 401, unknown org → 404, and a valid post stores the message in the org's box.
func TestInboundWebhookAuth(t *testing.T) {
	srv, db := newTestHandler(t)

	org := models.Org{Name: "Acme", Slug: "acme", InboundToken: "itok"}
	db.Create(&org)
	db.Create(&models.Mailbox{OrgID: org.ID, Address: "hi@acme.test", Enabled: true})

	msg := "From: a@x.com\r\nTo: hi@acme.test\r\nSubject: Hi\r\n\r\nbody"
	post := func(path string) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(msg))
		req.Header.Set("X-Led-To", "hi@acme.test")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post("/api/webhook/acme/email/inbound/wrong"); code != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401", code)
	}
	if code := post("/api/webhook/nope/email/inbound/itok"); code != http.StatusNotFound {
		t.Errorf("unknown org: got %d, want 404", code)
	}
	if code := post("/api/webhook/acme/email/inbound/itok"); code != http.StatusOK {
		t.Errorf("valid: got %d, want 200", code)
	}
	var count int64
	db.Model(&models.Email{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 stored email, got %d", count)
	}
}

// TestTwoFALoginFlow covers the full second-factor login: password alone defers,
// a wrong code is refused, and the right code completes the session.
func TestTwoFALoginFlow(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Enroll 2FA.
	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, "")
	var setup struct {
		Secret string `json:"secret"`
	}
	json.Unmarshal(rec.Body.Bytes(), &setup)
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/enable", cookies, `{"code":"`+code+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d", rec.Code)
	}

	// Password login now defers (no session cookie, twoFactorRequired).
	rec = do(srv, "POST", "/api/auth/login", nil, `{"username":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "twoFactorRequired") {
		t.Fatalf("login should require 2FA: %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("2FA-deferred login must not set a session cookie")
	}

	// Wrong code → 401.
	if rec := do(srv, "POST", "/api/auth/2fa/verify", nil, `{"username":"admin","password":"pw","code":"000000"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong code: got %d, want 401", rec.Code)
	}
	// Right code → 200 + session.
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"username":"admin","password":"pw","code":"`+code2+`"}`)
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("valid verify: got %d, cookies=%d", rec.Code, len(rec.Result().Cookies()))
	}
}

// TestReadEndpoints covers simple authenticated read handlers that had no
// coverage: me, health, audit log, and the DNS provider list.
func TestReadEndpoints(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)
	// Create a mailbox so there's at least one audit entry to list.
	do(srv, "POST", "/api/mailboxes", cookies, `{"address":"x@y.com"}`)

	for _, ep := range []string{"/api/auth/me", "/api/health", "/api/audit", "/api/dns/providers"} {
		if rec := do(srv, "GET", ep, cookies, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d (%s)", ep, rec.Code, rec.Body.String())
		}
	}
	// me without a session → 401.
	if rec := do(srv, "GET", "/api/auth/me", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauth me: got %d, want 401", rec.Code)
	}
}

// TestLinkStatsAndQR covers the per-link stats and QR handlers.
func TestLinkStatsAndQR(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/links", cookies, `{"slug":"q","target":"https://example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create link: got %d", rec.Code)
	}
	var link struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &link)
	id := strconv.FormatUint(uint64(link.ID), 10)

	if rec := do(srv, "GET", "/api/links/"+id+"/stats", cookies, ""); rec.Code != http.StatusOK {
		t.Errorf("stats: got %d", rec.Code)
	}
	rec = do(srv, "GET", "/api/links/"+id+"/qr", cookies, "")
	if rec.Code != http.StatusOK {
		t.Errorf("qr: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "png") {
		t.Errorf("qr content-type: got %q, want png", ct)
	}
}

// TestDNSRecordsCRUD covers the DNS record handlers through the mock provider
// (registered in comprehensive_api_test.go's init).
func TestDNSRecordsCRUD(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// A mock provider account + a domain bound to it with a zone id.
	rec := do(srv, "POST", "/api/provider-accounts", cookies, `{"name":"mock-acc","type":"mock","config":{"token":"t"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provider account: got %d (%s)", rec.Code, rec.Body.String())
	}
	var acc struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &acc)

	rec = do(srv, "POST", "/api/domains", cookies,
		`{"name":"mockdomain.com","providerAccountId":`+strconv.FormatUint(uint64(acc.ID), 10)+`,"zoneId":"z123","forLink":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create domain: got %d (%s)", rec.Code, rec.Body.String())
	}
	var dom struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &dom)
	did := strconv.FormatUint(uint64(dom.ID), 10)

	// list → create → update → delete a record.
	if rec := do(srv, "GET", "/api/domains/"+did+"/records", cookies, ""); rec.Code != http.StatusOK {
		t.Errorf("list records: got %d", rec.Code)
	}
	rec = do(srv, "POST", "/api/domains/"+did+"/records", cookies,
		`{"type":"A","name":"www.mockdomain.com","content":"1.2.3.4","ttl":300}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create record: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "PUT", "/api/domains/"+did+"/records/r123", cookies,
		`{"type":"A","name":"www.mockdomain.com","content":"5.6.7.8","ttl":300}`); rec.Code != http.StatusOK {
		t.Errorf("update record: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "DELETE", "/api/domains/"+did+"/records/r123", cookies, ""); rec.Code != http.StatusOK {
		t.Errorf("delete record: got %d", rec.Code)
	}
}

// TestWebhooksCRUD covers the event-webhook create/list/update/delete handlers.
func TestWebhooksCRUD(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/webhooks", cookies, `{"name":"hook","url":"https://example.com/hook","events":"*"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create webhook: got %d (%s)", rec.Code, rec.Body.String())
	}
	var wh struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &wh)
	id := strconv.FormatUint(uint64(wh.ID), 10)

	// missing name/url → 400.
	if rec := do(srv, "POST", "/api/webhooks", cookies, `{"name":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad webhook: got %d, want 400", rec.Code)
	}
	if rec := do(srv, "GET", "/api/webhooks", cookies, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hook") {
		t.Errorf("list webhooks: got %d", rec.Code)
	}
	if rec := do(srv, "PUT", "/api/webhooks/"+id, cookies, `{"name":"hook2","url":"https://example.com/h2","events":"link.click"}`); rec.Code != http.StatusOK {
		t.Errorf("update webhook: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "DELETE", "/api/webhooks/"+id, cookies, ""); rec.Code != http.StatusOK {
		t.Errorf("delete webhook: got %d", rec.Code)
	}
}

// TestNotificationChannelTest covers the "send a test notification" handler by
// pointing a webhook channel at a local server that accepts the POST.
func TestNotificationChannelTest(t *testing.T) {
	got := make(chan bool, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	body := `{"name":"ch","type":"webhook","config":"{\"url\":\"` + ts.URL + `\"}","enabled":true}`
	rec := do(srv, "POST", "/api/notification-channels", cookies, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create channel: got %d (%s)", rec.Code, rec.Body.String())
	}
	var ch struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &ch)

	rec = do(srv, "POST", "/api/notification-channels/"+strconv.FormatUint(uint64(ch.ID), 10)+"/test", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("test channel: got %d (%s)", rec.Code, rec.Body.String())
	}
	select {
	case <-got:
	default:
		t.Error("test notification did not reach the webhook server")
	}
}

// TestTokensCRUD covers API token create/list/delete.
func TestTokensCRUD(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/tokens", cookies, `{"name":"ci"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "led_") {
		t.Fatalf("create token: got %d (%s)", rec.Code, rec.Body.String())
	}
	var tok struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &tok)
	if rec := do(srv, "GET", "/api/tokens", cookies, ""); rec.Code != http.StatusOK {
		t.Errorf("list tokens: got %d", rec.Code)
	}
	if rec := do(srv, "DELETE", "/api/tokens/"+strconv.FormatUint(uint64(tok.ID), 10), cookies, ""); rec.Code != http.StatusOK {
		t.Errorf("delete token: got %d", rec.Code)
	}
	// deleting a non-existent token → 404.
	if rec := do(srv, "DELETE", "/api/tokens/99999", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing token: got %d, want 404", rec.Code)
	}
}

// TestSyncDomains covers importing zones from a provider (mock ListZones).
func TestSyncDomains(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/provider-accounts", cookies, `{"name":"m","type":"mock","config":{"token":"t"}}`)
	var acc struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &acc)

	if rec := do(srv, "POST", "/api/domains/sync", cookies, `{"providerAccountId":`+strconv.FormatUint(uint64(acc.ID), 10)+`}`); rec.Code != http.StatusOK {
		t.Fatalf("sync: got %d (%s)", rec.Code, rec.Body.String())
	}
	// The mock exposes mockdomain.com — it should now be listed.
	if rec := do(srv, "GET", "/api/domains", cookies, ""); !strings.Contains(rec.Body.String(), "mockdomain.com") {
		t.Errorf("synced domain missing: %s", rec.Body.String())
	}
	// providerAccountId required.
	if rec := do(srv, "POST", "/api/domains/sync", cookies, `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("sync without account: got %d, want 400", rec.Code)
	}
}

// TestCRUDErrorBranches sweeps the common error paths (invalid JSON body, bad
// non-numeric id, missing id) across CRUD resources — the branches unit happy
// paths skip.
func TestCRUDErrorBranches(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Invalid JSON body → 400 on create endpoints.
	for _, res := range []string{"/api/smtp-senders", "/api/provider-accounts", "/api/webhooks", "/api/mailboxes", "/api/links", "/api/tokens"} {
		if rec := do(srv, "POST", res, cookies, `{`); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s invalid body: got %d, want 400", res, rec.Code)
		}
	}
	// Non-numeric id → 400 on delete.
	for _, res := range []string{"/api/smtp-senders/", "/api/mailboxes/", "/api/links/", "/api/tokens/"} {
		if rec := do(srv, "DELETE", res+"abc", cookies, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("DELETE %sabc: got %d, want 400", res, rec.Code)
		}
	}
	// Missing resource → 404 on delete.
	for _, res := range []string{"/api/smtp-senders/", "/api/mailboxes/", "/api/links/"} {
		if rec := do(srv, "DELETE", res+"88888", cookies, ""); rec.Code != http.StatusNotFound {
			t.Errorf("DELETE %s88888: got %d, want 404", res, rec.Code)
		}
	}
}

// TestExtractBounceEvents covers the multi-provider bounce parser directly.
func TestExtractBounceEvents(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"sendgrid array", `[{"email":"a@x.com","event":"bounce"},{"email":"b@x.com","event":"dropped"}]`, 2},
		{"ses bounce", `{"notificationType":"Bounce","bounce":{"bouncedRecipients":[{"emailAddress":"a@x.com"}]}}`, 1},
		{"ses complaint", `{"notificationType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"a@x.com"}]}}`, 1},
		{"empty/irrelevant", `{"hello":"world"}`, 0},
		{"garbage", `not json`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(extractBounceEvents([]byte(c.body))); got != c.want {
				t.Errorf("%s: got %d events, want %d", c.name, got, c.want)
			}
		})
	}
}
