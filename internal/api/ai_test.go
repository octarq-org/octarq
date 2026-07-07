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

	"github.com/octarq-org/led/config"
	"github.com/octarq-org/led/internal/auth"
	"github.com/octarq-org/led/internal/crypto"
	"github.com/octarq-org/led/internal/geo"
	"github.com/octarq-org/led/internal/models"
	"github.com/octarq-org/led/internal/queue"
	"github.com/octarq-org/led/llmprovider"
	"github.com/glebarez/sqlite"
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
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
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
	return h.Routes(), db
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
	// No LED_LLM_* env in tests → unconfigured, and the LLM endpoints refuse
	// with a hint instead of calling out.
	if st.Configured {
		t.Skip("LED_LLM_*/ANTHROPIC_API_KEY set in this environment")
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
	if !strings.Contains(rec.Body.String(), "LED_LLM_API_KEY") {
		t.Errorf("error should hint at LED_LLM_API_KEY: %s", rec.Body.String())
	}
}

func TestAISummarizeEmailOrgIsolation(t *testing.T) {
	srv, db := newAITestHandler(t, "A billing notice; pay by Friday.")

	db.Create(&models.Mailbox{Address: "a@one.test", OrgID: 1})
	db.Create(&models.Mailbox{Address: "b@two.test", OrgID: 2})
	var mb1, mb2 models.Mailbox
	db.First(&mb1, "address = ?", "a@one.test")
	db.First(&mb2, "address = ?", "b@two.test")
	db.Create(&models.Email{MailboxID: mb1.ID, Subject: "Invoice", Text: "Pay us", ReceivedAt: time.Now()})
	db.Create(&models.Email{MailboxID: mb2.ID, Subject: "Other org", Text: "Secret", ReceivedAt: time.Now()})
	var e1, e2 models.Email
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
		{`["go-fast","led-live","ship-it"]`, []string{"go-fast", "led-live", "ship-it"}},
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
