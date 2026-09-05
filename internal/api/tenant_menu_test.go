package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestTenantMenu_SwitchOrg(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Admin user starts with org 1

	org2 := models.Org{Name: "Org 2", Slug: "org2"}
	db.Create(&org2)
	db.Create(&models.OrgMember{OrgID: org2.ID, UserID: 1, Role: "member"})

	t.Run("success", func(t *testing.T) {
		rec := do(srv, "POST", "/api/auth/switch-org", cookies, fmt.Sprintf(`{"orgId":%d}`, org2.ID))
		if rec.Code != http.StatusOK {
			t.Fatalf("switch-org failed: got %d (%s)", rec.Code, rec.Body.String())
		}
		var out SwitchOrgOutputBody
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if !out.OK {
			t.Errorf("expected OK=true")
		}
	})

	t.Run("not member", func(t *testing.T) {
		org3 := models.Org{Name: "Org 3", Slug: "org3"}
		db.Create(&org3)
		rec := do(srv, "POST", "/api/auth/switch-org", cookies, fmt.Sprintf(`{"orgId":%d}`, org3.ID))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})
}

func TestTenantMenu_ListOrgs(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/orgs", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list-orgs failed: got %d", rec.Code)
	}

	var out []OrgItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(out) < 1 {
		t.Errorf("expected at least 1 org, got %d", len(out))
	}
}

func TestTenantMenu_CreateOrg(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	t.Run("success", func(t *testing.T) {
		rec := do(srv, "POST", "/api/orgs", cookies, `{"name":"New Org"}`)
		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated && rec.Code != 200 {
			t.Fatalf("create-org failed: got %d (%s)", rec.Code, rec.Body.String())
		}

		var out models.Org
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if out.Name != "New Org" {
			t.Errorf("expected name 'New Org', got %q", out.Name)
		}
		if out.Slug == "" {
			t.Errorf("expected non-empty slug")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		rec := do(srv, "POST", "/api/orgs", cookies, `{"name":"   "}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty name, got %d", rec.Code)
		}
	})
}

func TestTenantMenu_UpdateOrg(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	t.Run("success", func(t *testing.T) {
		rec := do(srv, "PUT", "/api/org", cookies, `{"name":"New Name"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update-org failed: got %d (%s)", rec.Code, rec.Body.String())
		}

		var out models.Org
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if out.Name != "New Name" {
			t.Errorf("expected name 'New Name', got %q", out.Name)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		rec := do(srv, "PUT", "/api/org", cookies, `{"name":"   "}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty name, got %d", rec.Code)
		}
	})
}

func TestTenantMenu_GetUpdateUserSettings(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// Set a setting
	rec := do(srv, "PUT", "/api/user/settings", cookies, `{"key":"theme","value":"dark"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update setting failed: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Get settings
	rec = do(srv, "GET", "/api/user/settings", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings failed: got %d", rec.Code)
	}

	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out["theme"] != "dark" {
		t.Errorf("expected theme='dark', got %q", out["theme"])
	}

	t.Run("empty key", func(t *testing.T) {
		rec := do(srv, "PUT", "/api/user/settings", cookies, `{"key":"","value":"dark"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty key, got %d", rec.Code)
		}
	})
}

func TestTenantMenu_ListMenus(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/menus", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list-menus failed: got %d (%s)", rec.Code, rec.Body.String())
	}
	var out []MenuItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(out) == 0 {
		t.Errorf("expected menus to not be empty")
	}
}

func TestTenantMenu_ListActions(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/actions", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list-actions failed: got %d (%s)", rec.Code, rec.Body.String())
	}
	var out []Action
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
}

// Add dummy coverage to reach some 401s and nil contexts
func TestTenantMenu_NilCtxPaths(t *testing.T) {
	srv, _ := newTestHandler(t)
	// Testing Unauth responses
	unauthPaths := []struct{ method, path, body string }{
		{"POST", "/api/auth/switch-org", `{"orgId":1}`},
		{"GET", "/api/orgs", ""},
		{"POST", "/api/orgs", `{"name":"test"}`},
		{"PUT", "/api/org", `{"name":"test"}`},
		{"GET", "/api/menus", ""},
		{"GET", "/api/actions", ""},
		{"GET", "/api/user/settings", ""},
		{"PUT", "/api/user/settings", `{"key":"k","value":"v"}`},
	}

	for _, tc := range unauthPaths {
		rec := do(srv, tc.method, tc.path, nil, tc.body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s unauth: got %d", tc.method, tc.path, rec.Code)
		}
	}
}
