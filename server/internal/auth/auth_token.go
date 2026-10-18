package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

// TokenID extracts the API bearer token ID from the request context.
func (m *Manager) TokenID(r *http.Request) uint {
	return TokenIDFromContext(r.Context())
}

// WithTokenIdentity stamps a validated bearer token onto ctx and kicks off the
// best-effort LastUsedAt touch. Every bearer path funnels through here so token
// context is consistently populated across all paths.
func (m *Manager) WithTokenIdentity(ctx context.Context, tok models.Token) context.Context {
	ctx = WithTokenID(ctx, tok.ID)
	ctx = WithTokenRole(ctx, tok.Role)
	ctx = WithTokenAgent(ctx, tok.IsAgent, tok.AllowedTools)
	m.touchToken(tok.ID)
	return ctx
}

// touchToken records LastUsedAt asynchronously; never blocks a request.
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

// TokenByRequest resolves and validates the request's "oct_" bearer token.
// It does NOT touch LastUsedAt — callers that admit the token use
// withTokenIdentity, which does.
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

// bearerToken extracts a "oct_" token from the Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const pfx = "Bearer "
	if !strings.HasPrefix(h, pfx) {
		return ""
	}
	return strings.TrimSpace(h[len(pfx):])
}

// tokenAuthed reports whether the request carries a valid API bearer token.
// On success it best-effort records LastUsedAt asynchronously.
func (m *Manager) tokenAuthed(r *http.Request) bool {
	tok, ok := m.TokenByRequest(r)
	if !ok {
		return false
	}
	m.touchToken(tok.ID)
	return true
}

// APIAuthed reports whether the request is authenticated by either a valid
// session cookie or a valid API bearer token.
func (m *Manager) APIAuthed(r *http.Request) bool {
	return m.Authed(r) || m.tokenAuthed(r)
}

// RevokeUserOrgTokens deletes every API bearer token bound to the given user
// in the given org. A token's authority is normally read live from its
// holder's membership, but that live read is only one layer of defence: any
// authorization seam that answers before the live read (a plugin resolver, for
// one) would let a removed member's token keep acting. Deleting the rows makes
// the removal explicit rather than implied. Returns the number of tokens
// revoked.
//
// Token.OrgID maps to the owner_id column, so the query is scoped by that name;
// like sessions, revocation is per (user, org) and leaves the user's tokens in
// their other workspaces alone.
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
