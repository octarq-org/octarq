package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestChangeEmailSuccess(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, db, "user1@example.com", "password123")

	// Mark user verified initially
	if err := db.Model(&models.User{}).Where("email = ?", "user1@example.com").Update("email_verified", true).Error; err != nil {
		t.Fatalf("failed to update email_verified: %v", err)
	}

	rec := do(srv, "PUT", "/api/auth/email", cookies, `{"newEmail":"newuser1@example.com","currentPassword":"password123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change email: got %d (%s)", rec.Code, rec.Body.String())
	}

	var user models.User
	if err := db.Where("email = ?", "newuser1@example.com").First(&user).Error; err != nil {
		t.Fatalf("new email not found in DB: %v", err)
	}

	if user.EmailVerified {
		t.Fatal("expected EmailVerified to be false after email change, got true")
	}

	// Verify old email no longer exists for this user
	var oldUser models.User
	if err := db.Where("email = ?", "user1@example.com").First(&oldUser).Error; err == nil {
		t.Fatal("old email still exists in DB")
		return
	}
}

func TestChangeEmailWrongPassword(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, db, "user2@example.com", "password123")

	rec := do(srv, "PUT", "/api/auth/email", cookies, `{"newEmail":"newuser2@example.com","currentPassword":"wrongpassword"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong password, got %d (%s)", rec.Code, rec.Body.String())
	}

	var user models.User
	if err := db.Where("email = ?", "user2@example.com").First(&user).Error; err != nil {
		t.Fatalf("original email should remain: %v", err)
	}
}

func TestChangeEmailConflict(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies1 := registerUser(t, srv, db, "user3@example.com", "password123")
	_ = registerUser(t, srv, db, "occupied@example.com", "password123")

	rec := do(srv, "PUT", "/api/auth/email", cookies1, `{"newEmail":"occupied@example.com","currentPassword":"password123"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestChangeEmailSSOUserRejected(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	ssoUser := models.User{
		Email:        "sso@example.com",
		PasswordHash: "",
	}
	if err := db.Create(&ssoUser).Error; err != nil {
		t.Fatalf("failed to create sso user: %v", err)
	}

	org := models.Org{Name: "sso-org", Slug: "sso-org"}
	db.Create(&org)
	db.Create(&models.OrgMember{OrgID: org.ID, UserID: ssoUser.ID, Role: "owner"})

	rec := httptest.NewRecorder()
	h.auth.SetSession(rec, httptest.NewRequest(http.MethodGet, "/", nil), ssoUser.ID, org.ID)
	cookies := rec.Result().Cookies()

	res := do(srv, "PUT", "/api/auth/email", cookies, `{"newEmail":"sso-new@example.com"}`)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "external identity provider") {
		t.Fatalf("expected 400 with external identity provider message, got %d (%s)", res.Code, res.Body.String())
	}
}

func TestChangeEmailUnauthenticated(t *testing.T) {
	srv, _ := newTestHandler(t)
	rec := do(srv, "PUT", "/api/auth/email", nil, `{"newEmail":"unauth@example.com","currentPassword":"password123"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated, got %d (%s)", rec.Code, rec.Body.String())
	}
}
