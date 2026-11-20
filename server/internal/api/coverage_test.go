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

func TestLogoutClearsCookie(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)
	rec := do(srv, "POST", "/api/auth/logout", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: got %d", rec.Code)
	}
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

func TestOrgSwitchAndMembers(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/orgs", cookies, `{"name":"Second Org"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org: got %d (%s)", rec.Code, rec.Body.String())
	}
	var org struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &org)

	if rec := do(srv, "POST", "/api/auth/switch-org", cookies, `{"orgId":`+strconv.FormatUint(uint64(org.ID), 10)+`}`); rec.Code != http.StatusOK {
		t.Errorf("switch to own org: got %d", rec.Code)
	}
	if rec := do(srv, "POST", "/api/auth/switch-org", cookies, `{"orgId":99999}`); rec.Code != http.StatusForbidden {
		t.Errorf("switch to foreign org: got %d, want 403", rec.Code)
	}

	rec = do(srv, "GET", "/api/org/members", cookies, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "admin") {
		t.Errorf("list members: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestTwoFADisableAndStatus(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	if rec := do(srv, "GET", "/api/auth/2fa/status", cookies, ""); !strings.Contains(rec.Body.String(), "false") {
		t.Fatalf("status before: %s", rec.Body.String())
	}

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

	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"000000","password":"pw"}`); rec.Code == http.StatusOK {
		t.Error("disable accepted a wrong code")
	}
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
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/disable", cookies, `{"code":"`+code2+`","password":"pw"}`); rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "GET", "/api/auth/2fa/status", cookies, ""); !strings.Contains(rec.Body.String(), "false") {
		t.Errorf("status after disable: %s", rec.Body.String())
	}
}

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
	if rec := do(srv, "PUT", "/api/org", cookies, `{"name":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want 400", rec.Code)
	}
}

func TestTwoFALoginFlow(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/auth/2fa/setup", cookies, `{"password":"pw"}`)
	var setup struct {
		Secret string `json:"secret"`
	}
	json.Unmarshal(rec.Body.Bytes(), &setup)
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if rec := do(srv, "POST", "/api/auth/2fa/enable", cookies, `{"code":"`+code+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d", rec.Code)
	}

	rec = do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "twoFactorRequired") {
		t.Fatalf("login should require 2FA: %d (%s)", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("2FA-deferred login must not set a session cookie")
	}

	if rec := do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"admin","password":"pw","code":"000000"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong code: got %d, want 401", rec.Code)
	}
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	rec = do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"admin","password":"pw","code":"`+code2+`"}`)
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("valid verify: got %d, cookies=%d", rec.Code, len(rec.Result().Cookies()))
	}
}

func TestReadEndpoints(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)
	db.Create(&models.AuditLog{OrgID: 1, Action: "test.action"})

	for _, ep := range []string{"/api/auth/me", "/api/health", "/api/audit"} {
		if rec := do(srv, "GET", ep, cookies, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d (%s)", ep, rec.Code, rec.Body.String())
		}
	}
	if rec := do(srv, "GET", "/api/auth/me", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauth me: got %d, want 401", rec.Code)
	}
}

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
	if rec := do(srv, "DELETE", "/api/tokens/99999", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing token: got %d, want 404", rec.Code)
	}
}

func TestCRUDErrorBranches(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	for _, res := range []string{"/api/webhooks", "/api/tokens"} {
		if rec := do(srv, "POST", res, cookies, `{`); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s invalid body: got %d, want 400", res, rec.Code)
		}
	}
	for _, res := range []string{"/api/tokens/"} {
		if rec := do(srv, "DELETE", res+"abc", cookies, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("DELETE %sabc: got %d, want 400", res, rec.Code)
		}
	}
	for _, res := range []string{"/api/tokens/"} {
		if rec := do(srv, "DELETE", res+"88888", cookies, ""); rec.Code != http.StatusNotFound {
			t.Errorf("DELETE %s88888: got %d, want 404", res, rec.Code)
		}
	}
}
