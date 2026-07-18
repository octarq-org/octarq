package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	dns "github.com/octarq-org/octarq/plugins/dns"
	links "github.com/octarq-org/octarq/plugins/links"
	mail "github.com/octarq-org/octarq/plugins/mail"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"github.com/octarq-org/octarq/llmprovider"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type fakeLLM struct{ reply string }

func (f fakeLLM) Name() string         { return "fake" }
func (f fakeLLM) DefaultModel() string { return "fake-big" }
func (f fakeLLM) CheapModel() string   { return "fake-small" }
func (f fakeLLM) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{Text: f.reply, Model: req.Model}, nil
}

// newAITestHandler mirrors newTestHandler but keeps the *Handler so the test
// can inject a fake LLM resolver before routes are served. An empty reply
// leaves the default env-backed resolver in place.
func newAITestHandler(t *testing.T, reply string) (http.Handler, *gorm.DB) {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mail.Mailbox{}, &mail.Email{}, &mail.SMTPSender{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	if reply != "" {
		h.SetLLMResolver(func() (llmprovider.Provider, error) { return fakeLLM{reply: reply}, nil })
	}

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
	}
	dnsP.Mount(nil, pctx)
	mailP.Mount(nil, pctx)
	linksP.Mount(nil, pctx)

	return srv, db
}

func TestAIStatusUnconfigured(t *testing.T) {
	srv, _ := newAITestHandler(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/ai/assist/status", nil)
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d (%s)", rec.Code, rec.Body.String())
	}
	var st struct {
		Configured bool   `json:"configured"`
		Provider   string `json:"provider"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No OCTARQ_LLM_* env in tests → unconfigured, and the LLM endpoints refuse
	// with a hint instead of calling out.
	if st.Configured {
		t.Skip("OCTARQ_LLM_*/ANTHROPIC_API_KEY set in this environment")
	}
	req = httptest.NewRequest(http.MethodPost, "/api/ai/assist/suggest-slug", strings.NewReader(`{"target":"https://example.com"}`))
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unconfigured suggest-slug: got %d want 400 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "OCTARQ_LLM_API_KEY") {
		t.Errorf("error should hint at OCTARQ_LLM_API_KEY: %s", rec.Body.String())
	}
}

func TestAISummarizeEmailOrgIsolation(t *testing.T) {
	srv, db := newAITestHandler(t, "A billing notice; pay by Friday.")

	db.Create(&mail.Mailbox{Address: "a@one.test", OrgID: 1})
	db.Create(&mail.Mailbox{Address: "b@two.test", OrgID: 2})
	var mb1, mb2 mail.Mailbox
	db.First(&mb1, "address = ?", "a@one.test")
	db.First(&mb2, "address = ?", "b@two.test")
	db.Create(&mail.Email{MailboxID: mb1.ID, Subject: "Invoice", Text: "Pay us", ReceivedAt: time.Now()})
	db.Create(&mail.Email{MailboxID: mb2.ID, Subject: "Other org", Text: "Secret", ReceivedAt: time.Now()})
	var e1, e2 mail.Email
	db.First(&e1, "subject = ?", "Invoice")
	db.First(&e2, "subject = ?", "Other org")

	post := func(id uint) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/ai/assist/summarize-email/"+itoa(id), nil)
		for _, c := range sessionCookies(t, 1, 1) {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	rec := post(e1.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("summarize own email: got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "billing notice") {
		t.Errorf("summary missing: %s", rec.Body.String())
	}

	// Another org's email must be a 404, not a summary.
	if rec = post(e2.ID); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-org summarize: got %d want 404", rec.Code)
	}
}

func itoa(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestParseSlugList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`["go-fast","octarq-live","ship-it"]`, []string{"go-fast", "octarq-live", "ship-it"}},
		{"```json\n[\"promo-2026\"]\n```", []string{"promo-2026"}},
		{"Here you go:\n- launch-day\n- big-sale\n", []string{"launch-day", "big-sale"}},
		{`["Has Space", "UPPER", "ok-slug"]`, []string{"upper", "ok-slug"}}, // lowercased; invalid dropped
		{"no valid tokens here!!", nil},
	}
	for _, c := range cases {
		got := parseSlugList(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseSlugList(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
