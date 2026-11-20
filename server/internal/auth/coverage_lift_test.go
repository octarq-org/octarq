package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestCoverageLift_Auth_ContextAndIPHelpers(t *testing.T) {
	t.Parallel()

	ctxExplicit := WithUserID(context.Background(), 101)
	if got := UserIDFromContext(ctxExplicit); got != 101 {
		t.Fatalf("expected 101 from WithUserID context, got %d", got)
	}

	ctxTenant := tenantsql.WithUserID(context.Background(), 202)
	if got := UserIDFromContext(ctxTenant); got != 202 {
		t.Fatalf("expected 202 from tenantsql context, got %d", got)
	}

	if got := UserIDFromContext(context.Background()); got != 0 {
		t.Fatalf("expected 0 for empty context, got %d", got)
	}

	reqNoPort := httptest.NewRequest(http.MethodGet, "/", nil)
	reqNoPort.RemoteAddr = "192.168.1.1"
	if got := reporterIP(reqNoPort); got != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", got)
	}
}

func TestCoverageLift_Auth_Manager_NilDB(t *testing.T) {
	t.Parallel()
	mNoDB := &Manager{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if tok, ok := mNoDB.TokenByRequest(req); ok || tok.ID != 0 {
		t.Fatalf("expected false from TokenByRequest with nil db")
	}

	if count := mNoDB.RevokeUserOrgSessions(0, 0); count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	if count := mNoDB.RevokeUserOrgSessions(1, 0); count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	if count := mNoDB.RevokeUserOrgSessions(0, 1); count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	if count := mNoDB.RevokeUserOrgTokens(0, 0); count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	if count := mNoDB.RevokeUserOrgTokens(1, 0); count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
	if count := mNoDB.RevokeUserOrgTokens(0, 1); count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	mNoDB.TouchSession(req)

	w := httptest.NewRecorder()
	if _, err := mNoDB.LoginByIdentity(w, req, plugin.ExternalIdentity{}); err == nil {
		t.Fatalf("expected error from LoginByIdentity with nil db")
	}
}

func TestCoverageLift_Auth_TwoFAChallenge_Edges(t *testing.T) {
	t.Parallel()
	m := testManager(t)

	if uid := m.VerifyTwoFAChallenge("no-dot-token"); uid != 0 {
		t.Fatalf("expected 0 for malformed token without dot, got %d", uid)
	}

	if uid := m.VerifyTwoFAChallenge("???.signature"); uid != 0 {
		t.Fatalf("expected 0 for invalid payload base64, got %d", uid)
	}

	tok, err := m.NewTwoFAChallenge(55)
	if err != nil {
		t.Fatalf("NewTwoFAChallenge failed: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}

	if uid := m.VerifyTwoFAChallenge(parts[0] + ".???"); uid != 0 {
		t.Fatalf("expected 0 for invalid signature base64, got %d", uid)
	}

	if uid := m.VerifyTwoFAChallenge(parts[0] + "." + parts[1] + "tampered"); uid != 0 {
		t.Fatalf("expected 0 for tampered signature, got %d", uid)
	}
}

func TestCoverageLift_Auth_ProvisionAndIdentity_Edges(t *testing.T) {
	db := testDB(t)

	if _, _, err := UpsertUserByEmail(db, "   ", true); err == nil {
		t.Fatalf("expected error for empty email")
	}

	if _, _, err := UpsertUserByEmail(db, "notallowed@example.com", false); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled, got %v", err)
	}

	if registrationAllowed(nil) {
		t.Fatalf("expected false for nil db in registrationAllowed")
	}

	unverified := models.User{Email: "verify_me@example.com", EmailVerified: false}
	if err := db.Create(&unverified).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	uid, _, err := UpsertUserByEmail(db, "verify_me@example.com", false)
	if err != nil {
		t.Fatalf("UpsertUserByEmail failed: %v", err)
	}
	var reloaded models.User
	if err := db.First(&reloaded, uid).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if !reloaded.EmailVerified {
		t.Fatalf("expected email to be verified")
	}

	if _, _, err := resolveIdentity(db, plugin.ExternalIdentity{Provider: "", Subject: "s"}); err == nil {
		t.Fatalf("expected error for empty provider")
	}
	if _, _, err := resolveIdentity(db, plugin.ExternalIdentity{Provider: "p", Subject: ""}); err == nil {
		t.Fatalf("expected error for empty subject")
	}

	if _, _, err := resolveIdentity(db, plugin.ExternalIdentity{Provider: "p", Subject: "s", Email: ""}); err == nil {
		t.Fatalf("expected error for missing email claim")
	}

	if _, _, err := resolveIdentity(db, plugin.ExternalIdentity{Provider: "p", Issuer: "iss", Subject: "s_link", Email: "verify_me@example.com"}); !errors.Is(err, ErrAccountLinkRequired) {
		t.Fatalf("expected ErrAccountLinkRequired, got %v", err)
	}

	if _, _, err := resolveIdentity(db, plugin.ExternalIdentity{Provider: "p", Issuer: "iss", Subject: "s_nojit", Email: "brandnew@example.com", MayCreateUser: false}); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled, got %v", err)
	}

	if _, _, err := joinOrg(db, unverified.ID, plugin.ExternalIdentity{OrgID: 888, AllowJIT: false}); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled from joinOrg")
	}

	noOrgUser := models.User{Email: "personal_org@example.com", EmailVerified: true}
	if err := db.Create(&noOrgUser).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	newOrgID, err := firstOrPersonalOrg(db, noOrgUser.ID)
	if err != nil || newOrgID == 0 {
		t.Fatalf("firstOrPersonalOrg failed: %v, orgID: %d", err, newOrgID)
	}
}

func TestCoverageLift_Auth_OAuth_Edges(t *testing.T) {
	db := testDB(t)
	m := testManager(t)
	h := NewOAuthHandler(db, m, nil)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/auth/login/test", nil)

	store := requestSecureStore{CookieStore: sessions.NewCookieStore([]byte("secret"))}
	store.markSecure(req, nil)

	sess := &sessions.Session{Options: nil}
	store.markSecure(req, sess)
	if sess.Options == nil {
		t.Fatalf("expected sess.Options to be initialized")
	}

	if h.loadProvider(context.Background(), "unsupported_oauth", "name", "http://example.com") {
		t.Fatalf("expected false for unsupported provider")
	}

	if h.loadProvider(context.Background(), "google", "google@example.com", "http://example.com") {
		t.Fatalf("expected false when credentials missing from settings")
	}

	if _, ok := h.prepare(req, "unsupported_provider"); ok {
		t.Fatalf("expected false for unsupported provider in prepare")
	}
}
