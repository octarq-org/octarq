package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

func (m *Manager) TokenID(r *http.Request) uint {
	return TokenIDFromContext(r.Context())
}

func (m *Manager) WithTokenIdentity(ctx context.Context, tok models.Token) context.Context {
	ctx = WithTokenID(ctx, tok.ID)
	ctx = WithTokenRole(ctx, tok.Role)
	ctx = WithTokenAgent(ctx, tok.IsAgent, tok.AllowedTools)
	m.touchToken(tok.ID)
	return ctx
}

func (m *Manager) touchToken(id uint) {
	db := m.db
	if db == nil {
		return
	}
	go func() {
		now := time.Now()
		db.Model(&models.Token{}).Where("id = ?", id).Update("last_used_at", &now)
	}()
}

func (m *Manager) TokenByRequest(r *http.Request) (models.Token, bool) {
	var tok models.Token
	if m.db == nil {
		return tok, false
	}
	raw := bearerToken(r)
	if !strings.HasPrefix(raw, "oct_") {
		return tok, false
	}
	if m.db.Where("hash = ?", models.HashToken(raw)).First(&tok).Error != nil {
		return tok, false
	}
	if tok.Expired() {
		return models.Token{}, false
	}
	return tok, true
}

func bearerToken(r *http.Request) string {
	const pfx = "Bearer "
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, pfx) {
		return strings.TrimSpace(h[len(pfx):])
	}
	return strings.TrimSpace(r.Header.Get("X-Goog-Api-Key"))
}

func (m *Manager) tokenAuthed(r *http.Request) bool {
	tok, ok := m.TokenByRequest(r)
	if !ok {
		return false
	}
	m.touchToken(tok.ID)
	return true
}

func (m *Manager) APIAuthed(r *http.Request) bool {
	return m.Authed(r) || m.tokenAuthed(r)
}

func (m *Manager) RevokeUserOrgTokens(userID, orgID uint) int {
	if m.db == nil || userID == 0 || orgID == 0 {
		return 0
	}
	res := m.db.Where("user_id = ? AND owner_id = ?", userID, orgID).Delete(&models.Token{})
	if res.Error != nil {
		return 0
	}
	return int(res.RowsAffected)
}
