package api

// Plugin toggles across a *workspace switch*.
//
// TestPluginToggleIsolatedPerOrg already covers the boundary, but it fabricates
// one pre-baked session per org, each belonging to a different user. That is not
// how the dashboard works: one user belongs to several orgs and moves between
// them with POST /api/auth/switch-org, which re-issues the session cookie with a
// new active org. These tests drive that real path, because a leak that only
// shows up after a switch would be invisible to the pre-baked-session test.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

// switchTo performs the real workspace switch and returns the re-issued cookies.
func switchTo(t *testing.T, srv http.Handler, cookies []*http.Cookie, orgID uint) []*http.Cookie {
	t.Helper()
	rec := do(srv, "POST", "/api/auth/switch-org", cookies, `{"orgId":`+itoa(orgID)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to org %d: got %d (%s)", orgID, rec.Code, rec.Body.String())
	}
	fresh := rec.Result().Cookies()
	if len(fresh) == 0 {
		t.Fatalf("switch to org %d returned no cookie: the session still points at the old org", orgID)
	}
	return fresh
}

// featureEnabled reads one feature's enabled flag as the dashboard's plugin
// manager sees it.
func featureEnabled(t *testing.T, srv http.Handler, cookies []*http.Cookie, key string) bool {
	t.Helper()
	return feature(t, srv, cookies, key).Enabled
}

// TestPluginToggleDoesNotLeakAcrossWorkspaceSwitch is the reported bug: toggle a
// feature in workspace A, switch to workspace B, and B shows A's state.
func TestPluginToggleDoesNotLeakAcrossWorkspaceSwitch(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())

	const orgA, orgB = uint(911), uint(912)

	// One user, member of both workspaces — the shape the switcher requires.
	u := models.User{Email: t.Name() + "+multi@x.com"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, org := range []uint{orgA, orgB} {
		if err := db.Create(&models.OrgMember{OrgID: org, UserID: u.ID, Role: "owner"}).Error; err != nil {
			t.Fatalf("membership in %d: %v", org, err)
		}
	}

	sess := sessionCookies(t, u.ID, orgA)

	if rec := do(srv, "PUT", "/api/plugins/dns", sess, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable dns in A: got %d (%s)", rec.Code, rec.Body.String())
	}
	if !featureEnabled(t, srv, sess, "dns") {
		t.Fatalf("dns should be on in workspace A right after enabling it")
	}

	// Switch to B on the same session.
	sess = switchTo(t, srv, sess, orgB)

	if featureEnabled(t, srv, sess, "dns") {
		t.Errorf("dns reads as enabled in workspace B after being enabled only in A")
	}

	var rows []models.PluginSetting
	db.Where("org_id = ?", orgB).Find(&rows)
	for _, r := range rows {
		if r.Enabled {
			t.Errorf("workspace B has an enabled row for %q that it never set", r.Plugin)
		}
	}

	// Toggling in B must not reach back into A.
	if rec := do(srv, "PUT", "/api/plugins/links", sess, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable links in B: got %d", rec.Code)
	}
	sess = switchTo(t, srv, sess, orgA)
	if featureEnabled(t, srv, sess, "links") {
		t.Errorf("links reads as enabled in workspace A after being enabled only in B")
	}
	if !featureEnabled(t, srv, sess, "dns") {
		t.Errorf("workspace A lost its own dns toggle after a round trip through B")
	}
}

// TestMenusFollowWorkspaceSwitch: the sidebar is backend-driven, so /api/menus
// must answer for the *switched-to* workspace. If it kept answering for the old
// org, a user would see A's menus while looking at B's data — which is how a
// leak like this gets noticed in the first place.
func TestMenusFollowWorkspaceSwitch(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())

	const orgA, orgB = uint(913), uint(914)
	u := models.User{Email: t.Name() + "+multi@x.com"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, org := range []uint{orgA, orgB} {
		if err := db.Create(&models.OrgMember{OrgID: org, UserID: u.ID, Role: "owner"}).Error; err != nil {
			t.Fatalf("membership in %d: %v", org, err)
		}
	}

	sess := sessionCookies(t, u.ID, orgA)
	if rec := do(srv, "PUT", "/api/plugins/dns", sess, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable dns in A: got %d", rec.Code)
	}

	hasDNS := func(cookies []*http.Cookie) bool {
		rec := do(srv, "GET", "/api/menus", cookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("menus: got %d (%s)", rec.Code, rec.Body.String())
		}
		var menus []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil {
			t.Fatalf("decode menus: %v (%s)", err, rec.Body.String())
		}
		for _, m := range menus {
			if m.ID == "dns" || m.ID == "domains" {
				return true
			}
		}
		return false
	}

	if !hasDNS(sess) {
		t.Fatalf("workspace A should have the DNS menu after enabling it")
	}
	sess = switchTo(t, srv, sess, orgB)
	if hasDNS(sess) {
		t.Errorf("workspace B serves the DNS menu, but DNS was only enabled in A")
	}
}
