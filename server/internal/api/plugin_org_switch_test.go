package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

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

func featureEnabled(t *testing.T, srv http.Handler, cookies []*http.Cookie, key string) bool {
	t.Helper()
	return feature(t, srv, cookies, key).Enabled
}

func TestPluginToggleDoesNotLeakAcrossWorkspaceSwitch(t *testing.T) {
	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins(mailDependsOnDNS())

	const orgA, orgB = uint(911), uint(912)

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
