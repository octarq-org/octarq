package api

// Self-serve account deletion (GDPR). A user with no remaining org memberships
// can erase their own record; a user still in any org is refused; and the route
// takes no user-ID parameter, so there is no way to address someone else's
// account.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestDeleteAccount_NoMembershipsCascades(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(401)
	db.Create(&models.Org{ID: org, Name: "Self Delete Org", Slug: "self-delete-org"})
	uid := seedOrgMember(t, db, org, "self@x.com", "owner")
	cookies := sessionCookies(t, uid, org)

	// The session minted a membership row; drop it so the user holds none.
	db.Where("user_id = ?", uid).Delete(&models.OrgMember{})

	// Seed the user-scoped rows the cascade must remove.
	db.Create(&models.UserIdentity{UserID: uid, Provider: "oidc", Issuer: "https://idp.example", Subject: "sub-self-delete"})
	db.Create(&models.UserSetting{UserID: uid, Key: "onboarding_dismissed", Value: "true"})
	db.Create(&models.Token{OrgID: org, UserID: uid, Name: "t", Hash: models.HashToken("oct_acct_self_delete_token_0000000"), Prefix: "oct_acct"})
	db.Create(&models.Session{UserID: uid, OrgID: org, Token: models.HashToken("second-session-token"), IP: "127.0.0.1", UserAgent: "ua"})

	// Wrong confirmation is rejected and deletes nothing.
	if rec := do(srv, "DELETE", "/api/account/user", cookies, `{"confirm":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("delete w/o confirm: got %d, want 400", rec.Code)
	}
	var userCount int64
	db.Model(&models.User{}).Where("id = ?", uid).Count(&userCount)
	if userCount != 1 {
		t.Fatalf("rejected delete removed the user: count=%d", userCount)
	}

	if rec := do(srv, "DELETE", "/api/account/user", cookies, `{"confirm":"DELETE MY ACCOUNT"}`); rec.Code != http.StatusOK {
		t.Fatalf("delete: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var n int64
	db.Model(&models.User{}).Where("id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("user row remains after self-delete: %d", n)
	}
	db.Model(&models.OrgMember{}).Where("user_id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("membership remains after self-delete: %d", n)
	}
	db.Model(&models.UserIdentity{}).Where("user_id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("identity remains after self-delete: %d", n)
	}
	db.Model(&models.UserSetting{}).Where("user_id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("user setting remains after self-delete: %d", n)
	}
	db.Model(&models.Session{}).Where("user_id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("session remains after self-delete: %d", n)
	}
	db.Model(&models.Token{}).Where("user_id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("token remains after self-delete: %d", n)
	}

	// The org itself survives — self-deletion erases the person, not the tenant.
	db.Model(&models.Org{}).Where("id = ?", org).Count(&n)
	if n != 1 {
		t.Errorf("org count = %d, want 1 — self-delete must not purge the org", n)
	}

	// The revoked cookie no longer authenticates.
	if rec := do(srv, "GET", "/api/overview", cookies, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("post-delete overview: got %d, want 401", rec.Code)
	}
}

func TestDeleteAccount_RefusedWhileMemberOfAnyOrg(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(402)
	uid := seedOrgMember(t, db, org, "member@x.com", "member")
	cookies := sessionCookies(t, uid, org)
	db.Create(&models.Token{OrgID: org, UserID: uid, Name: "t", Hash: models.HashToken("oct_acct_still_member_token_0000"), Prefix: "oct_acct"})

	if rec := do(srv, "DELETE", "/api/account/user", cookies, `{"confirm":"DELETE MY ACCOUNT"}`); rec.Code != http.StatusConflict {
		t.Fatalf("delete while still a member: got %d, want 409 (%s)", rec.Code, rec.Body.String())
	}

	// Nothing was deleted: the user, their membership, and their token survive.
	var n int64
	db.Model(&models.User{}).Where("id = ?", uid).Count(&n)
	if n != 1 {
		t.Errorf("user row changed while refused: %d", n)
	}
	db.Model(&models.OrgMember{}).Where("user_id = ?", uid).Count(&n)
	if n != 1 {
		t.Errorf("membership changed while refused: %d", n)
	}
	db.Model(&models.Token{}).Where("user_id = ?", uid).Count(&n)
	if n != 1 {
		t.Errorf("token changed while refused: %d", n)
	}

	// Unauthenticated delete is rejected before any guard is evaluated.
	if rec := do(srv, "DELETE", "/api/account/user", nil, `{"confirm":"DELETE MY ACCOUNT"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated delete: got %d, want 401", rec.Code)
	}
}

func TestDeleteAccount_NoUserIdParameter(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(403)
	ua := seedOrgMember(t, db, org, "a@x.com", "owner")
	ub := seedOrgMember(t, db, org, "b@x.com", "owner")
	cookiesA := sessionCookies(t, ua, org)
	sessionCookies(t, ub, org)
	db.Where("user_id IN ?", []uint{ua, ub}).Delete(&models.OrgMember{})

	// There is no {userId} path parameter on this route, so addressing another
	// account by id 404s before it can delete anything.
	if rec := do(srv, "DELETE", "/api/account/user/"+strconv.FormatUint(uint64(ub), 10), cookiesA, `{"confirm":"DELETE MY ACCOUNT"}`); rec.Code != http.StatusNotFound {
		t.Errorf("delete by other user id: got %d, want 404", rec.Code)
	}
	var n int64
	db.Model(&models.User{}).Where("id = ?", ub).Count(&n)
	if n != 1 {
		t.Errorf("other user was affected by a bogus route: %d", n)
	}

	// Deleting one's own account leaves the other user untouched.
	if rec := do(srv, "DELETE", "/api/account/user", cookiesA, `{"confirm":"DELETE MY ACCOUNT"}`); rec.Code != http.StatusOK {
		t.Fatalf("self delete: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	db.Model(&models.User{}).Where("id = ?", ua).Count(&n)
	if n != 0 {
		t.Errorf("self user row remains: %d", n)
	}
	db.Model(&models.User{}).Where("id = ?", ub).Count(&n)
	if n != 1 {
		t.Errorf("neighbor user row lost: %d", n)
	}
	db.Model(&models.Session{}).Where("user_id = ?", ub).Count(&n)
	if n != 1 {
		t.Errorf("neighbor session lost: %d", n)
	}
}
