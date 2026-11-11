package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
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
		if c.Name == "octarq_session" && (c.MaxAge < 0 || c.Value == "") {
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
	// Enrolling and un-enrolling both re-authenticate: a live session is not
	// enough authority to touch the second factor (requirePasswordStepUp).
	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, `{"password":"pw"}`)
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
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"000000","password":"pw"}`); rec.Code == http.StatusOK {
		t.Error("disable accepted a wrong code")
	}
	// A right code with no password → also rejected. Password alone used to be
	// enough to strip the factor, which is the leaked-password case 2FA exists
	// for; a code alone is the hijacked-session case.
	codeNoPw, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"`+codeNoPw+`"}`); rec.Code == http.StatusOK {
		t.Error("disable accepted a code with no password confirmation")
	}
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"password":"pw"}`); rec.Code == http.StatusOK {
		t.Error("disable accepted a password with no second-factor code")
	}
	if rec := do(srv, "GET", "/api/auth/2fa/status", cookies, ""); !strings.Contains(rec.Body.String(), "true") {
		t.Fatalf("2FA came off despite every rejected attempt: %s", rec.Body.String())
	}
	// Disable with a valid code → off.
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"`+code2+`","password":"pw"}`); rec.Code != http.StatusOK {
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

// TestTwoFALoginFlow covers the full second-factor login: password alone defers,
// a wrong code is refused, and the right code completes the session.
func TestTwoFALoginFlow(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Enroll 2FA.
	// Enrolling and un-enrolling both re-authenticate: a live session is not
	// enough authority to touch the second factor (requirePasswordStepUp).
	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, `{"password":"pw"}`)
	var setup struct {
		Secret string `json:"secret"`
	}
	json.Unmarshal(rec.Body.Bytes(), &setup)
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/enable", cookies, `{"code":"`+code+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d", rec.Code)
	}

	// Password login now defers (no session cookie, twoFactorRequired).
	rec = do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "twoFactorRequired") {
		t.Fatalf("login should require 2FA: %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("2FA-deferred login must not set a session cookie")
	}

	// Wrong code → 401.
	if rec := do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"admin","password":"pw","code":"000000"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong code: got %d, want 401", rec.Code)
	}
	// Right code → 200 + session.
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"admin","password":"pw","code":"`+code2+`"}`)
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("valid verify: got %d, cookies=%d", rec.Code, len(rec.Result().Cookies()))
	}
}

// TestReadEndpoints covers simple authenticated read handlers: me, health, audit log.
func TestReadEndpoints(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)
	db.Create(&models.AuditLog{OrgID: 1, Action: "test.action"})

	for _, ep := range []string{"/api/auth/me", "/api/health", "/api/audit"} {
		if rec := do(srv, "GET", ep, cookies, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d (%s)", ep, rec.Code, rec.Body.String())
		}
	}
	// me without a session → 401.
	if rec := do(srv, "GET", "/api/auth/me", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauth me: got %d, want 401", rec.Code)
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
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "oct_") {
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

// TestCRUDErrorBranches sweeps the common error paths (invalid JSON body, bad
// non-numeric id, missing id) across CRUD resources — the branches unit happy
// paths skip.
func TestCRUDErrorBranches(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Invalid JSON body → 400 on create endpoints.
	for _, res := range []string{"/api/webhooks", "/api/tokens"} {
		if rec := do(srv, "POST", res, cookies, `{`); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s invalid body: got %d, want 400", res, rec.Code)
		}
	}
	// Non-numeric id → 400 on delete.
	for _, res := range []string{"/api/tokens/"} {
		if rec := do(srv, "DELETE", res+"abc", cookies, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("DELETE %sabc: got %d, want 400", res, rec.Code)
		}
	}
	// Missing resource → 404 on delete.
	for _, res := range []string{"/api/tokens/"} {
		if rec := do(srv, "DELETE", res+"88888", cookies, ""); rec.Code != http.StatusNotFound {
			t.Errorf("DELETE %s88888: got %d, want 404", res, rec.Code)
		}
	}
}

// TestExtractBounceEvents covers the multi-provider bounce parser directly.
