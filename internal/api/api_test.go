package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jungley/led/config"
	"github.com/jungley/led/internal/auth"
	"github.com/jungley/led/internal/crypto"
	"github.com/jungley/led/internal/geo"
	"github.com/jungley/led/internal/models"
	"gorm.io/gorm"
)

func newTestHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g)
	return h.Routes(), db
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
	sessionMgr.SetSession(cookieRec, models.SingleUserID)
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
