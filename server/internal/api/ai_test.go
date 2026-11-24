package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/llmprovider"
)

func newAITestHandler(t *testing.T) http.Handler {
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
	return h.Routes()
}

func TestAIStatusUnconfigured(t *testing.T) {
	srv := newAITestHandler(t)
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
	if st.Configured {
		t.Skip("OCTARQ_LLM_*/ANTHROPIC_API_KEY set in this environment")
	}
}

func itoa(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}

type namedLLM struct{ name string }

func (n namedLLM) Name() string         { return n.name }
func (n namedLLM) DefaultModel() string { return "m" }
func (n namedLLM) CheapModel() string   { return "m" }
func (n namedLLM) Complete(_ context.Context, _ llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{}, nil
}

func TestAIStatusConfigured(t *testing.T) {
	h := &Handler{}
	h.SetLLMResolverForOrg(func(_ uint) (llmprovider.Provider, error) {
		return namedLLM{name: "provider-org-1"}, nil
	})
	out, err := h.aiStatus(context.Background(), &AIStatusInput{Ctx: nil})
	if err != nil {
		t.Fatalf("aiStatus: %v", err)
	}
	if !out.Body.Configured || out.Body.Provider != "provider-org-1" {
		t.Fatalf("aiStatus = %+v, want configured provider-org-1", out.Body)
	}
}

func TestAIStatusRejectsNilProvider(t *testing.T) {
	h := &Handler{}
	h.SetLLMResolverForOrg(func(_ uint) (llmprovider.Provider, error) {
		return nil, nil
	})
	out, err := h.aiStatus(context.Background(), &AIStatusInput{Ctx: nil})
	if err != nil {
		t.Fatalf("aiStatus: %v", err)
	}
	if out.Body.Configured || out.Body.Provider != "" {
		t.Fatalf("aiStatus = %+v, want not configured", out.Body)
	}
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
