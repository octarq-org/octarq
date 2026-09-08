package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func TestLogin(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)

	// Create a user in the database
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	user := models.User{
		Email:         "test@example.com",
		PasswordHash:  string(hash),
		EmailVerified: true,
	}
	db.Create(&user)

	t.Run("success", func(t *testing.T) {
		rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"test@example.com","password":"password123"}`)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Error("expected session cookie, got none")
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["email"] != "test@example.com" {
			t.Errorf("expected email in response to be test@example.com, got %v", resp["email"])
		}
	})

	t.Run("invalid_credentials", func(t *testing.T) {
		rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"test@example.com","password":"wrongpassword"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("nonexistent_user", func(t *testing.T) {
		rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"nobody@example.com","password":"password123"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("rate_limit", func(t *testing.T) {
		gotRateLimit := false
		for i := 0; i < 10; i++ {
			rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"nobody@example.com","password":"password123"}`)
			if rec.Code == http.StatusTooManyRequests {
				gotRateLimit = true
				break
			}
		}
		if !gotRateLimit {
			t.Error("expected to eventually get 429 Too Many Requests")
		}
	})
}
