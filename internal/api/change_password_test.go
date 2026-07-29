package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// registerUser signs up a real (non-bootstrap) account and returns its session
// cookies. The bootstrap admin can't be used here: its credentials come from
// the environment and its user row has an empty PasswordHash, which the change
// endpoint deliberately refuses.
func registerUser(t *testing.T, srv http.Handler, email, password string) []*http.Cookie {
	t.Helper()
	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"`+email+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register %s: got %d (%s)", email, rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

func TestChangePasswordReplacesHashAndKeepsCallerSignedIn(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := registerUser(t, srv, "owner@example.com", "originalpw1")

	var before models.User
	if err := db.Where("email = ?", "owner@example.com").First(&before).Error; err != nil {
		t.Fatalf("user not created: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/password", cookies,
		`{"currentPassword":"originalpw1","newPassword":"replacementpw2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: got %d (%s)", rec.Code, rec.Body.String())
	}

	// The stored hash actually changed, and it verifies against the NEW secret.
	// Checking bcrypt rather than just "the row is different" is the point: the
	// bug this endpoint replaces was a handler that reported success without
	// touching anything at all.
	var after models.User
	db.First(&after, before.ID)
	if after.PasswordHash == before.PasswordHash {
		t.Fatal("password hash unchanged")
	}
	if bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("replacementpw2")) != nil {
		t.Fatal("stored hash does not verify against the new password")
	}

	// The caller keeps working — changing your password must not sign you out
	// of the page you changed it on.
	if rec := do(srv, "GET", "/api/auth/me", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("me after change: got %d, want 200", rec.Code)
	}

	// And the new password is the one that logs in.
	if rec := do(srv, "POST", "/api/auth/login", nil,
		`{"username":"owner@example.com","password":"replacementpw2"}`); rec.Code != http.StatusOK {
		t.Fatalf("login with new password: got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "POST", "/api/auth/login", nil,
		`{"username":"owner@example.com","password":"originalpw1"}`); rec.Code == http.StatusOK {
		t.Fatal("login with the old password still succeeds")
	}
}

// doUA is do() with a User-Agent. Sessions are keyed on
// (user, ip, user_agent) — a second login with the same fingerprint refreshes
// the existing row and rotates its token rather than creating a second session
// — so two concurrent sessions in a test require two distinct agents.
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
	rec := doUA(srv, "POST", "/api/auth/register", nil,
		`{"email":"owner@example.com","password":"originalpw1"}`, agentA)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	first := rec.Result().Cookies()

	// A second sign-in from another device.
	rec = doUA(srv, "POST", "/api/auth/login", nil,
		`{"username":"owner@example.com","password":"originalpw1"}`, agentB)
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

	// Locking out the other sessions is half the reason to change a password in
	// a hurry, so it is asserted rather than assumed.
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
	cookies := registerUser(t, srv, "owner@example.com", "originalpw1")

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

// The bootstrap admin authenticates against OCTARQ_ADMIN_PASSWORD, checked
// before any stored hash, so writing one for that account would leave the env
// password working and report a change that changed nothing.
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
