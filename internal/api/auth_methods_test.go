package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/auth"
)

func TestAuthMethodsPublicEndpoint(t *testing.T) {
	auth.ResetMethodsForTesting()
	defer auth.ResetMethodsForTesting()

	h, _ := newHandlerForAdminTest(t)
	mux := h.Routes()

	// 1. Unauthenticated request when registry is empty -> 200 OK and []
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/methods want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}

	var res1 []auth.AuthMethod
	if err := json.Unmarshal(rec.Body.Bytes(), &res1); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}
	if len(res1) != 0 {
		t.Fatalf("expected empty list, got %#v", res1)
	}

	// 2. Register a method, test public endpoint again
	auth.Register(auth.AuthMethod{
		ID:       "sso",
		Label:    "Sign in with SSO",
		LoginURL: "/api/sso/login",
		IconKey:  "shield",
	})

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/methods want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}

	var res2 []auth.AuthMethod
	if err := json.Unmarshal(rec.Body.Bytes(), &res2); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}
	if len(res2) != 1 || res2[0].ID != "sso" || res2[0].Label != "Sign in with SSO" {
		t.Fatalf("unexpected auth methods response: %#v", res2)
	}
}
