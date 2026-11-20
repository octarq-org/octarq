package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestTokenUpdate(t *testing.T) {
	srv, db := newTestHandler(t)

	seedMember(t, db, 10, "owner")
	rawOwner := "oct_owner00000000000000000000000001"
	seedToken(t, db, rawOwner, 10, "owner")

	seedMember(t, db, 11, "admin")
	rawAdmin := "oct_admin00000000000000000000000001"
	seedToken(t, db, rawAdmin, 11, "admin")

	seedMember(t, db, 12, "member")
	rawMember := "oct_member0000000000000000000000001"
	seedToken(t, db, rawMember, 12, "member")

	rawTarget := "oct_target0000000000000000000000001"
	seedToken(t, db, rawTarget, 10, "owner")
	var tokTarget models.Token
	if err := db.Where("prefix = ?", tokenPrefix(rawTarget)).First(&tokTarget).Error; err != nil {
		t.Fatalf("find target token: %v", err)
	}

	rawAdminTarget := "oct_admintarget000000000000000001"
	seedToken(t, db, rawAdminTarget, 11, "admin")
	var tokAdminTarget models.Token
	if err := db.Where("prefix = ?", tokenPrefix(rawAdminTarget)).First(&tokAdminTarget).Error; err != nil {
		t.Fatalf("find admin target token: %v", err)
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

	t.Run("admin update name and note", func(t *testing.T) {
		name := "updated-name"
		note := "updated-note"
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
			"name": name,
			"note": note,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("got code %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var updated models.Token
		db.First(&updated, tokAdminTarget.ID)
		if updated.Name != name || updated.Note != note {
			t.Errorf("got name=%q note=%q; want name=%q note=%q", updated.Name, updated.Note, name, note)
		}
	})

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

	t.Run("member cannot edit a token they do not own", func(t *testing.T) {
		rec := putJSON(rawMember, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"name": "hacked-name",
		})
		if rec.Code != http.StatusNotFound {
			t.Errorf("got code %d, want 404; body=%s", rec.Code, rec.Body.String())
		}
		var after models.Token
		db.First(&after, tokTarget.ID)
		if after.Name == "hacked-name" {
			t.Error("a member renamed a token belonging to someone else")
		}
	})

	t.Run("update token in another org returns 404", func(t *testing.T) {
		tokOtherOrg := models.Token{
			OrgID:  999,
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

	t.Run("admin set role to owner forbidden", func(t *testing.T) {
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
			"role": "owner",
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("got code %d, want 403; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("role empty string rejected", func(t *testing.T) {
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
			"role": "",
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("got code %d, want 400; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("expiresInDays validation and updates", func(t *testing.T) {
		recNeg := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
			"expiresInDays": -1,
		})
		if recNeg.Code != http.StatusBadRequest {
			t.Errorf("negative expiresInDays: got code %d, want 400; body=%s", recNeg.Code, recNeg.Body.String())
		}

		recPos := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
			"expiresInDays": 10,
		})
		if recPos.Code != http.StatusOK {
			t.Fatalf("positive expiresInDays: got code %d, want 200; body=%s", recPos.Code, recPos.Body.String())
		}
		var updated models.Token
		db.First(&updated, tokAdminTarget.ID)
		if updated.ExpiresAt == nil || !updated.ExpiresAt.After(time.Now()) {
			t.Errorf("expected future ExpiresAt, got %v", updated.ExpiresAt)
		}

		recZero := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
			"expiresInDays": 0,
		})
		if recZero.Code != http.StatusOK {
			t.Fatalf("zero expiresInDays: got code %d, want 200; body=%s", recZero.Code, recZero.Body.String())
		}
		updated = models.Token{}
		db.First(&updated, tokAdminTarget.ID)
		if updated.ExpiresAt != nil {
			t.Errorf("expected nil ExpiresAt when 0, got %v", updated.ExpiresAt)
		}
	})

	t.Run("response body conceals raw token and hash", func(t *testing.T) {
		rec := putJSON(rawAdmin, "/api/tokens/"+testIDStr(tokAdminTarget.ID), map[string]any{
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

	t.Run("original raw token remains valid after update", func(t *testing.T) {
		recUpdate := putJSON(rawOwner, "/api/tokens/"+testIDStr(tokTarget.ID), map[string]any{
			"role": "member",
		})
		if recUpdate.Code != http.StatusOK {
			t.Fatalf("failed to update token role: %d, body=%s", recUpdate.Code, recUpdate.Body.String())
		}

		req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
		req.Header.Set("Authorization", "Bearer "+rawTarget)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("original raw token request failed with status %d (body=%s)", rec.Code, rec.Body.String())
		}

		reqAdmin := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
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
