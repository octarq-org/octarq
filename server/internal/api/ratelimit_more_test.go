package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

func TestRateLimiterMore(t *testing.T) {
	// 1. In-memory store
	rl := newRateLimiter("", "test", 3, time.Minute)

	// Initial requests allowed
	if !rl.allow("1.2.3.4") {
		t.Error("expected allow initially")
	}

	// 2. Record 3 failures
	rl.recordFailure("1.2.3.4")
	rl.recordFailure("1.2.3.4")
	rl.recordFailure("1.2.3.4")

	// 3. Now rejected
	if rl.allow("1.2.3.4") {
		t.Error("expected reject after 3 failures")
	}

	// 4. Reset
	rl.reset("1.2.3.4")
	if !rl.allow("1.2.3.4") {
		t.Error("expected allow after reset")
	}

	// 5. Test with unreachable redis URL (falls back to memory store)
	rlRedisFail := newRateLimiter("redis://127.0.0.1:65534", "test", 2, time.Minute)
	if !rlRedisFail.allow("5.6.7.8") {
		t.Error("expected allow on fallback memory store")
	}
	rlRedisFail.recordFailure("5.6.7.8")
	rlRedisFail.recordFailure("5.6.7.8")
	if rlRedisFail.allow("5.6.7.8") {
		t.Error("expected reject after 2 failures")
	}

	// 6. Test with invalid redis URL
	rlInvalidRedis := newRateLimiter("invalid://redis", "test", 5, time.Minute)
	if !rlInvalidRedis.allow("9.9.9.9") {
		t.Error("expected allow on invalid redis url fallback")
	}
}

func TestTokensMore(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	ctx := context.Background()

	// 1. tokenPrefix helper
	if p := tokenPrefix("oct_1234567890"); p != "oct_1234" {
		t.Errorf("tokenPrefix = %q, want oct_1234", p)
	}
	if p := tokenPrefix("short"); p != "short" {
		t.Errorf("tokenPrefix(\"short\") = %q, want short", p)
	}

	// 2. newRawToken helper
	tok := newRawToken()
	if len(tok) < 10 || tok[:4] != "oct_" {
		t.Errorf("newRawToken() = %q, want oct_ prefix", tok)
	}

	// 3. Nil Ctx guards on token methods
	if _, err := h.listTokens(ctx, &ListTokensInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listTokens")
	}
	if _, err := h.createToken(ctx, &CreateTokenInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in createToken")
	}
	if _, err := h.updateToken(ctx, &UpdateTokenInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateToken")
	}
	if _, err := h.deleteToken(ctx, &DeleteTokenInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in deleteToken")
	}

	// 4. Direct unauthenticated calls (humaCtx with unauthenticated request)
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	humaCtx := humago.NewContext(nil, req, w)

	if _, err := h.listTokens(ctx, &ListTokensInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct listTokens")
	}
	if _, err := h.createToken(ctx, &CreateTokenInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct createToken")
	}
	if _, err := h.updateToken(ctx, &UpdateTokenInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct updateToken")
	}
	if _, err := h.deleteToken(ctx, &DeleteTokenInput{Ctx: humaCtx}); err == nil {
		t.Error("expected 401 for direct deleteToken")
	}
}

func TestTokensCRUDMore(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	// 1. List tokens unauth -> 401
	rec := do(srv, "GET", "/api/tokens", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list tokens: got %d, want 401", rec.Code)
	}

	// 2. List tokens auth -> 200
	rec = do(srv, "GET", "/api/tokens", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth list tokens: got %d", rec.Code)
	}

	// 3. Create token empty name -> 400
	rec = do(srv, "POST", "/api/tokens", cookies, `{"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create token empty name: got %d, want 400", rec.Code)
	}

	// 4. Create token valid -> 200/201
	rec = do(srv, "POST", "/api/tokens", cookies, `{"name":"CLI Token","role":"admin","expiresInDays":30}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create token valid: got %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
		ID     uint   `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Token == "" || created.ID == 0 {
		t.Fatalf("unexpected created token: %+v", created)
	}

	// 5. Update token
	rec = do(srv, "PUT", fmt.Sprintf("/api/tokens/%d", created.ID), cookies, `{"name":"Renamed CLI Token","note":"updated note"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update token: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 6. Delete token
	rec = do(srv, "DELETE", fmt.Sprintf("/api/tokens/%d", created.ID), cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete token: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Delete again -> 404
	rec = do(srv, "DELETE", fmt.Sprintf("/api/tokens/%d", created.ID), cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete nonexistent token: got %d, want 404", rec.Code)
	}

	// 7. Token validation cases
	// Negative expiresInDays -> 400
	rec = do(srv, "POST", "/api/tokens", cookies, `{"name":"T","expiresInDays":-5}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative expiresInDays: got %d, want 400", rec.Code)
	}

	// Invalid role -> 400
	rec = do(srv, "POST", "/api/tokens", cookies, `{"name":"T","role":"superadmin"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid role token: got %d, want 400", rec.Code)
	}

	// Member trying to mint owner token -> 403
	memUser := models.User{Email: "tokenmember@example.com"}
	db.Create(&memUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memUser.ID, Role: "member"})
	memCookies := sessionCookies(t, memUser.ID, 1)
	rec = do(srv, "POST", "/api/tokens", memCookies, `{"name":"T","role":"owner"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member mint owner token: got %d, want 403", rec.Code)
	}

	// validTokenRole helper
	if !validTokenRole("member") || !validTokenRole("admin") || !validTokenRole("owner") {
		t.Error("expected validTokenRole to return true for standard roles")
	}
	if validTokenRole("guest") {
		t.Error("expected validTokenRole to return false for guest")
	}
}
