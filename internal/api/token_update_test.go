package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

func TestTokenUpdate(t *testing.T) {
	srv, db := newTestHandler(t)

	// Setup users and tokens in Org 1
	// User 10: Owner in Org 1
	seedMember(t, db, 10, "owner")
	rawOwner := "oct_owner00000000000000000000000001"
	seedToken(t, db, rawOwner, 10, "owner")

	// User 11: Admin in Org 1
	seedMember(t, db, 11, "admin")
	rawAdmin := "oct_admin00000000000000000000000001"
	seedToken(t, db, rawAdmin, 11, "admin")

	// User 12: Member in Org 1
	seedMember(t, db, 12, "member")
	rawMember := "oct_member0000000000000000000000001"
	seedToken(t, db, rawMember, 12, "member")

	// Target token in Org 1
	rawTarget := "oct_target0000000000000000000000001"
	seedToken(t, db, rawTarget, 10, "owner")
	var tokTarget models.Token
	if err := db.Where("prefix = ?", tokenPrefix(rawTarget)).First(&tokTarget).Error; err != nil {
		t.Fatalf("find target token: %v", err)
	}

	putJSON := func(rawToken, path string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if rawToken != "" {
			req.Header.Set("Authorization", "Bearer "+rawToken)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	// 1. Admin updates name/note -> 200, fields applied
	t.Run("admin update name and note", func(t *testing.T) {
		name := "updated-name"
		note := "updated-note"
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"name": name,
			"note": note,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("got code %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var updated models.Token
		db.First(&updated, tokTarget.ID)
		if updated.Name != name || updated.Note != note {
			t.Errorf("got name=%q note=%q; want name=%q note=%q", updated.Name, updated.Note, name, note)
		}
	})

	// 2. Narrow role (owner -> member) -> 200
	t.Run("narrow role owner to member", func(t *testing.T) {
		role := "member"
		rec := putJSON(rawOwner, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"role": role,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("got code %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var updated models.Token
		db.First(&updated, tokTarget.ID)
		if updated.Role != "member" {
			t.Errorf("got role=%q; want member", updated.Role)
		}
	})

	// 3. Member calls update -> 403
	t.Run("member calling update is forbidden", func(t *testing.T) {
		rec := putJSON(rawMember, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"name": "hacked-name",
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("got code %d, want 403; body=%s", rec.Code, rec.Body.String())
		}
	})

	// 4. Update token in another org -> 404 (and doesn't leak existence)
	t.Run("update token in another org returns 404", func(t *testing.T) {
		tokOtherOrg := models.Token{
			OrgID:  999, // Org 999
			UserID: 999,
			Name:   "other-org-token",
			Hash:   models.HashToken("oct_otherorgtoken0000000000000001"),
			Prefix: "oct_othe",
			Role:   "admin",
		}
		db.Create(&tokOtherOrg)

		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokOtherOrg.ID), map[string]any{
			"name": "try-hack",
		})
		if rec.Code != http.StatusNotFound {
			t.Errorf("got code %d, want 404; body=%s", rec.Code, rec.Body.String())
		}
	})

	// 5. Admin tries to set role to owner -> 403 (callerHoldsRole blocks)
	t.Run("admin set role to owner forbidden", func(t *testing.T) {
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"role": "owner",
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("got code %d, want 403; body=%s", rec.Code, rec.Body.String())
		}
	})

	// 6. Role: "" -> 400 (validTokenRole rejects)
	t.Run("role empty string rejected", func(t *testing.T) {
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"role": "",
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("got code %d, want 400; body=%s", rec.Code, rec.Body.String())
		}
	})

	// 7. ExpiresInDays: -1 -> 400; 0 -> ExpiresAt nil; positive -> future
	t.Run("expiresInDays validation and updates", func(t *testing.T) {
		// Negative -> 400
		recNeg := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"expiresInDays": -1,
		})
		if recNeg.Code != http.StatusBadRequest {
			t.Errorf("negative expiresInDays: got code %d, want 400; body=%s", recNeg.Code, recNeg.Body.String())
		}

		// Positive -> set future ExpiresAt
		recPos := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"expiresInDays": 10,
		})
		if recPos.Code != http.StatusOK {
			t.Fatalf("positive expiresInDays: got code %d, want 200; body=%s", recPos.Code, recPos.Body.String())
		}
		var updated models.Token
		db.First(&updated, tokTarget.ID)
		if updated.ExpiresAt == nil || !updated.ExpiresAt.After(time.Now()) {
			t.Errorf("expected future ExpiresAt, got %v", updated.ExpiresAt)
		}

		// 0 -> clear ExpiresAt to nil
		recZero := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"expiresInDays": 0,
		})
		if recZero.Code != http.StatusOK {
			t.Fatalf("zero expiresInDays: got code %d, want 200; body=%s", recZero.Code, recZero.Body.String())
		}
		updated = models.Token{}
		db.First(&updated, tokTarget.ID)
		if updated.ExpiresAt != nil {
			t.Errorf("expected nil ExpiresAt when 0, got %v", updated.ExpiresAt)
		}
	})

	// 8. Response body contains no raw token / hash
	t.Run("response body conceals raw token and hash", func(t *testing.T) {
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"name": "check-security",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("got code %d, want 200", rec.Code)
		}
		var resp map[string]any
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if _, ok := resp["token"]; ok {
			t.Errorf("response leaked 'token' field: %v", resp["token"])
		}
		if _, ok := resp["hash"]; ok {
			t.Errorf("response leaked 'hash' field: %v", resp["hash"])
		}
		if _, ok := resp["Hash"]; ok {
			t.Errorf("response leaked 'Hash' field: %v", resp["Hash"])
		}
	})

	// 9. Original secret key still works after update (proving secret was not rotated)
	t.Run("original raw token remains valid after update", func(t *testing.T) {
		// Update target token to member role using owner auth
		recUpdate := putJSON(rawOwner, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"role": "member",
		})
		if recUpdate.Code != http.StatusOK {
			t.Fatalf("failed to update token role: %d, body=%s", recUpdate.Code, recUpdate.Body.String())
		}

		// Verify target token authenticates fine on ungated route
		req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		req.Header.Set("Authorization", "Bearer "+rawTarget)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("original raw token request failed with status %d (body=%s)", rec.Code, rec.Body.String())
		}

		// Verify role narrowing actually took effect: target token should be denied on admin route
		reqAdmin := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
		reqAdmin.Header.Set("Authorization", "Bearer "+rawTarget)
		recAdmin := httptest.NewRecorder()
		srv.ServeHTTP(recAdmin, reqAdmin)
		if recAdmin.Code != http.StatusForbidden {
			t.Errorf("narrowed token got status %d on admin route, want 403", recAdmin.Code)
		}
	})
}

func testIDStr(id uint) string {
	var buf [20]byte
	i := len(buf)
	for id >= 10 {
		i--
		buf[i] = byte('0' + id%10)
		id /= 10
	}
	i--
	buf[i] = byte('0' + id)
	return string(buf[i:])
}
