package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestTokenByRequestAcceptsTheGeminiKeyHeader(t *testing.T) {
	db := identityDB(t)
	m := identityManager(t, db)
	raw := "oct_gemini_cli_token_" + "0"
	for len(raw) < 32 {
		raw += "0"
	}
	if err := db.Create(&models.Token{
		Name: "gemini", Hash: models.HashToken(raw), Prefix: raw[:8], UserID: 1, OrgID: 1,
	}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("X-Goog-Api-Key", raw)
	if tok, ok := m.TokenByRequest(req); !ok || tok.OrgID != 1 {
		t.Fatalf("the Gemini key header was not admitted: ok=%v tok=%+v", ok, tok)
	}

	other := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	other.Header.Set("X-Goog-Api-Key", "AIzaSy-not-an-octarq-token")
	if _, ok := m.TokenByRequest(other); ok {
		t.Fatal("a foreign key was admitted as a workspace token")
	}
}
