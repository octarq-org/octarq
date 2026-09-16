package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notify"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/dns"
	"github.com/octarq-org/octarq/server/plugins/links"
	"github.com/octarq-org/octarq/server/plugins/mail"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mountCoreDNS mounts the built-in dns Core plugin onto the handler's API so the
// domain / provider-account / DNS-record routes (extracted out of the Handler,
// see website/src/content/docs/architecture/overview.md) are present in the
// test server exactly as the app mounts them in production. Uses default (net)
// that stub resolution live in plugins/dns.
func mountCoreDNS(h *Handler, db *gorm.DB, authMgr *auth.Manager, cipher *crypto.Cipher) {
	reg := plugin.NewRegistry()
	dns.New().Mount(nil, &plugin.Context{
		Huma:        h.Huma(),
		DB:          db,
		OrgID:       authMgr.OrgID,
		Audit:       h.Audit,
		Encrypt:     cipher.Encrypt,
		Decrypt:     cipher.Decrypt,
		Provide:     reg.Provide,
		Lookup:      reg.Lookup,
		RequireRole: func(*http.Request, string) bool { return true },
		RequirePerm: func(*http.Request, string, string) bool { return true },
	})
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
		RequireRole:         func(*http.Request, string) bool { return true },
		RequirePerm:         func(*http.Request, string, string) bool { return true },
	})
}

func newTestHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	_, srv, db := newTestHandlerRaw(t)
	return srv, db
}

// disableEmailVerification turns the (default-on) require_email_verification
// setting off so tests exercising register-then-login against a fresh handler
// keep the pre-gate behavior. Explicit opt-out: nothing may rely on the
// verification default being off.
func disableEmailVerification(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "false"}).Error; err != nil {
		t.Fatalf("set require_email_verification=false: %v", err)
	}
}

var (
	testDBTemplateOnce sync.Once
	testDBFullDDL      string
	testDBNoMailDDL    string
)

func initTestDBTemplates() {
	testDBTemplateOnce.Do(func() {
		fullDB, err := gorm.Open(sqlite.Open("file:template_full_init?mode=memory&cache=shared"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			panic("initTestDBTemplates fullDB: " + err.Error())
		}
		fullModels := append(models.AllModels(),
			&links.Link{}, &links.LinkEvent{},
			&dns.Domain{}, &dns.ProviderAccount{}, &dns.DDNSToken{},
			&mail.Mailbox{}, &mail.Email{}, &mail.SMTPSender{},
		)
		if err := fullDB.AutoMigrate(fullModels...); err != nil {
			panic("initTestDBTemplates migrate fullDB: " + err.Error())
		}
		var fullStmts []string
		if err := fullDB.Raw("SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY type DESC").Scan(&fullStmts).Error; err != nil {
			panic("initTestDBTemplates read full master: " + err.Error())
		}
		ddl := strings.Join(fullStmts, ";\n") + ";"
		ddl = strings.ReplaceAll(ddl, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ")
		ddl = strings.ReplaceAll(ddl, "CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ")
		ddl = strings.ReplaceAll(ddl, "CREATE UNIQUE INDEX ", "CREATE UNIQUE INDEX IF NOT EXISTS ")
		testDBFullDDL = ddl

		noMailDB, err := gorm.Open(sqlite.Open("file:template_nomail_init?mode=memory&cache=shared"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			panic("initTestDBTemplates noMailDB: " + err.Error())
		}
		noMailModels := append(models.AllModels(),
			&links.Link{}, &links.LinkEvent{},
			&dns.Domain{}, &dns.ProviderAccount{}, &dns.DDNSToken{},
		)
		if err := noMailDB.AutoMigrate(noMailModels...); err != nil {
			panic("initTestDBTemplates migrate noMailDB: " + err.Error())
		}
		var noMailStmts []string
		if err := noMailDB.Raw("SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY type DESC").Scan(&noMailStmts).Error; err != nil {
			panic("initTestDBTemplates read noMail master: " + err.Error())
		}
		nmDDL := strings.Join(noMailStmts, ";\n") + ";"
		nmDDL = strings.ReplaceAll(nmDDL, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ")
		nmDDL = strings.ReplaceAll(nmDDL, "CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ")
		nmDDL = strings.ReplaceAll(nmDDL, "CREATE UNIQUE INDEX ", "CREATE UNIQUE INDEX IF NOT EXISTS ")
		testDBNoMailDDL = nmDDL
	})
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	initTestDBTemplates()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Exec(testDBFullDDL).Error; err != nil {
		t.Fatalf("apply test db template: %v", err)
	}
	return db
}

// newTestHandlerRaw is newTestHandler plus the *Handler itself, for tests that
// exercise handler methods directly rather than over HTTP.
func newTestHandlerRaw(t *testing.T) (*Handler, http.Handler, *gorm.DB) {
	t.Helper()
	return newTestHandlerRawCfg(t, &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"})
}

// newTestHandlerRawCfg is newTestHandlerRaw with an explicit config, for tests
// that need sentinel values (e.g. proving a response omits secrets) or a
// specific driver/DSN the default handler cannot express.
func newTestHandlerRawCfg(t *testing.T, cfg *config.Config) (*Handler, http.Handler, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	// Isolate from other tests sharing the cache.
	db.Where("1 = 1").Delete(&models.Token{})
	db.Where("1 = 1").Delete(&links.Link{})

	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	notify.SetConfigDecryptor(func(stored string) (string, bool) {
		b, err := cipher.Decrypt(stored)
		if err != nil {
			return "", false
		}
		return string(b), true
	})
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))

	dnsP := dns.New()
	mailP := mail.New()
	linksP := links.New()
	h.SetPlugins([]plugin.Plugin{dnsP, mailP, linksP})

	reg := plugin.NewRegistry()
	h.SetServiceLookup(reg.Lookup)

	srv := h.Routes()

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
		Provide:             reg.Provide,
		Lookup:              reg.Lookup,
		RequireRole:         func(*http.Request, string) bool { return true },
		RequirePerm:         h.RequirePerm,
	}
	dnsP.Mount(nil, pctx)
	mailP.Mount(nil, pctx)
	linksP.Mount(nil, pctx)

	return h, srv, db
}

// newTestHandlerWithoutMail builds a test handler with only dns and links mounted
// (no mail plugin), mirroring the edition-nomail composition.
func newTestHandlerWithoutMail(t *testing.T) (*Handler, http.Handler, *gorm.DB) {
	t.Helper()
	initTestDBTemplates()
	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "_nomail?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Exec(testDBNoMailDDL).Error; err != nil {
		t.Fatalf("apply nomail template: %v", err)
	}
	db.Where("1 = 1").Delete(&models.Token{})
	db.Where("1 = 1").Delete(&links.Link{})

	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	notify.SetConfigDecryptor(func(stored string) (string, bool) {
		b, err := cipher.Decrypt(stored)
		if err != nil {
			return "", false
		}
		return string(b), true
	})
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))

	dnsP := dns.New()
	linksP := links.New()
	h.SetPlugins([]plugin.Plugin{dnsP, linksP})

	reg := plugin.NewRegistry()
	h.SetServiceLookup(reg.Lookup)

	srv := h.Routes()

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
		Provide:             reg.Provide,
		Lookup:              reg.Lookup,
		RequireRole:         func(*http.Request, string) bool { return true },
		RequirePerm:         h.RequirePerm,
	}
	dnsP.Mount(nil, pctx)
	linksP.Mount(nil, pctx)

	return h, srv, db
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
