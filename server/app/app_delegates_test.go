package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/internal/notify"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/builtin"
	"gorm.io/gorm"
)

// pluginEnabledStub stands in for the API handler's PluginEnabled in pluginGate
// tests.
type pluginEnabledStub struct {
	enabled func(orgID uint, key string) bool
}

func (s pluginEnabledStub) PluginEnabled(orgID uint, key string) bool {
	return s.enabled(orgID, key)
}

// TestPluginGate pins the org-0 contract (not workspace-scoped, never gated) and
// the delegation to the API handler's PluginEnabled for a real workspace.
func TestPluginGate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := &config.Config{SecretKey: "secret", AdminUser: "admin", AdminPassword: "pw"}
	a := &App{auth: auth.New(cfg, crypto.New("secret")).WithDB(db)}

	gate := a.pluginGate(pluginEnabledStub{enabled: func(orgID uint, key string) bool { return true }})

	// No workspace in session -> not scoped, and the route stays unblocked.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if allowed, scoped := gate(r, "x"); allowed || scoped {
		t.Errorf("org-0 gate = (%v, %v), want (false, false)", allowed, scoped)
	}

	// A workspace present is gated through the API handler.
	r = r.WithContext(plugin.WithOrgID(r.Context(), 7))
	if allowed, scoped := gate(r, "x"); !allowed || !scoped {
		t.Errorf("org-7 gate = (%v, %v), want (true, true)", allowed, scoped)
	}

	gateOff := a.pluginGate(pluginEnabledStub{enabled: func(orgID uint, key string) bool { return false }})
	r = r.WithContext(plugin.WithOrgID(r.Context(), 9))
	if allowed, scoped := gateOff(r, "x"); allowed || !scoped {
		t.Errorf("org-9 disabled gate = (%v, %v), want (false, true)", allowed, scoped)
	}
}

// TestRecoverWriterFlushAndWrite exercises the stream-forwarding half of
// recoverWriter: headers/writes mark the written flag, Flush forwards to a
// flusher and is a no-op without one.
func TestRecoverWriterFlushAndWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &recoverWriter{ResponseWriter: rec}

	if n, err := rw.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("Write = (%d, %v), want (3, nil)", n, err)
	}
	if !rw.wroteHeader {
		t.Error("Write did not mark wroteHeader")
	}
	if rec.Body.String() != "abc" {
		t.Errorf("Write did not forward the body: %q", rec.Body.String())
	}

	// WriteHeader forwards, but the recorder has already implicitly written 200
	// on the Write above — a fresh writer records it directly.
	rec2 := httptest.NewRecorder()
	rw2 := &recoverWriter{ResponseWriter: rec2}
	rw2.WriteHeader(http.StatusTeapot)
	if rec2.Code != http.StatusTeapot {
		t.Errorf("WriteHeader did not forward to the underlying writer: got %d", rec2.Code)
	}

	// Flush forwards when the underlying writer implements http.Flusher.
	rw.Flush()
	if !rec.Flushed {
		t.Error("Flush did not reach the underlying http.Flusher")
	}

	// Flush without an http.Flusher must be a safe no-op.
	plain := &plainResponseWriter{new(bytes.Buffer)}
	rw3 := &recoverWriter{ResponseWriter: plain}
	rw3.Flush()
}

type plainResponseWriter struct{ buf *bytes.Buffer }

func (p *plainResponseWriter) Header() http.Header         { return http.Header{} }
func (p *plainResponseWriter) Write(b []byte) (int, error) { return p.buf.Write(b) }
func (p *plainResponseWriter) WriteHeader(int)             {}

// TestGatedMuxHandle covers the Handle method (as opposed to HandleFunc), which
// registers a gated handler under an exact pattern.
func TestGatedMuxHandle(t *testing.T) {
	real := http.NewServeMux()
	gm := &gatedMux{
		real:   real,
		plugin: "fake",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return true, true
		},
	}
	gm.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	real.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed Handle route = %d, want 200", rec.Code)
	}

	gmDis := &gatedMux{
		real:   real,
		plugin: "fake-dis",
		enabled: func(_ *http.Request, _ string) (bool, bool) {
			return false, true
		},
	}
	gmDis.Handle("/blocked", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	recDis := httptest.NewRecorder()
	real.ServeHTTP(recDis, httptest.NewRequest(http.MethodGet, "/blocked", nil))
	if recDis.Code != http.StatusNotFound {
		t.Fatalf("disabled Handle route = %d, want 404", recDis.Code)
	}
}

