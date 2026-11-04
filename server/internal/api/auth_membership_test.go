package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// seedPasswordUser creates an account that carries a password and no workspace
// membership. This is the state an administrator leaves behind by removing a
// member: the account survives, the membership row does not.
func seedPasswordUser(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := models.User{Email: email, PasswordHash: string(hash), EmailVerified: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

// loginOrgID signs in over HTTP and reports the workspace the session landed in.
func loginOrgID(t *testing.T, srv http.Handler, email string) uint {
	t.Helper()
	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"`+email+`","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d (%s)", rec.Code, rec.Body.String())
	}
	me := do(srv, "GET", "/api/auth/me", rec.Result().Cookies(), "")
	if me.Code != http.StatusOK {
		t.Fatalf("me: got %d (%s)", me.Code, me.Body.String())
	}
	var body struct {
		OrgID uint `json:"orgId"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	return body.OrgID
}

func assertNoCookies(t *testing.T, rec interface{ Result() *http.Response }, what string) {
	t.Helper()
	if cookies := rec.Result().Cookies(); len(cookies) > 0 {
		t.Fatalf("%s set %d cookies; a refused sign-in must issue no session", what, len(cookies))
	}
}

// refusedCode reports the machine-readable code of a refused response. The
// client branches on it, so a refusal without the right code sends the user
// down the wrong recovery path.
func refusedCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

// TestLoginWithoutWorkspaceIsRefused covers the escalation a removed member
// used to get: the account still has its password, and sign-in minted a fresh
// owner membership so the removed person was back inside, now with full rights.
func TestLoginWithoutWorkspaceIsRefused(t *testing.T) {
	srv, db := newTestHandler(t)
	user := seedPasswordUser(t, db, "removed@example.com")

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"removed@example.com","password":"pw"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login of an account in no workspace: got %d (%s), want 403", rec.Code, rec.Body.String())
	}
	assertNoCookies(t, rec, "refused login")
	if code := refusedCode(t, rec); code != apierror.CodeNoWorkspace {
		t.Fatalf("refusal code = %q, want %q", code, apierror.CodeNoWorkspace)
	}

	var memberships int64
	if err := db.Model(&models.OrgMember{}).Where("user_id = ?", user.ID).Count(&memberships).Error; err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberships != 0 {
		t.Fatalf("sign-in created %d membership rows; it must create none", memberships)
	}
}

// TestLoginLandsInTheExistingWorkspace guards the other side of the change:
// an account that does belong somewhere still signs in, into that workspace.
func TestLoginLandsInTheExistingWorkspace(t *testing.T) {
	srv, db := newTestHandler(t)
	user := seedPasswordUser(t, db, "member@example.com")
	org := models.Org{Name: "member workspace", Slug: "member-workspace", InboundToken: "member-token"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	if got := loginOrgID(t, srv, "member@example.com"); got != org.ID {
		t.Fatalf("session landed in workspace %d, want %d", got, org.ID)
	}
}

// TestTwoFactorLoginWithoutWorkspaceIsRefused covers the second door into the
// same session. The endpoint re-checks the password, so it must apply the same
// refusal rather than issuing a workspace-less session.
func TestTwoFactorLoginWithoutWorkspaceIsRefused(t *testing.T) {
	srv, db := newTestHandler(t)
	seedPasswordUser(t, db, "removed2fa@example.com")

	rec := do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"removed2fa@example.com","password":"pw","code":"123456"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("2fa login of an account in no workspace: got %d (%s), want 403", rec.Code, rec.Body.String())
	}
	assertNoCookies(t, rec, "refused 2fa login")
	if code := refusedCode(t, rec); code != apierror.CodeNoWorkspace {
		t.Fatalf("refusal code = %q, want %q", code, apierror.CodeNoWorkspace)
	}
}

// TestLoginRequiresVerificationReportsItsOwnCode keeps the two 403 refusals
// apart. This one tells the client to offer the verification email; the
// no-workspace one tells it to ask an administrator for an invitation.
func TestLoginRequiresVerificationReportsItsOwnCode(t *testing.T) {
	srv, db := newTestHandler(t)
	user := seedPasswordUser(t, db, "unverified@example.com")
	if err := db.Model(&user).Update("email_verified", false).Error; err != nil {
		t.Fatalf("clear verification: %v", err)
	}
	org := models.Org{Name: "unverified workspace", Slug: "unverified-workspace", InboundToken: "unverified-token"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&models.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"unverified@example.com","password":"pw"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login before email verification: got %d (%s), want 403", rec.Code, rec.Body.String())
	}
	if code := refusedCode(t, rec); code != apierror.CodeEmailVerificationRequired {
		t.Fatalf("refusal code = %q, want %q", code, apierror.CodeEmailVerificationRequired)
	}
}

// TestLoginWrongPasswordStaysUnauthorized keeps the two refusals apart: only a
// correct password learns that the account exists.
func TestLoginWrongPasswordStaysUnauthorized(t *testing.T) {
	srv, db := newTestHandler(t)
	seedPasswordUser(t, db, "removed3@example.com")

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"removed3@example.com","password":"wrong-password"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login with a wrong password: got %d, want 401", rec.Code)
	}
}

// TestLoginReportsStorageFault: a failure to read the membership is not a
// credential verdict. Folding it into "invalid credentials" would tell a
// correctly authenticated user their password is wrong.
func TestLoginReportsStorageFault(t *testing.T) {
	srv, db := newTestHandler(t)
	seedPasswordUser(t, db, "fault@example.com")
	if err := db.Migrator().DropTable(&models.OrgMember{}); err != nil {
		t.Fatalf("drop membership table: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"fault@example.com","password":"pw"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("login over a storage fault: got %d (%s), want 500", rec.Code, rec.Body.String())
	}
}

// TestTwoFactorLoginReportsStorageFault covers the same branch on the second
// door into a session.
func TestTwoFactorLoginReportsStorageFault(t *testing.T) {
	srv, db := newTestHandler(t)
	seedPasswordUser(t, db, "fault2fa@example.com")
	if err := db.Migrator().DropTable(&models.OrgMember{}); err != nil {
		t.Fatalf("drop membership table: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/2fa/verify", nil, `{"email":"fault2fa@example.com","password":"pw","code":"123456"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("2fa login over a storage fault: got %d (%s), want 500", rec.Code, rec.Body.String())
	}
}
