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

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
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
	db := newTestDB(t)
	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	if reply != "" {
		h.SetLLMResolverForOrg(func(_ uint) (llmprovider.Provider, error) { return fakeLLM{reply: reply}, nil })
	}

	reg := plugin.NewRegistry()
	reg.Provide(plugin.ServiceMailEmailGet, plugin.EmailGetter(func(orgID, id uint) (string, string, string, bool) {
		var em testEmail
		if err := db.Where("id = ?", id).First(&em).Error; err != nil {
			return "", "", "", false
		}
		var mb testMailbox
		if err := db.Where("id = ?", em.MailboxID).First(&mb).Error; err != nil || mb.OrgID != orgID {
			return "", "", "", false
		}
		return em.FromAddr, em.Subject, em.Text, true
	}))
	h.SetServiceLookup(reg.Lookup)

	srv := h.Routes()
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
	if !strings.Contains(rec.Body.String(), "LLM Gateway") {
		t.Errorf("error should point at the LLM Gateway: %s", rec.Body.String())
	}
}

func TestAISummarizeEmailOrgIsolation(t *testing.T) {
	srv, db := newAITestHandler(t, "A billing notice; pay by Friday.")

	db.Create(&testMailbox{Address: "a@one.test", OrgID: 1})
	db.Create(&testMailbox{Address: "b@two.test", OrgID: 2})
	var mb1, mb2 testMailbox
	db.First(&mb1, "address = ?", "a@one.test")
	db.First(&mb2, "address = ?", "b@two.test")
	db.Create(&testEmail{MailboxID: mb1.ID, Subject: "Invoice", Text: "Pay us", ReceivedAt: time.Now()})
	db.Create(&testEmail{MailboxID: mb2.ID, Subject: "Other org", Text: "Secret", ReceivedAt: time.Now()})
	var e1, e2 testEmail
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

type namedLLM struct{ name string }

func (n namedLLM) Name() string         { return n.name }
func (n namedLLM) DefaultModel() string { return "m" }
func (n namedLLM) CheapModel() string   { return "m" }
func (n namedLLM) Complete(_ context.Context, _ llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{}, nil
}

func TestSetLLMResolverForOrgPerTenant(t *testing.T) {
	h := &Handler{}
	h.SetLLMResolverForOrg(func(orgID uint) (llmprovider.Provider, error) {
		if orgID == 7 {
			return namedLLM{name: "provider-org-7"}, nil
		}
		if orgID == 42 {
			return namedLLM{name: "provider-org-42"}, nil
		}
		return nil, errLLMNotConfigured
	})

	p7, err7 := h.llmFor(7)
	if err7 != nil || p7 == nil || p7.Name() != "provider-org-7" {
		t.Fatalf("llmFor(7) got p=%v err=%v, want provider-org-7", p7, err7)
	}

	p42, err42 := h.llmFor(42)
	if err42 != nil || p42 == nil || p42.Name() != "provider-org-42" {
		t.Fatalf("llmFor(42) got p=%v err=%v, want provider-org-42", p42, err42)
	}

	if p7.Name() == p42.Name() {
		t.Fatalf("llmFor(7) and llmFor(42) returned the same backend: %s", p7.Name())
	}
}
