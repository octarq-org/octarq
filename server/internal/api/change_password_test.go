package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func registerUser(t *testing.T, srv http.Handler, db *gorm.DB, email, password string) []*http.Cookie {
	t.Helper()
	disableEmailVerification(t, db)
	enableMail(t, db)
	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"`+email+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register %s: got %d (%s)", email, rec.Code, rec.Body.String())
	}
	rec = do(srv, "POST", "/api/auth/login", nil, `{"email":"`+email+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: got %d (%s)", email, rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

func TestChangePasswordReplacesHashAndKeepsCallerSignedIn(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, db, "owner@example.com", "originalpw1")

	var before models.User
	if err := db.Where("email = ?", "owner@example.com").First(&before).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/password", cookies,
		`{"currentPassword":"originalpw1","newPassword":"replacementpw2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: got %d (%s)", rec.Code, rec.Body.String())
	}

	var after models.User
	db.First(&after, before.ID)
	if after.PasswordHash == before.PasswordHash {
		t.Fatal("password hash unchanged")
	}
	if bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("replacementpw2")) != nil {
		t.Fatal("stored hash does not verify against the new password")
	}

	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("me after change: got %d, want 200", rec.Code)
	}

	if rec := do(srv, "POST", "/api/auth/login", nil,
		`{"email":"owner@example.com","password":"replacementpw2"}`); rec.Code != http.StatusOK {
		t.Fatalf("login with new password: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "POST", "/api/auth/login", nil,
		`{"email":"owner@example.com","password":"originalpw1"}`); rec.Code == http.StatusOK {
		t.Fatal("login with the old password still succeeds")
	}
}

func doUA(srv http.Handler, method, path string, cookies []*http.Cookie, body, ua string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", ua)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestChangePasswordRevokesOtherSessionsOnly(t *testing.T) {
	const agentA = "device-a"
	const agentB = "device-b"

	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)
	rec := doUA(srv, "POST", "/api/auth/register", nil,
		`{"email":"owner@example.com","password":"originalpw1"}`, agentA)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doUA(srv, "POST", "/api/auth/login", nil,
		`{"email":"owner@example.com","password":"originalpw1"}`, agentA)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d (%s)", rec.Code, rec.Body.String())
	}
	first := rec.Result().Cookies()

	rec = doUA(srv, "POST", "/api/auth/login", nil,
		`{"email":"owner@example.com","password":"originalpw1"}`, agentB)
	if rec.Code != http.StatusOK {
		t.Fatalf("second login: got %d (%s)", rec.Code, rec.Body.String())
	}
	second := rec.Result().Cookies()
	if rec := doUA(srv, "GET", "/api/auth/me", second, "", agentB); rec.Code != http.StatusOK {
		t.Fatalf("second session should work before the change: got %d", rec.Code)
	}

	if rec := doUA(srv, "POST", "/api/auth/password", first,
		`{"currentPassword":"originalpw1","newPassword":"replacementpw2"}`, agentA); rec.Code != http.StatusOK {
		t.Fatalf("change password: got %d (%s)", rec.Code, rec.Body.String())
	}

	if rec := doUA(srv, "GET", "/api/auth/me", second, "", agentB); rec.Code != http.StatusUnauthorized {
		t.Fatalf("other session after change: got %d, want 401", rec.Code)
	}
	if rec := doUA(srv, "GET", "/api/auth/me", first, "", agentA); rec.Code != http.StatusOK {
		t.Fatalf("caller's own session after change: got %d, want 200", rec.Code)
	}

	var user models.User
	db.Where("email = ?", "owner@example.com").First(&user)
	var count int64
	db.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("sessions left for user: got %d, want 1 (the caller's)", count)
	}
}

func TestChangePasswordRejectsBadInput(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, db, "owner@example.com", "originalpw1")

	var before models.User
	db.Where("email = ?", "owner@example.com").First(&before)

	cases := []struct {
		name string
		body string
	}{
		{"wrong current password", `{"currentPassword":"notitatall","newPassword":"replacementpw2"}`},
		{"empty current password", `{"currentPassword":"","newPassword":"replacementpw2"}`},
		{"new password too short", `{"currentPassword":"originalpw1","newPassword":"short7c"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(srv, "POST", "/api/auth/password", cookies, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d (%s), want 400", rec.Code, rec.Body.String())
			}
			var after models.User
			db.First(&after, before.ID)
			if after.PasswordHash != before.PasswordHash {
				t.Fatal("a rejected request still changed the stored hash")
			}
		})
	}
}

func TestChangePasswordRequiresASession(t *testing.T) {
	srv, _ := newTestHandler(t)
	rec := do(srv, "POST", "/api/auth/password", nil,
		`{"currentPassword":"originalpw1","newPassword":"replacementpw2"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated change: got %d (%s), want 401", rec.Code, rec.Body.String())
	}
}

func TestChangePasswordRefusesAccountWithNoStoredPassword(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/auth/password", cookies,
		`{"currentPassword":"pw","newPassword":"replacementpw2"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bootstrap admin change: got %d (%s), want 400", rec.Code, rec.Body.String())
	}

	var admin models.User
	if err := db.Where("password_hash = ?", "").First(&admin).Error; err != nil {
		t.Skip("no passwordless account in this fixture")
	}
	if admin.PasswordHash != "" {
		t.Fatal("refused request still stored a hash")
	}
}

func TestChangePasswordInvalidatesAnOutstandingResetToken(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, db, "target@example.com", "originalpw1")

	if rec := do(srv, "POST", "/api/auth/forgot", nil,
		`{"email":"target@example.com"}`); rec.Code != http.StatusOK {
		t.Fatalf("forgot: got %d (%s)", rec.Code, rec.Body.String())
	}

	var issued models.User
	if err := db.Where("email = ?", "target@example.com").First(&issued).Error; err != nil {
		t.Fatalf("user missing: %v", err)
	}
	if issued.ResetTokenHash == "" {
		t.Fatal("no reset token was issued — this test would pass without proving anything")
	}

	if rec := do(srv, "POST", "/api/auth/password", cookies,
		`{"currentPassword":"originalpw1","newPassword":"chosenbyowner2"}`); rec.Code != http.StatusOK {
		t.Fatalf("change password: got %d (%s)", rec.Code, rec.Body.String())
	}

	var after models.User
	db.First(&after, issued.ID)
	if after.ResetTokenHash != "" {
		t.Error("reset token hash survived the password change")
	}
	if after.ResetTokenExpiry != nil {
		t.Error("reset token expiry survived the password change")
	}

	if bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("chosenbyowner2")) != nil {
		t.Fatal("the owner's new password is not the stored one")
	}
	if rec := do(srv, "POST", "/api/auth/login", nil,
		`{"email":"target@example.com","password":"chosenbyowner2"}`); rec.Code != http.StatusOK {
		t.Fatalf("owner cannot log in with the password they chose: %d", rec.Code)
	}
}
