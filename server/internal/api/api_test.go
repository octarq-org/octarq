package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/builtin"
	"github.com/octarq-org/octarq/server/plugins/vault"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	_, srv, db := newTestHandlerRaw(t)
	return srv, db
}

// disableEmailVerification turns the (default-on) require_email_verification
// setting off so tests exercising register-then-login against a fresh handler
// keep the pre-gate behavior. Explicit opt-out: nothing may rely on the
// verification default being off.
//
// It no longer makes registration work: sign-up always ends in the mailbox,
// so a test that registers needs enableMail as well.
func disableEmailVerification(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "false"}).Error; err != nil {
		t.Fatalf("set require_email_verification=false: %v", err)
	}
}

// enableMail seeds an SMTP sender so the instance can deliver system mail.
// Registration answers "a message is on its way" whatever it finds, so an
// instance that cannot send mail refuses sign-ups outright (503) rather than
// promising a message that never arrives.
func enableMail(t *testing.T, db *gorm.DB) {
	t.Helper()
	sender := testSMTPSender{
		OrgID: 1, Name: "test", Host: "smtp.example.com", Port: 587,
		User: "u", Pass: "enc", FromEmail: "noreply@example.com",
	}
	if err := db.Create(&sender).Error; err != nil {
		t.Fatalf("seed smtp sender: %v", err)
	}
	// System mail reads the explicitly selected sender only; readiness must
	// answer for that same row or sign-ups are promised mail that never sends.
	if err := db.Save(&models.Setting{Key: "mail_system_sender_id", Value: strconv.FormatUint(uint64(sender.ID), 10)}).Error; err != nil {
		t.Fatalf("set mail_system_sender_id: %v", err)
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
			&testLink{},
			&testDomain{}, &testProviderAccount{},
			&testMailbox{}, &testEmail{}, &testSMTPSender{},
		)
		fullModels = append(fullModels, vault.New().Models()...)
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
			&testLink{},
			&testDomain{}, &testProviderAccount{},
		)
		noMailModels = append(noMailModels, vault.New().Models()...)
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
	db.Where("1 = 1").Delete(&testLink{})

	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))

	// The chat resolves the published harness through the plugin host. A host
	// with no harness provider means the workspace published nothing, which
	// uses the built-in default.
	h.SetHost(&plugin.TestHost{})

	plugins := builtin.Default()
	h.SetPlugins(plugins)

	reg := plugin.NewRegistry()
	h.SetServiceLookup(reg.Lookup)

	reg.Provide(plugin.ServiceMailReady, plugin.MailReady(func() bool {
		var setting models.Setting
		if err := db.First(&setting, "key = ?", "mail_system_sender_id").Error; err != nil || setting.Value == "" {
			return false
		}
		var sender testSMTPSender
		return db.First(&sender, "id = ?", setting.Value).Error == nil
	}))
	reg.Provide(plugin.ServiceMailSendSystem, plugin.SystemMailSender(func(to, subject, html, text string) error {
		var setting models.Setting
		if err := db.First(&setting, "key = ?", "mail_system_sender_id").Error; err != nil || setting.Value == "" {
			return errors.New("no system sender configured")
		}
		var sender testSMTPSender
		if err := db.First(&sender, "id = ?", setting.Value).Error; err != nil {
			return errors.New("configured system sender not found")
		}
		pass, err := cipher.Decrypt(sender.Pass)
		if err != nil {
			return err
		}
		var msg strings.Builder
		fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
			sender.FromEmail, to, subject, text)
		addr := net.JoinHostPort(sender.Host, strconv.Itoa(sender.Port))
		return smtp.SendMail(addr, smtp.PlainAuth("", sender.User, string(pass), sender.Host), sender.FromEmail, []string{to}, []byte(msg.String()))
	}))
	reg.Provide(plugin.ServiceLinkResolve, plugin.LinkResolver(func(host, slug string) (string, uint, bool) {
		var l testLink
		if err := db.Where("slug = ?", slug).First(&l).Error; err == nil {
			return l.Target, l.OrgID, true
		}
		return "", 0, false
	}))
	reg.Provide(plugin.ServiceMailEmailGet, plugin.EmailGetter(func(orgID uint, id uint) (string, string, string, bool) {
		var em testEmail
		if err := db.Where("id = ?", id).First(&em).Error; err != nil {
			return "", "", "", false
		}
		return em.FromAddr, em.Subject, em.Text, true
	}))

	srv := h.Routes()

	pctx := &plugin.Context{
		Huma:        h.Huma(),
		DB:          db,
		Guard:       authMgr.Require,
		Audit:       h.Audit,
		Enqueue:     h.queue.Enqueue,
		DeleteCache: authMgr.Cache().Delete,
		Provide:     reg.Provide,
		Lookup:      reg.Lookup,
		Host: &plugin.TestHost{
			SessionMock: &plugin.TestSession{
				UserIDFn:      authMgr.UserID,
				OrgIDFn:       authMgr.OrgID,
				RequirePermFn: h.RequirePerm,
			},
			CryptoMock: apiTestCrypto(cipher),
			SettingsMock: &plugin.TestSettings{
				GetWorkspaceSettingFn: h.GetWorkspaceSetting,
				GetGlobalSettingFn:    h.GetGlobalSetting,
			},
		},
	}
	for _, p := range plugins {
		p.Mount(nil, pctx)
	}
	if vs, ok := plugin.LookupServiceAs[plugin.VaultService](reg.Lookup, plugin.ServiceVault); ok {
		eventbus.SetVaultService(vs)
		notification.DefaultRouter().SetVaultService(vs)
		h.SetVaultService(vs)
		t.Cleanup(func() {
			eventbus.SetVaultService(nil)
			notification.DefaultRouter().SetVaultService(nil)
		})
	}

	return h, srv, db
}

