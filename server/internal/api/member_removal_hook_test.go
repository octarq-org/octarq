package api

// Member-removal hook tests. Removing a member must notify registered plugins
// so they can clean their own per-member state (e.g. downstream
// member role rows) instead of letting it silently survive a re-invite.
// The notification is best-effort and must never affect the removal request:
// a hook failure or panic cannot turn a successful removal into an error.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// registerRemovedHook registers a member-removed hook keyed by the test name
// and resets the whole registry when the test finishes, so one test's hook
// never leaks into another.
func registerRemovedHook(t *testing.T, name string, hook plugin.MemberRemovedHook) {
	t.Helper()
	plugin.RegisterMemberRemovedHook(name, hook)
	t.Cleanup(plugin.ResetMemberRemovedHooks)
}

// Removing a member invokes the registered hooks with the exact (orgID,
// userID) that was removed — not the caller's org or some zero value.
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

// A failed removal must not notify: the member is still in the workspace, and
// a hook clearing their state would wipe it for someone who remains.
func TestMemberRemovalFailureSkipsHooks(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5201)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	ownerSession := sessionCookies(t, ownerUID, org)

	calls := 0
	registerRemovedHook(t, t.Name(), func(orgID, userID uint) { calls++ })

	// 99999 exists on no test DB, so the membership lookup fails and the
	// request must 404 — with the hooks left untouched.
	rec := do(srv, "DELETE", "/api/org/members/99999", ownerSession, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("remove non-member: got %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
	if calls != 0 {
		t.Fatalf("hook called %d times on failed removal, want 0", calls)
	}

	// And the "not a member" member actually still exists afterwards.
	var count int64
	db.Model(&models.OrgMember{}).Where("org_id = ? AND user_id = ?", org, uint(99999)).Count(&count)
	if count != 0 {
		t.Fatal("stray membership row for non-member appeared")
	}
}

// Registration is keyed by plugin name, not appended: Mount runs more than
// once per process (internal/mcp re-Mounts every plugin), and an append-style
// registry would run the cleanup N times. Re-registering the same name must
// replace, so the removed request triggers the hook exactly once.
func TestMemberRemovalHookRegistrationIsKeyed(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5301)

	ownerUID := seedOrgMember(t, db, org, "owner@x.com", "owner")
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	ownerSession := sessionCookies(t, ownerUID, org)

	stale := 0
	live := 0
	const name = "member-removal-hook-keyed-test"
	// The second registration replaces the first — the stale hook must never run.
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

// A panicking hook is recovered per-hook: the removal request still returns
// 200, the membership row is still gone, and sibling hooks still run.
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