// TestGatedAdapterPanic verifies a panicking Huma handler registered through
// the gated adapter is recovered into a 500 JSON response instead of crashing
// the process.
func TestGatedAdapterPanic(t *testing.T) {
	realMux := http.NewServeMux()
	realAPI := humago.New(realMux, huma.DefaultConfig("Test API", "1.0.0"))
	gAPI := &gatedAPI{
		API: realAPI,
		gAdapter: &gatedAdapter{
			Adapter: realAPI.Adapter(),
			plugin:  "crash",
			enabled: func(_ *http.Request, _ string) (bool, bool) { return true, true },
		},
	}
	type In struct{}
	type Out struct{ Body struct{} }
	huma.Register(gAPI, huma.Operation{Method: "GET", Path: "/api/boom-huma"}, func(ctx context.Context, in *In) (*Out, error) {
		panic("huma boom")
	})

	rec := httptest.NewRecorder()
	realMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/boom-huma", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panicking huma route = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q, want application/json", ct)
	}
}

// appAuthDB builds an App with an in-memory auth manager and migrated schema
// for the login delegate tests.
func appAuthDB(t *testing.T) (*App, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:appauth-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Drop the shared in-memory DB on cleanup so -count=2 re-runs stay isolated.
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{SecretKey: "secret", AdminUser: "admin", AdminPassword: "pw"}
	c := crypto.New("secret")
	m := auth.New(cfg, c).WithDB(db)
	return &App{cfg: cfg, cipher: c, auth: m}, db
}

