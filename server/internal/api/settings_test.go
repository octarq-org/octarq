package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestSplitList(t *testing.T) {
	cases := map[string][]string{
		"":                         nil,
		"a,b,c":                    {"a", "b", "c"},
		"a\nb\nc":                  {"a", "b", "c"},
		" A , a ,B ":               {"a", "b"},
		"go\tlogin pricing":        {"go", "login", "pricing"},
		"x,,,y":                    {"x", "y"},
		"Admin\nadmin\nPOSTMASTER": {"admin", "postmaster"},
	}
	for in, want := range cases {
		got := splitList(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("splitList(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRequireEmailVerificationDefaultOn(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	if !h.requireEmailVerification() {
		t.Fatal("absent setting: requireEmailVerification() = false, want true (default on)")
	}
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "false"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if h.requireEmailVerification() {
		t.Fatal("explicit \"false\": requireEmailVerification() = true, want false")
	}
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if !h.requireEmailVerification() {
		t.Fatal("explicit \"true\": requireEmailVerification() = false, want true")
	}
}

func TestCORSOriginsSources(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	if got := h.CORSOrigins(); len(got) != 0 {
		t.Fatalf("nothing configured: CORSOrigins() = %v, want empty", got)
	}

	h.cfg.PublicCORSOrigins = "https://octarq.org, https://app.octarq.org"
	if got := h.CORSOrigins(); !reflect.DeepEqual(got, []string{"https://octarq.org", "https://app.octarq.org"}) {
		t.Fatalf("env fallback: CORSOrigins() = %v", got)
	}

	if err := db.Save(&models.Setting{Key: keyPublicCORSOrigins, Value: "https://example.com"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if got := h.CORSOrigins(); !reflect.DeepEqual(got, []string{"https://example.com"}) {
		t.Fatalf("settings table must win over env: CORSOrigins() = %v", got)
	}
}

func TestInstanceSettingsCORSRoundTrip(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	do := func(method, body string) string {
		req := httptest.NewRequest(method, "/api/instance-settings", strings.NewReader(body))
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s /api/instance-settings: got %d (%s)", method, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	do("PUT", `{"publicCorsOrigins":"https://octarq.org, https://app.octarq.org"}`)
	got := do("GET", "")
	if !strings.Contains(got, "https://octarq.org") || !strings.Contains(got, "https://app.octarq.org") {
		t.Fatalf("instance settings did not round-trip the whitelist: %s", got)
	}

	var raw models.Setting
	if err := db.First(&raw, "key = ?", keyPublicCORSOrigins).Error; err != nil {
		t.Fatalf("setting not persisted: %v", err)
	}
	if raw.Value != "https://octarq.org\nhttps://app.octarq.org" {
		t.Fatalf("persisted value = %q, want the normalized list form", raw.Value)
	}
}
