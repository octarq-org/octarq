package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func TestRegisterCreatesUserAndOrg(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"new@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("register: got %d cookie(s), want none", len(cookies))
	}

	var user models.User
	if err := db.Where("email = ?", "new@user.com").First(&user).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.PasswordHash == "" {
		t.Fatal("register: password hash not stored")
	}
	var member models.OrgMember
	if err := db.Where("user_id = ?", user.ID).First(&member).Error; err != nil {
		t.Fatalf("membership not created: %v", err)
	}
	if member.Role != "owner" {
		t.Fatalf("role: got %q, want owner", member.Role)
	}

	rec = do(srv, "POST", "/api/auth/login", nil, `{"email":"new@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login after register: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestRegisterHidesAnExistingAccount(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	first := do(srv, "POST", "/api/auth/register", nil, `{"email":"taken@user.com","password":"hunter2pw"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first register: got %d (%s)", first.Code, first.Body.String())
	}
	second := do(srv, "POST", "/api/auth/register", nil, `{"email":"TAKEN@user.com","password":"differentpw"}`)
	if second.Code != first.Code {
		t.Fatalf("second register: got %d, want %d — the two answers must match", second.Code, first.Code)
	}
	if second.Body.String() != first.Body.String() {
		t.Fatalf("second register body = %s, want %s — the two answers must match", second.Body.String(), first.Body.String())
	}
	if cookies := second.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("second register: got %d cookie(s), want none", len(cookies))
	}
	var count int64
	if err := db.Model(&models.User{}).Where("LOWER(email) = ?", "taken@user.com").Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("user rows for the address: got %d, want 1", count)
	}
	var user models.User
	if err := db.Where("LOWER(email) = ?", "taken@user.com").First(&user).Error; err != nil {
		t.Fatalf("user not found: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("hunter2pw")) != nil {
		t.Fatal("the second sign-up changed the existing account's password")
	}
}

func TestRegisterRejectsShortPassword(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"a@b.com","password":"short"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: got %d, want 400", rec.Code)
	}
}

func TestRegisterDisabledByToggle(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyAllowRegistration, Value: "false"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	enableMail(t, db)
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"x@y.com","password":"longenough"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("register while disabled: got %d, want 403", rec.Code)
	}
}

func TestRegisterUsesOrgNameOverEmail(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"named@user.com","password":"hunter2pw","orgName":"  Acme Corp  "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register with orgName: got %d (%s)", rec.Code, rec.Body.String())
	}
	var named models.Org
	if err := db.Where("name = ?", "Acme Corp").First(&named).Error; err != nil {
		t.Fatalf("org named after orgName not created: %v", err)
	}

	rec = do(srv, "POST", "/api/auth/register", nil, `{"email":"fallback@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register without orgName: got %d (%s)", rec.Code, rec.Body.String())
	}
	var fallback models.Org
	if err := db.Where("name = ?", "fallback@user.com").First(&fallback).Error; err != nil {
		t.Fatalf("org falling back to email not created: %v", err)
	}

	long := strings.Repeat("x", 256)
	enableMail(t, db)
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"long@user.com","password":"hunter2pw","orgName":"`+long+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("register with oversized orgName: got %d, want 400", rec.Code)
	}
}

func TestDBUserPasswordLogin(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

	enableMail(t, db)
	if rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"member@corp.com","password":"correcthorse"}`); rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"member@corp.com","password":"correcthorse"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("db-user login: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "GET", "/api/auth/me", rec.Result().Cookies(), ""); rec.Code != http.StatusOK {
		t.Fatalf("me after db-user login: got %d, want 200", rec.Code)
	}

	if rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"member@corp.com","password":"wrongpass"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad password: got %d, want 401", rec.Code)
	}
}

type registerBody struct {
	OK    bool   `json:"ok"`
	Email string `json:"email"`
}

func TestRegisterWithVerificationGateGivesNoSession(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"gated@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 0 {
		t.Fatalf("register with gate on: got %d cookie(s), want none", len(cookies))
	}
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code == http.StatusOK {
		t.Fatal("register with gate on: response authenticates /api/auth/me, want rejected")
	}

	var user models.User
	if err := db.Where("email = ?", "gated@user.com").First(&user).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.EmailVerified {
		t.Fatal("fresh account should start unverified")
	}
}

func TestRegisterWithoutVerificationGateStillGivesNoSession(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"open@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("register with gate off: got %d cookie(s), want none", len(cookies))
	}
}

func TestRegisterFailsLoudlyWhenMailUnavailable(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Save(&models.Setting{Key: keyRequireEmailVerification, Value: "true"}).Error; err != nil {
		t.Fatalf("set setting: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"stuck@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("register on mail-less instance: got %d (%s), want 503", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cannot send email") {
		t.Fatalf("register 503 does not explain the failure to the user: %s", rec.Body.String())
	}

	var count int64
	db.Model(&models.User{}).Where("email = ?", "stuck@user.com").Count(&count)
	if count != 0 {
		t.Fatal("register 503 still created an account")
	}
}

func TestRegisterWithoutMailAndWithoutGateStillFails(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"nogate@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("register on mail-less instance with gate off: got %d (%s), want 503", rec.Code, rec.Body.String())
	}
}

func TestRegisterRateLimitedPerIP(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)

	for i := 0; i < 5; i++ {
		rec := do(srv, "POST", "/api/auth/register", nil, fmt.Sprintf(`{"email":"user%d@rate.test","password":"hunter2pw"}`, i))
		if rec.Code != http.StatusOK {
			t.Fatalf("register %d: got %d (%s), want 200", i+1, rec.Code, rec.Body.String())
		}
	}

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"user5@rate.test","password":"hunter2pw"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th register from the same IP: got %d, want 429", rec.Code)
	}
}
