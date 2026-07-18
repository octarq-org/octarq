package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
	"github.com/octarq-org/octarq/plugins/mail"
	"gorm.io/gorm"
)

// mountCoreDNS mounts the built-in dns Core plugin onto the handler's API so the
// domain / provider-account / DNS-record routes (extracted out of the Handler,
// see docs/CORE-PLUGIN-EXTRACTION.md) are present in the test server exactly as
// the app mounts them in production. Uses default (net) DNS resolvers — tests
// that stub resolution live in plugins/dns.
func mountCoreDNS(h *Handler, db *gorm.DB, authMgr *auth.Manager, cipher *crypto.Cipher) {
	reg := plugin.NewRegistry()
	dns.New().Mount(nil, &plugin.Context{
		Huma:    h.Huma(),
		DB:      db,
		OrgID:   authMgr.OrgID,
		Audit:   h.Audit,
		Encrypt: cipher.Encrypt,
		Decrypt: cipher.Decrypt,
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	})
}

func mountCoreLinks(h *Handler, db *gorm.DB, authMgr *auth.Manager, cipher *crypto.Cipher) {
	pctx := &plugin.Context{
		Huma:                h.Huma(),
		DB:                  db,
		Guard:               authMgr.Require,
		UserID:              authMgr.UserID,
		OrgID:               authMgr.OrgID,
		Audit:               h.Audit,
		Encrypt:             cipher.Encrypt,
		Decrypt:             cipher.Decrypt,
		GetGlobalSetting:    h.GetGlobalSetting,
		GetWorkspaceSetting: h.GetWorkspaceSetting,
		Enqueue:             h.queue.Enqueue,
		DeleteCache:         authMgr.Cache().Delete,
	}
	links.New().Mount(nil, pctx)
}

func mountCoreMail(h *Handler, db *gorm.DB, authMgr *auth.Manager, cipher *crypto.Cipher) {
	reg := plugin.NewRegistry()
	mail.New().Mount(nil, &plugin.Context{
		Huma:                h.Huma(),
		DB:                  db,
		OrgID:               authMgr.OrgID,
		Audit:               h.Audit,
		Encrypt:             cipher.Encrypt,
		Decrypt:             cipher.Decrypt,
		GetWorkspaceSetting: h.GetWorkspaceSetting,
		GetGlobalSetting:    h.GetGlobalSetting,
		Provide:             reg.Provide,
		Lookup:              reg.Lookup,
	})
}

func newTestHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Isolate from other tests sharing the cache.
	db.Where("1 = 1").Delete(&models.Token{})
	db.Where("1 = 1").Delete(&models.Link{})

	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	srv := h.Routes()
	mountCoreDNS(h, db, authMgr, cipher)
	mountCoreMail(h, db, authMgr, cipher)
	mountCoreLinks(h, db, authMgr, cipher)
	return srv, db
}

// apiEnvStore backs crypto.EnableEnvelope with the test DB's settings table.
type apiEnvStore struct{ db *gorm.DB }

func (s apiEnvStore) Get(key string) (string, bool) {
	var row models.Setting
	if s.db.First(&row, "key = ?", key).Error != nil {
		return "", false
	}
	return row.Value, true
}

func (s apiEnvStore) Set(key, val string) error {
	return s.db.Save(&models.Setting{Key: key, Value: val}).Error
}

func TestTokenLifecycleAndBearerAuth(t *testing.T) {
	srv, _ := newTestHandler(t)

	// Unauthenticated create is rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"ci"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth create: got %d want 401", rec.Code)
	}

	cookies := sessionCookies(t, 1, 1)

	req = httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"ci","note":"ci use"}`))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: got %d want 201 (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !strings.HasPrefix(created.Token, "oct_") {
		t.Fatalf("raw token not returned: %q", created.Token)
	}

	// The list endpoint must never expose the raw token or hash.
	req = httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tokens: got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), created.Token) {
		t.Error("token list leaked the raw token")
	}
	if strings.Contains(rec.Body.String(), "hash") {
		t.Error("token list leaked the hash field")
	}

	// Use the bearer token against a protected data endpoint.
	req = httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer-authed /api/links: got %d want 200", rec.Code)
	}

	// A bad bearer token is rejected.
	req = httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.Header.Set("Authorization", "Bearer oct_totallybogus0000000000000000000000")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad bearer token: got %d want 401", rec.Code)
	}
}

func TestAPIVersioningRewrite(t *testing.T) {
	srv, _ := newTestHandler(t)

	// A request to /api/v1/health should rewrite to /api/health and succeed
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("URL rewrite failed: got code %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestEmailBounceWebhook(t *testing.T) {
	srv, db := newTestHandler(t)

	// The webhook is tenant-scoped + token-authed: create the org, then a mailbox
	// under it, and post to /api/webhook/{slug}/email/bounce/{token}.
	org := models.Org{Name: "Acme", Slug: "acme", InboundToken: "btok"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	mb := models.Mailbox{
		OrgID:   org.ID,
		Address: "bounced@example.com",
		Enabled: true,
	}
	if err := db.Create(&mb).Error; err != nil {
		t.Fatalf("failed to create test mailbox: %v", err)
	}
	bounceURL := "/api/webhook/acme/email/bounce/btok"

	// 0. Unauthenticated (bad token) → 401; unknown org → 404.
	for _, tc := range []struct {
		url  string
		want int
	}{
		{"/api/webhook/acme/email/bounce/wrong", http.StatusUnauthorized},
		{"/api/webhook/nope/email/bounce/btok", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader("[]")))
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.url, rec.Code, tc.want)
		}
	}

	// SSRF guard: an SNS SubscriptionConfirmation with a non-AWS SubscribeURL is rejected.
	rec0 := httptest.NewRecorder()
	srv.ServeHTTP(rec0, httptest.NewRequest(http.MethodPost, bounceURL,
		strings.NewReader(`{"Type":"SubscriptionConfirmation","SubscribeURL":"http://169.254.169.254/latest/meta-data/"}`)))
	if rec0.Code != http.StatusBadRequest {
		t.Errorf("SSRF SubscribeURL: got %d, want 400", rec0.Code)
	}

	// 1. Test SendGrid style bounce payload
	payload := `[{"email":"bounced@example.com","event":"bounce","reason":"550 Invalid recipient","status":"5.1.1"}]`
	req := httptest.NewRequest(http.MethodPost, bounceURL, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bounce webhook failed: got code %d, body %s", rec.Code, rec.Body.String())
	}

	// Verify audit log entry
	var logs []models.AuditLog
	if err := db.Where("action = ? AND target_id = ?", "email.bounce", mb.ID).Find(&logs).Error; err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if !strings.Contains(logs[0].Meta, "550 Invalid recipient") {
		t.Errorf("meta does not contain reason: %s", logs[0].Meta)
	}

	// 2. Test AWS SES style bounce payload
	sesPayload := `{
		"notificationType": "Bounce",
		"bounce": {
			"bounceType": "Permanent",
			"bounceSubType": "General",
			"bouncedRecipients": [
				{
					"emailAddress": "bounced@example.com"
				}
			]
		}
	}`
	req = httptest.NewRequest(http.MethodPost, bounceURL, strings.NewReader(sesPayload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("SES bounce webhook failed: got code %d, body %s", rec.Code, rec.Body.String())
	}

	// Verify audit log entry
	if err := db.Where("action = ? AND target_id = ?", "email.bounce", mb.ID).Find(&logs).Error; err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit logs, got %d", len(logs))
	}
}
