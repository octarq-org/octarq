package api

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func registerRemovedHook(t *testing.T, name string, hook plugin.MemberRemovedHook) {
	t.Helper()
	plugin.RegisterMemberRemovedHook(name, hook)
	t.Cleanup(plugin.ResetMemberRemovedHooks)
}

func TestMemberRemovalNotifiesHookWithCorrectPair(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5101)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	ownerSession := sessionCookies(t, ownerUID, org)

	var gotOrg, gotUser uint
	calls := 0
	registerRemovedHook(t, t.Name(), func(orgID, userID uint) {
		calls++
		gotOrg, gotUser = orgID, userID
	})

	rec := do(srv, "DELETE", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	if calls != 1 {
		t.Fatalf("hook called %d times, want once", calls)
	}
	if gotOrg != org || gotUser != memberUID {
		t.Fatalf("hook got (%d, %d), want (%d, %d)", gotOrg, gotUser, org, memberUID)
	}
}

func TestMemberRemovalFailureSkipsHooks(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5201)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	ownerSession := sessionCookies(t, ownerUID, org)

	calls := 0
	registerRemovedHook(t, t.Name(), func(orgID, userID uint) { calls++ })

	rec := do(srv, "DELETE", "/api/org/members/99999", ownerSession, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("remove non-member: got %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
	if calls != 0 {
		t.Fatalf("hook called %d times on failed removal, want 0", calls)
	}

	var count int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, uint(99999)).Count(&count)
	if count != 0 {
		t.Fatal("stray membership row for non-member appeared")
	}
}

func TestMemberRemovalHookRegistrationIsKeyed(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5301)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	ownerSession := sessionCookies(t, ownerUID, org)

	stale := 0
	live := 0
	const name = "member-removal-hook-keyed-test"
	registerRemovedHook(t, name, func(orgID, userID uint) { stale++ })
	registerRemovedHook(t, name, func(orgID, userID uint) { live++ })

	rec := do(srv, "DELETE", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	if stale != 0 {
		t.Errorf("stale hook ran %d times — registration appended instead of replaced", stale)
	}
	if live != 1 {
		t.Errorf("live hook ran %d times, want exactly once", live)
	}
}

func TestMemberRemovalHookPanicDoesNotFailRequest(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5401)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	ownerSession := sessionCookies(t, ownerUID, org)

	calls := 0
	registerRemovedHook(t, t.Name()+"-boom", func(orgID, userID uint) { panic("boom") })
	registerRemovedHook(t, t.Name()+"-count", func(orgID, userID uint) { calls++ })

	rec := do(srv, "DELETE", "/api/org/members/"+strconv.FormatUint(uint64(memberUID), 10), ownerSession, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member with panicking hook: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var count int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, memberUID).Count(&count)
	if count != 0 {
		t.Fatal("membership row survived removal with a panicking hook")
	}
	if calls != 1 {
		t.Errorf("sibling hook ran %d times, want once — a panic must not abort the loop", calls)
	}
}
