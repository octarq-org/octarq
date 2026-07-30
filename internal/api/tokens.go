package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/authz"
	"github.com/octarq-org/octarq/internal/models"
)

// tokenAlphabet length-independent random token body. We use URL-safe base64
// (without padding) so the token is copy/paste friendly.
func newRawToken() string {
	b := make([]byte, 24) // 24 bytes -> 32 url-safe chars
	rand.Read(b)
	return "oct_" + base64.RawURLEncoding.EncodeToString(b)
}

// tokenPrefix is the short, non-secret identifier shown in the list.
func tokenPrefix(raw string) string {
	if len(raw) <= 8 {
		return raw
	}
	return raw[:8]
}

type ListTokensInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListTokensInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListTokensOutput struct {
	Body []models.Token
}

func (h *Handler) listTokens(ctx context.Context, input *ListTokensInput) (*ListTokensOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	var toks []models.Token
	h.orgDB(r).Order("created_at DESC").Find(&toks)
	return &ListTokensOutput{Body: toks}, nil
}

type CreateTokenInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name          string `json:"name"`
		Note          string `json:"note,omitempty"`
		ExpiresInDays int    `json:"expiresInDays,omitempty"`
		// Role narrows the token below its holder; it can never widen it. Omitted
		// means "member" — the least privilege that still works — so a caller who
		// does not think about it gets a narrow token rather than a copy of their
		// own account.
		Role string `json:"role,omitempty"`
	}
}

func (i *CreateTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateTokenOutput struct {
	Body map[string]any
}

func (h *Handler) createToken(ctx context.Context, input *CreateTokenInput) (*CreateTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(input.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("name is required")
	}
	if input.Body.ExpiresInDays < 0 {
		return nil, huma.Error400BadRequest("expiresInDays must be zero (never) or positive")
	}
	var expiresAt *time.Time
	if input.Body.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, input.Body.ExpiresInDays)
		expiresAt = &t
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	role := authz.Role(strings.TrimSpace(input.Body.Role))
	if role == "" {
		role = authz.RoleMember
	}
	if !validTokenRole(role) {
		return nil, huma.Error400BadRequest("role must be one of member, admin, owner")
	}
	// A token must not out-rank the caller minting it, or an admin could mint an
	// owner token and use it to walk straight past the gate that just checked them.
	if !h.callerHoldsRole(r, role) {
		return nil, huma.Error403Forbidden("forbidden: cannot mint a token above your own role")
	}
	// The token acts as this person from here on. Minting it while
	// unattributable would be the one way to create authority that outlives the
	// account it came from.
	uid := h.auth.UserID(r)
	if uid == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	raw := newRawToken()
	tok := models.Token{
		OrgID:     orgID,
		UserID:    uid,
		Name:      name,
		Hash:      models.HashToken(raw),
		Prefix:    tokenPrefix(raw),
		Note:      input.Body.Note,
		Role:      string(role),
		ExpiresAt: expiresAt,
	}
	if err := h.db.Create(&tok).Error; err != nil {
		return nil, huma.Error500InternalServerError("create token")
	}
	h.audit(r, "token.create", "token", tok.ID, map[string]any{"name": tok.Name, "prefix": tok.Prefix, "role": tok.Role})
	// The raw token is returned ONLY here; it is never stored or shown again.
	return &CreateTokenOutput{
		Body: map[string]any{
			"id":        tok.ID,
			"name":      tok.Name,
			"note":      tok.Note,
			"prefix":    tok.Prefix,
			"role":      tok.Role,
			"expiresAt": tok.ExpiresAt,
			"createdAt": tok.CreatedAt,
			"token":     raw,
		},
	}, nil
}

type DeleteTokenInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteTokenOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) deleteToken(ctx context.Context, input *DeleteTokenInput) (*DeleteTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).Delete(&models.Token{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	h.audit(r, "token.delete", "token", input.ID, nil)
	out := &DeleteTokenOutput{}
	out.Body.OK = true
	return out, nil
}

// validTokenRole reports whether role is one a token may be minted with.
// It deliberately rejects "" — that value means "legacy unrestricted" and is
// reachable only by rows that predate scoping, never by minting a new token.
func validTokenRole(role authz.Role) bool {
	switch role {
	case authz.RoleMember, authz.RoleAdmin, authz.RoleOwner:
		return true
	}
	return false
}
