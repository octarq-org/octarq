package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestUpsertUserByEmailCreatesUserAndOrg(t *testing.T) {
	db := testDB(t)

	uid, orgID, err := UpsertUserByEmail(db, "Alice@Example.com", true)
	if err != nil {
		t.Fatalf("UpsertUserByEmail: %v", err)
	}
	if uid == 0 || orgID == 0 {
		t.Fatalf("expected non-zero ids, got uid=%d orgID=%d", uid, orgID)
	}

	var user models.User
	if err := db.First(&user, uid).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q, want lowercased", user.Email)
	}
	if user.IsInstanceAdmin {
		t.Error("JIT-provisioned user must not be an instance admin")
	}

	uid2, orgID2, err := UpsertUserByEmail(db, "alice@example.com", true)
	if err != nil {
		t.Fatalf("second UpsertUserByEmail: %v", err)
	}
	if uid2 != uid || orgID2 != orgID {
		t.Errorf("not idempotent: (%d,%d) != (%d,%d)", uid2, orgID2, uid, orgID)
	}
}

func TestUpsertUserByEmailRespectsRegistrationPolicy(t *testing.T) {
	db := testDB(t)

	if _, _, err := UpsertUserByEmail(db, "stranger@example.com", false); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled, got %v", err)
	}
	var count int64
	db.Model(&models.User{}).Where("email = ?", "stranger@example.com").Count(&count)
	if count != 0 {
		t.Errorf("no user should have been created, found %d", count)
	}

	uid, _, err := UpsertUserByEmail(db, "known@example.com", true)
	if err != nil {
		t.Fatalf("seed known user: %v", err)
	}
	uid2, _, err := UpsertUserByEmail(db, "known@example.com", false)
	if err != nil {
		t.Fatalf("known user must resolve with registration off: %v", err)
	}
	if uid2 != uid {
		t.Errorf("resolved different user: %d != %d", uid2, uid)
	}
}

func TestUpsertUserByEmailRejectsEmpty(t *testing.T) {
	db := testDB(t)
	if _, _, err := UpsertUserByEmail(db, "   ", true); err == nil {
		t.Fatal("expected error for empty email")
		return
	}
}

func TestLoginByEmailIssuesSession(t *testing.T) {
	db := testDB(t)
	db.Where("key = ?", "allow_registration").Delete(&models.Setting{})
	m := testManager(t).WithDB(db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sso/callback", nil)

	uid, err := m.LoginByEmail(rec, req, "bob@example.com")
	if err != nil {
		t.Fatalf("LoginByEmail: %v", err)
	}
	if uid == 0 {
		t.Fatal("expected a user id")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie to be set")
	}
	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ?", uid).Count(&sessions)
	if sessions == 0 {
		t.Error("expected a session row for the logged-in user")
	}
}

func TestLoginByEmailHonoursInviteOnly(t *testing.T) {
	db := testDB(t)
	if err := db.Save(&models.Setting{Key: "allow_registration", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}
	m := testManager(t).WithDB(db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sso/callback", nil)
	if _, err := m.LoginByEmail(rec, req, "outsider@example.com"); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("expected ErrRegistrationDisabled on invite-only instance, got %v", err)
	}
}
