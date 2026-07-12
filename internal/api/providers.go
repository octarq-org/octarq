package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

type providerAccountDTO struct {
	Name   string         `json:"name,omitempty"`
	Type   string         `json:"type,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type ListProviderAccountsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListProviderAccountsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListProviderAccountsOutput struct {
	Body []models.ProviderAccount
}

func (h *Handler) listProviderAccounts(ctx context.Context, input *ListProviderAccountsInput) (*ListProviderAccountsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var accounts []models.ProviderAccount
	h.orgDB(r).Order("created_at DESC").Find(&accounts)
	for i := range accounts {
		accounts[i].HasCredentials = accounts[i].Config != ""
	}
	return &ListProviderAccountsOutput{Body: accounts}, nil
}

type CreateProviderAccountInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body providerAccountDTO
}

func (i *CreateProviderAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateProviderAccountOutput struct {
	Body models.ProviderAccount
}

func (h *Handler) createProviderAccount(ctx context.Context, input *CreateProviderAccountInput) (*CreateProviderAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(input.Body.Name)
	typ := strings.TrimSpace(input.Body.Type)
	if name == "" || typ == "" {
		return nil, huma.Error400BadRequest("name and type are required")
	}
	enc, err := h.encryptConfig(input.Body.Config)
	if err != nil {
		return nil, huma.Error500InternalServerError("encrypt config")
	}
	acc := models.ProviderAccount{
		OrgID:  h.orgID(r),
		Name:   name,
		Type:   typ,
		Config: enc,
	}
	if err := h.db.Create(&acc).Error; err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	h.audit(r, "provider.create", "provider", acc.ID, map[string]any{"name": acc.Name, "type": acc.Type})
	acc.HasCredentials = acc.Config != ""
	return &CreateProviderAccountOutput{Body: acc}, nil
}

type UpdateProviderAccountInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body providerAccountDTO
}

func (i *UpdateProviderAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateProviderAccountOutput struct {
	Body models.ProviderAccount
}

func (h *Handler) updateProviderAccount(ctx context.Context, input *UpdateProviderAccountInput) (*UpdateProviderAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var acc models.ProviderAccount
	if h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&acc).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if strings.TrimSpace(input.Body.Name) != "" {
		acc.Name = strings.TrimSpace(input.Body.Name)
	}
	if len(input.Body.Config) > 0 {
		enc, err := h.encryptConfig(input.Body.Config)
		if err != nil {
			return nil, huma.Error500InternalServerError("encrypt config")
		}
		acc.Config = enc
	}
	h.db.Save(&acc)
	meta := make(map[string]any)
	if strings.TrimSpace(input.Body.Name) != "" {
		meta["name"] = acc.Name
	}
	if len(input.Body.Config) > 0 {
		redactedConfig := make(map[string]string)
		for k := range input.Body.Config {
			redactedConfig[k] = "[REDACTED]"
		}
		meta["config"] = redactedConfig
	}
	h.audit(r, "provider.update", "provider", acc.ID, meta)
	acc.HasCredentials = acc.Config != ""
	return &UpdateProviderAccountOutput{Body: acc}, nil
}

type DeleteProviderAccountInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteProviderAccountInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteProviderAccountOutput struct {
	Body map[string]bool
}

func (h *Handler) deleteProviderAccount(ctx context.Context, input *DeleteProviderAccountInput) (*DeleteProviderAccountOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Check if any domain is using this account
	var count int64
	h.db.Model(&models.Domain{}).Where("provider_account_id = ?", input.ID).Count(&count)
	if count > 0 {
		return nil, huma.NewError(http.StatusConflict, "cannot delete provider account because it is used by one or more domains")
	}

	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).Delete(&models.ProviderAccount{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	h.audit(r, "provider.delete", "provider", input.ID, nil)
	return &DeleteProviderAccountOutput{Body: map[string]bool{"ok": true}}, nil
}

