package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// orgSlugFixture sets up two workspaces so "taken" and "another org's slug" are
// distinguishable, and returns a request helper.
func orgSlugFixture(t *testing.T) (*gorm.DB, func(method, path string, cookies []*http.Cookie, body string) *httptest.ResponseRecorder, []*http.Cookie, []*http.Cookie, []*http.Cookie) {
	t.Helper()
	_, srv, db := newTestHandlerRaw(t)

	db.Create(&models.Org{ID: 1, Name: "Acme", Slug: "owner-example-com"})
	db.Create(&models.Org{ID: 2, Name: "Other", Slug: "othertaken"})
	db.Create(&models.User{ID: 1, Email: "owner@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 1, Role: "owner"})
	db.Create(&models.User{ID: 2, Email: "admin@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 2, Role: "admin"})
	db.Create(&models.User{ID: 3, Email: "member@example.com"})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 3, Role: "member"})

	do := func(method, path string, cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	return db, do, sessionCookies(t, 1, 1), sessionCookies(t, 2, 1), sessionCookies(t, 3, 1)
}

// Changing the slug invalidates addresses held by Stripe and by the org's IdP,
// so it is the owner's call — not something the workspace-rename role can do.
func TestUpdateOrgSlugIsOwnerOnly(t *testing.T) {
	db, do, owner, admin, member := orgSlugFixture(t)

	if code := do("PUT", "/api/org/slug", member, `{"slug":"acme"}`).Code; code != 403 {
		t.Errorf("member PUT /api/org/slug = %d, want 403", code)
	}
	if code := do("PUT", "/api/org/slug", admin, `{"slug":"acme"}`).Code; code != 403 {
		t.Errorf("admin PUT /api/org/slug = %d, want 403", code)
	}
	if code := do("PUT", "/api/org/slug", nil, `{"slug":"acme"}`).Code; code != 401 {
		t.Errorf("anonymous PUT /api/org/slug = %d, want 401", code)
	}

	var org models.Org
	db.First(&org, 1)
	if org.Slug != "owner-example-com" {
		t.Fatalf("a refused request changed the slug to %q", org.Slug)
	}

	if code := do("PUT", "/api/org/slug", owner, `{"slug":"acme"}`).Code; code != 200 {
		t.Fatalf("owner PUT /api/org/slug = %d, want 200", code)
	}
	db.First(&org, 1)
	if org.Slug != "acme" {
		t.Fatalf("slug = %q after owner rename, want acme", org.Slug)
	}
}

func TestUpdateOrgSlugValidation(t *testing.T) {
	db, do, owner, _, _ := orgSlugFixture(t)

	cases := []struct {
		slug string
		want int
		why  string
	}{
		{"Acme", 200, "uppercase is lowercased, not refused"},
		{"a", 400, "too short"},
		{"-acme", 400, "leading hyphen"},
		{"acme-", 400, "trailing hyphen"},
		{"ac me", 400, "space"},
		{"ac/me", 400, "path separator"},
		{"acme.com", 400, "dot"},
		{"admin", 409, "reserved for a top-level route"},
		{"billing", 409, "reserved"},
		{"othertaken", 409, "another workspace holds it"},
	}
	for _, c := range cases {
		body, _ := json.Marshal(map[string]string{"slug": c.slug})
		if code := do("PUT", "/api/org/slug", owner, string(body)).Code; code != c.want {
			t.Errorf("PUT slug=%q = %d, want %d (%s)", c.slug, code, c.want, c.why)
		}
	}

	// The other workspace is untouched by any of the above.
	var other models.Org
	db.First(&other, 2)
	if other.Slug != "othertaken" {
		t.Fatalf("other workspace slug = %q", other.Slug)
	}
}

func TestOrgSlugRead(t *testing.T) {
	_, do, owner, _, member := orgSlugFixture(t)

	rec := do("GET", "/api/org/slug", owner, "")
	if rec.Code != 200 {
		t.Fatalf("GET /api/org/slug = %d", rec.Code)
	}
	var view OrgSlugView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Slug != "owner-example-com" {
		t.Fatalf("view = %+v, want the workspace's own slug", view)
	}

	// Reading the workspace's address is still workspace administration.
	if code := do("GET", "/api/org/slug", member, "").Code; code != 403 {
		t.Errorf("member GET /api/org/slug = %d, want 403", code)
	}
}
