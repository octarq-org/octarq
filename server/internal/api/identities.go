package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
)

type ListIdentitiesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListIdentitiesInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

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

func (h *Handler) listIdentities(ctx context.Context, input *ListIdentitiesInput) (*ListIdentitiesOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
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

func (h *Handler) unlinkIdentity(ctx context.Context, input *UnlinkIdentityInput) (*UnlinkIdentityOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	uid := h.auth.UserID(r)

	var id models.UserIdentity
	if err := h.db.Where("id = ? AND user_id = ?", input.ID, uid).First(&id).Error; err != nil {
		return nil, huma.Error404NotFound("Identity not found; verify the identity identifier and try again")
	}

	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not load account details; retry the request")
	}
	if user.PasswordHash == "" {
		var n int64
		h.db.Model(&models.UserIdentity{}).Where("user_id = ?", uid).Count(&n)
		if n <= 1 {
			return nil, huma.Error409Conflict("Set an account password before removing your only sign-in method")
		}
	}

	if err := h.db.Delete(&models.UserIdentity{}, id.ID).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not unlink identity; retry the request")
	}
	h.audit(r, "identity.unlink", "user_identity", id.ID, map[string]any{
		"provider": id.Provider, "issuer": id.Issuer,
	})

	out := &UnlinkIdentityOutput{}
	out.Body.OK = true
	return out, nil
}
