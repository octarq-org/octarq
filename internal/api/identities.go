package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

// Linked external identities (SSO / OIDC), from the account owner's side.
//
// The binding itself is created by whichever identity plugin ran the handshake,
// through plugin.Context.BindIdentity — it needs a verified assertion, which
// only that plugin has. What lives here is the part the account owner needs
// without any plugin: seeing what can sign in as them, and taking it away.
// Both are scoped to the caller's own user; there is no admin view, because
// nobody else's list is anybody's business.

type ListIdentitiesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListIdentitiesInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

// IdentityRow is one linked identity as shown in account settings. The subject
// is omitted: it is an opaque provider ID that means nothing to the person
// reading the page, and it is half the key an attacker would need to forge.
type IdentityRow struct {
	ID        uint      `json:"id"`
	Provider  string    `json:"provider"`
	Issuer    string    `json:"issuer"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type ListIdentitiesOutput struct {
	Body []IdentityRow
}

// listIdentities returns the external identities bound to the caller.
// GET /api/account/identities
func (h *Handler) listIdentities(ctx context.Context, input *ListIdentitiesInput) (*ListIdentitiesOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	uid := h.auth.UserID(r)

	var ids []models.UserIdentity
	h.db.Where("user_id = ?", uid).Order("id").Find(&ids)
	rows := make([]IdentityRow, len(ids))
	for i, id := range ids {
		rows[i] = IdentityRow{ID: id.ID, Provider: id.Provider, Issuer: id.Issuer, Email: id.Email, CreatedAt: id.CreatedAt}
	}
	return &ListIdentitiesOutput{Body: rows}, nil
}

type UnlinkIdentityInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *UnlinkIdentityInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type UnlinkIdentityOutputBody struct {
	OK bool `json:"ok"`
}

type UnlinkIdentityOutput struct {
	Body UnlinkIdentityOutputBody
}

// unlinkIdentity removes one of the caller's own linked identities.
// DELETE /api/account/identities/{id}
//
// The delete is scoped by user_id as well as by id, so the path parameter can
// only ever address a row the caller owns — an unowned id is "not found", not
// somebody else's unbinding.
func (h *Handler) unlinkIdentity(ctx context.Context, input *UnlinkIdentityInput) (*UnlinkIdentityOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	uid := h.auth.UserID(r)

	var id models.UserIdentity
	if err := h.db.Where("id = ? AND user_id = ?", input.ID, uid).First(&id).Error; err != nil {
		return nil, huma.Error404NotFound("identity not found")
	}

	// Removing the last way in is not a setting, it's a lockout. A user
	// provisioned through SSO has no password, so their identities are the whole
	// of their credentials.
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error500InternalServerError("could not load account")
	}
	if user.PasswordHash == "" {
		var n int64
		h.db.Model(&models.UserIdentity{}).Where("user_id = ?", uid).Count(&n)
		if n <= 1 {
			return nil, huma.Error409Conflict("set a password before removing your only sign-in method")
		}
	}

	if err := h.db.Delete(&models.UserIdentity{}, id.ID).Error; err != nil {
		return nil, huma.Error500InternalServerError("could not unlink identity")
	}
	h.audit(r, "identity.unlink", "user_identity", id.ID, map[string]any{
		"provider": id.Provider, "issuer": id.Issuer,
	})

	out := &UnlinkIdentityOutput{}
	out.Body.OK = true
	return out, nil
}
