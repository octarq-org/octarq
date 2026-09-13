package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestRequirePermNilInputs(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)

	// r == nil -> false
	var h *Handler
	if h.RequirePerm(nil, "dns.records.delete", "admin") {
		t.Fatal("expected RequirePerm(nil, ...) to return false, got true")
	}

	// h == nil and no resolver -> false (fail-closed)
	plugin.ResetPermRegistry()
	defer plugin.ResetPermRegistry()
	if h.RequirePerm(req, "dns.records.delete", "admin") {
		t.Fatal("expected RequirePerm on nil Handler to return false, got true")
	}
}

func TestRequirePermFallbackToRole(t *testing.T) {
	t.Cleanup(plugin.ResetPermRegistry)
	plugin.ResetPermRegistry()

	h, srv, db := newTestHandlerRaw(t)
	_ = srv

	seedMember(t, db, 100, "admin")
	seedMember(t, db, 101, "member")

	adminCookies := sessionCookies(t, 100, 1)
	memberCookies := sessionCookies(t, 101, 1)

	reqAdmin := httptest.NewRequest("GET", "/api/test", nil)
	for _, c := range adminCookies {
		reqAdmin.AddCookie(c)
	}

	reqMember := httptest.NewRequest("GET", "/api/test", nil)
	for _, c := range memberCookies {
		reqMember.AddCookie(c)
	}

	if !h.RequirePerm(reqAdmin, "dns.records.delete", "admin") {
		t.Fatal("expected admin caller to pass RequirePerm fallback for admin role")
	}

	if h.RequirePerm(reqMember, "dns.records.delete", "admin") {
		t.Fatal("expected member caller to fail RequirePerm fallback for admin role")
	}
}

func TestRequirePermResolverPriority(t *testing.T) {
	t.Cleanup(plugin.ResetPermRegistry)
	plugin.ResetPermRegistry()

	h, srv, db := newTestHandlerRaw(t)
	_ = srv

	seedMember(t, db, 102, "owner")

	ownerCookies := sessionCookies(t, 102, 1)
	reqOwner := httptest.NewRequest("GET", "/api/test", nil)
	for _, c := range ownerCookies {
		reqOwner.AddCookie(c)
	}

	// 1. Resolver is decided=true, allow=false -> must refuse even an owner.
	plugin.SetPermResolver(func(r *http.Request, permKey string) (allow, decided bool) {
		if permKey == "dns.records.delete" {
			return false, true
		}
		return false, false
	})

	if h.RequirePerm(reqOwner, "dns.records.delete", "admin") {
		t.Fatal("expected decided=true, allow=false resolver to refuse owner, but got allowed")
	}

	// 2. Resolver is decided=false -> fallback to role check (owner >= admin -> allowed).
	plugin.SetPermResolver(func(r *http.Request, permKey string) (allow, decided bool) {
		return false, false
	})

	if !h.RequirePerm(reqOwner, "dns.records.delete", "admin") {
		t.Fatal("expected decided=false resolver to fallback to role check for owner, but got refused")
	}
}