type apiSubjectVault struct{ sv *crypto.SubjectVault }

func (v apiSubjectVault) Encrypt(p []byte) (string, error) { return v.sv.Encrypt(p) }
func (v apiSubjectVault) Decrypt(s string) ([]byte, error) { return v.sv.Decrypt(s) }
func (v apiSubjectVault) EncryptWithAAD(p, aad []byte) (string, error) {
	return v.sv.EncryptWithAAD(p, aad)
}
func (v apiSubjectVault) DecryptWithAAD(s string, aad []byte) ([]byte, error) {
	return v.sv.DecryptWithAAD(s, aad)
}
func (v apiSubjectVault) SubjectVault(ownerID uint, subject string) (plugin.CryptoVault, error) {
	sv, err := v.sv.SubjectVault(ownerID, subject)
	if err != nil {
		return nil, err
	}
	return apiSubjectVault{sv: sv}, nil
}
func (v apiSubjectVault) ShredSubjectKey(ownerID uint, subject string) error {
	return v.sv.ShredSubjectKey(ownerID, subject)
}
func (v apiSubjectVault) HasSubjectKey(ownerID uint, subject string) bool {
	return v.sv.HasSubjectKey(ownerID, subject)
}

func apiTestCrypto(cipher *crypto.Cipher) *plugin.TestCrypto {
	return &plugin.TestCrypto{
		EncryptFn:        cipher.Encrypt,
		DecryptFn:        cipher.Decrypt,
		EncryptWithAADFn: cipher.EncryptWithAAD,
		DecryptWithAADFn: cipher.DecryptWithAAD,
		SubjectVaultFn: func(ownerID uint, subject string) (plugin.CryptoVault, error) {
			sv, err := cipher.SubjectVault(ownerID, subject)
			if err != nil {
				return nil, err
			}
			return apiSubjectVault{sv: sv}, nil
		},
		ShredSubjectKeyFn: cipher.ShredSubjectKey,
		HasSubjectKeyFn:   cipher.HasSubjectKey,
	}
}

// newTestHandlerWithoutMail builds a test handler without mail plugin.
func newTestHandlerWithoutMail(t *testing.T) (*Handler, http.Handler, *gorm.DB) {
	t.Helper()
	return newTestHandlerRawCfg(t, &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"})
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

func (s apiEnvStore) Delete(key string) error {
	return s.db.Delete(&models.Setting{Key: key}).Error
}

func (s apiEnvStore) HasEncryptedValues() (bool, error) {
	return models.HasEncryptedValues(s.db)
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
	req = httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer-authed /api/overview: got %d want 200", rec.Code)
	}

	// A bad bearer token is rejected.
	req = httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	req.Header.Set("Authorization", "Bearer oct_totallybogus0000000000000000000000")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad bearer token: got %d want 401", rec.Code)
	}
}
