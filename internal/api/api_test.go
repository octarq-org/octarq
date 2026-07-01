package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/auth"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/geo"
	"github.com/Jungley8/led/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

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
	h := New(cfg, db, cipher, authMgr, g)
	return h.Routes(), db
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

	// Create a token with a session cookie.
	cfg := &config.Config{SecretKey: "secret"}
	sessionMgr := auth.New(cfg, crypto.New("secret"))
	cookieRec := httptest.NewRecorder()
	sessionMgr.SetSession(cookieRec, 1, 1)
	cookies := cookieRec.Result().Cookies()

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
	if !strings.HasPrefix(created.Token, "led_") {
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
	req.Header.Set("Authorization", "Bearer led_totallybogus0000000000000000000000")
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

	// Create test mailbox
	mb := models.Mailbox{
		OrgID:   1,
		Address: "bounced@example.com",
		Enabled: true,
	}
	if err := db.Create(&mb).Error; err != nil {
		t.Fatalf("failed to create test mailbox: %v", err)
	}

	// 1. Test SendGrid style bounce payload
	payload := `[{"email":"bounced@example.com","event":"bounce","reason":"550 Invalid recipient","status":"5.1.1"}]`
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/email/bounce", strings.NewReader(payload))
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
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/email/bounce", strings.NewReader(sesPayload))
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
