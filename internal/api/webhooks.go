package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
)

// webhookSecretPlaintext returns the usable signing secret for a stored webhook.
// Secrets are AES-GCM encrypted at rest; older rows may still hold plaintext, so
// a failed decrypt falls back to the raw value for backward compatibility.
func (h *Handler) webhookSecretPlaintext(stored string) string {
	if stored == "" {
		return ""
	}
	if b, err := h.cipher.Decrypt(stored); err == nil {
		return string(b)
	}
	return stored // legacy plaintext row
}

// encryptWebhookSecret seals a plaintext signing secret for storage.
func (h *Handler) encryptWebhookSecret(plaintext string) (string, error) {
	return h.cipher.Encrypt([]byte(plaintext))
}

// decryptedForResponse returns a copy of the hook with its secret decrypted, so
// the dashboard (behind auth) can display/copy the signing secret while the
// value stays encrypted at rest.
func (h *Handler) decryptedForResponse(hook models.Webhook) models.Webhook {
	hook.Secret = h.webhookSecretPlaintext(hook.Secret)
	return hook
}

type ListWebhooksInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListWebhooksInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListWebhooksOutput struct {
	Body []models.Webhook
}

func (h *Handler) listWebhooks(ctx context.Context, input *ListWebhooksInput) (*ListWebhooksOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var hooks []models.Webhook
	h.orgDB(r).Order("created_at DESC").Find(&hooks)
	for i := range hooks {
		hooks[i] = h.decryptedForResponse(hooks[i])
	}
	return &ListWebhooksOutput{Body: hooks}, nil
}

type CreateWebhookInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Secret  string `json:"secret,omitempty"` // optional, will auto-generate if empty
		Events  string `json:"events,omitempty"` // comma-separated, default "*"
		Enabled *bool  `json:"enabled,omitempty"`
	}
}

func (i *CreateWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateWebhookOutput struct {
	Body models.Webhook
}

func (h *Handler) createWebhook(ctx context.Context, input *CreateWebhookInput) (*CreateWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(input.Body.Name)
	url := strings.TrimSpace(input.Body.URL)
	if name == "" || url == "" {
		return nil, huma.Error400BadRequest("name and url are required")
	}

	secret := strings.TrimSpace(input.Body.Secret)
	if secret == "" {
		// Generate random 16-byte hex secret (32 chars)
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
	}

	events := strings.TrimSpace(input.Body.Events)
	if events == "" {
		events = "*"
	}

	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}

	encSecret, err := h.encryptWebhookSecret(secret)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to secure secret")
	}

	hook := models.Webhook{
		OrgID:   h.orgID(r),
		Name:    name,
		URL:     url,
		Secret:  encSecret,
		Events:  events,
		Enabled: enabled,
	}

	if err := h.db.Create(&hook).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save")
	}

	h.audit(r, "webhook.create", "webhook", hook.ID, map[string]any{"name": hook.Name, "url": hook.URL})
	hook.Secret = secret // return the plaintext secret to the creator
	return &CreateWebhookOutput{Body: hook}, nil
}

type UpdateWebhookInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Name    string `json:"name,omitempty"`
		URL     string `json:"url,omitempty"`
		Secret  string `json:"secret,omitempty"`
		Events  string `json:"events,omitempty"`
		Enabled *bool  `json:"enabled,omitempty"`
	}
}

func (i *UpdateWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateWebhookOutput struct {
	Body models.Webhook
}

func (h *Handler) updateWebhook(ctx context.Context, input *UpdateWebhookInput) (*UpdateWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var hook models.Webhook
	if h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&hook).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}

	name := strings.TrimSpace(input.Body.Name)
	url := strings.TrimSpace(input.Body.URL)
	if name == "" || url == "" {
		return nil, huma.Error400BadRequest("name and url are required")
	}

	hook.Name = name
	hook.URL = url
	if input.Body.Secret != "" {
		enc, err := h.encryptWebhookSecret(strings.TrimSpace(input.Body.Secret))
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to secure secret")
		}
		hook.Secret = enc
	}
	if input.Body.Events != "" {
		hook.Events = strings.TrimSpace(input.Body.Events)
	}
	if input.Body.Enabled != nil {
		hook.Enabled = *input.Body.Enabled
	}

	h.db.Save(&hook)
	meta := map[string]any{
		"name":    hook.Name,
		"url":     hook.URL,
		"events":  hook.Events,
		"enabled": hook.Enabled,
	}
	if input.Body.Secret != "" {
		meta["secret"] = "[REDACTED]"
	}
	h.audit(r, "webhook.update", "webhook", hook.ID, meta)
	return &UpdateWebhookOutput{Body: h.decryptedForResponse(hook)}, nil
}

type DeleteWebhookInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteWebhookOutput struct {
	Body map[string]bool
}

func (h *Handler) deleteWebhook(ctx context.Context, input *DeleteWebhookInput) (*DeleteWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).Delete(&models.Webhook{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}

	h.audit(r, "webhook.delete", "webhook", input.ID, nil)
	return &DeleteWebhookOutput{Body: map[string]bool{"ok": true}}, nil
}

type ListWebhookEventsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListWebhookEventsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListWebhookEventsOutput struct {
	Body []eventbus.EventGroup
}

// listWebhookEvents returns the registered webhook event definitions, grouped
// in registration order, so the dashboard's webhook editor only offers events
// this build can actually fire.
func (h *Handler) listWebhookEvents(ctx context.Context, input *ListWebhookEventsInput) (*ListWebhookEventsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if _, ok := h.auth.AuthenticateRequest(r); !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	return &ListWebhookEventsOutput{Body: eventbus.EventGroups()}, nil
}