func seedIdentityUser(t *testing.T, db *gorm.DB) (models.User, models.Org) {
	t.Helper()
	slug, err := models.AllocateOrgSlug(db)
	if err != nil {
		t.Fatalf("slug: %v", err)
	}
	u := models.User{Email: "sso@acme.com", EmailVerified: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	o := models.Org{Name: "Acme", Slug: slug, InboundToken: "tok"}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("org: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: o.ID, UserID: u.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("member: %v", err)
	}
	return u, o
}

// TestLoginDelegates maps the app-level login methods onto the auth manager:
// registration-disabled and account-link refusals must translate to the
// plugin-package sentinels, and a bound identity completes a session.
func TestLoginDelegates(t *testing.T) {
	t.Run("registration disabled maps to plugin sentinel", func(t *testing.T) {
		a, db := appAuthDB(t)
		if err := db.Create(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
			t.Fatalf("setting: %v", err)
		}
		id := plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://idp.acme.com", Subject: "s1",
			Email: "new@acme.com", MayCreateUser: true,
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := a.loginByIdentity(rec, req, id)
		if !errors.Is(err, plugin.ErrLoginRegistrationDisabled) {
			t.Fatalf("loginByIdentity disabled = %v, want plugin.ErrLoginRegistrationDisabled", err)
		}
		_, err = a.loginByEmail(rec, req, "stranger@acme.com")
		if !errors.Is(err, plugin.ErrLoginRegistrationDisabled) {
			t.Fatalf("loginByEmail disabled = %v, want plugin.ErrLoginRegistrationDisabled", err)
		}
	})

	t.Run("account link required maps to plugin sentinel", func(t *testing.T) {
		a, db := appAuthDB(t)
		seedIdentityUser(t, db)
		id := plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://evil.example", Subject: "s2",
			Email: "sso@acme.com", MayCreateUser: true,
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := a.loginByIdentity(rec, req, id)
		if !errors.Is(err, plugin.ErrAccountLinkRequired) {
			t.Fatalf("loginByIdentity = %v, want plugin.ErrAccountLinkRequired", err)
		}
	})

	t.Run("bound identity succeeds and issues a session", func(t *testing.T) {
		a, db := appAuthDB(t)
		u, o := seedIdentityUser(t, db)
		id := plugin.ExternalIdentity{
			Provider: "oidc", Issuer: "https://idp.acme.com", Subject: "s3",
			Email: "sso@acme.com", OrgID: o.ID,
		}
		if err := db.Create(&models.UserIdentity{UserID: u.ID, Provider: "oidc", Issuer: "https://idp.acme.com", Subject: "s3", Email: "sso@acme.com"}).Error; err != nil {
			t.Fatalf("bind: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		uid, err := a.loginByIdentity(rec, req, id)
		if err != nil {
			t.Fatalf("loginByIdentity: %v", err)
		}
		if uid != u.ID {
			t.Errorf("uid = %d, want %d", uid, u.ID)
		}
		if len(rec.Result().Cookies()) == 0 {
			t.Error("login issued no session cookie")
		}
	})
}

// TestAppNotify routes through the notify registry: an unknown channel type is
// refused, and a registered provider receives the message text.
func TestAppNotify(t *testing.T) {
	a := &App{}
	ctx := context.Background()

	// notify.Send decrypts stored channel config before dispatch; a bare App{}
	// has no decryptor registered, so install a passthrough for this test.
	notification.SetConfigDecryptor(func(stored string) (string, bool) { return stored, true })
	defer notification.SetConfigDecryptor(nil)

	// Test decrypt error branch
	notification.SetConfigDecryptor(func(stored string) (string, bool) { return "", false })
	if err := a.Notify(ctx, "apptest-chan", "garbled", "hi"); err == nil {
		t.Fatal("expected error on config decryption failure, got nil")
	}
	notification.SetConfigDecryptor(func(stored string) (string, bool) { return stored, true })

	if err := a.Notify(ctx, "apptest-nosuch", "{}", "hi"); err == nil {
		t.Fatal("Notify accepted an unregistered channel type")
		return
	}

	var gotText string
	notify.Register("apptest-chan", func(_ context.Context, _, text string) error {
		gotText = text
		return nil
	})
	if err := a.Notify(ctx, "apptest-chan", "{}", "hello world"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotText != "hello world" {
		t.Errorf("provider received %q, want hello world", gotText)
	}
}

// fakeDNSManager satisfies plugin.DNSManager for the lazy-resolver happy path.
type fakeDNSManager struct{}

func (fakeDNSManager) List(context.Context, uint, uint) ([]plugin.DNSRecord, error) {
	return nil, nil
}
func (fakeDNSManager) Set(context.Context, uint, uint, plugin.DNSRecord) (plugin.DNSRecord, error) {
	return plugin.DNSRecord{}, nil
}
func (fakeDNSManager) Delete(context.Context, uint, uint, string) error { return nil }

// TestLazyDNSResolution covers resolve()'s type-assertion guard and the three
// delegated operations through a resolvable manager.
func TestLazyDNSResolution(t *testing.T) {
	ctx := context.Background()
	missing := &lazyDNSManager{lookup: func(string) (any, bool) { return nil, false }}
	if _, err := missing.List(ctx, 1, 1); err == nil {
		t.Error("List on unmounted manager = nil")
	}

	wrongType := &lazyDNSManager{lookup: func(string) (any, bool) { return "not-a-dns-manager", true }}
	if _, err := wrongType.List(ctx, 1, 1); err == nil {
		t.Error("List with a wrongly-typed service = nil")
	}

	ok := &lazyDNSManager{lookup: func(string) (any, bool) { return fakeDNSManager{}, true }}
	if _, err := ok.List(ctx, 1, 1); err != nil {
		t.Errorf("List: %v", err)
	}
	if _, err := ok.Set(ctx, 1, 1, plugin.DNSRecord{}); err != nil {
		t.Errorf("Set: %v", err)
	}
	if err := ok.Delete(ctx, 1, 1, "r1"); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

// TestBuiltinDefaultMonitor ensures the default open-core composition used by
// main() still wires through the mount loop used by Run.
func TestBuiltinDefaultMonitor(t *testing.T) {
	a := bootApp(t)
	for _, p := range builtin.Default() {
		a.Use(p)
	}
	if len(a.Plugins()) != len(builtin.Default()) {
		t.Fatalf("Use did not accumulate %d plugins", len(builtin.Default()))
	}
}
