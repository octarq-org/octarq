package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// WebhookView is returned on list and update endpoints. Secret is never returned,
// but SecretSet indicates whether a signing secret has been configured.
type WebhookView struct {
	models.Webhook
	SecretSet bool `json:"secretSet"`
}

// WebhookCreateView is returned once on webhook creation, carrying the plaintext
// signing secret to the creator.
type WebhookCreateView struct {
	models.Webhook
	Secret    string `json:"secret"`
	SecretSet bool   `json:"secretSet"`
}

type ListWebhooksInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListWebhooksInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListWebhooksOutput struct {
	Body []WebhookView
}

func (h *Handler) listWebhooks(ctx context.Context, input *ListWebhooksInput) (*ListWebhooksOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}
	var hooks []models.Webhook
	h.orgDB(r).Order("created_at DESC").Find(&hooks)
	views := make([]WebhookView, len(hooks))
	for i, hook := range hooks {
		views[i] = WebhookView{
			Webhook:   hook,
			SecretSet: hook.VaultItemID > 0,
		}
	}
	return &ListWebhooksOutput{Body: views}, nil
}

type CreateWebhookInputBody struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Secret  string `json:"secret,omitempty"` // optional, will auto-generate if empty
	Events  string `json:"events,omitempty"` // comma-separated, default "*"
	Enabled *bool  `json:"enabled,omitempty"`
}

type CreateWebhookInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body CreateWebhookInputBody
}

func (i *CreateWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateWebhookOutput struct {
	Body WebhookCreateView
}

func (h *Handler) createWebhook(ctx context.Context, input *CreateWebhookInput) (*CreateWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	name := strings.TrimSpace(input.Body.Name)
	url := strings.TrimSpace(input.Body.URL)
	if name == "" || url == "" {
		return nil, huma.Error400BadRequest("Webhook name and target URL are required; provide both fields")
	}

	secret := strings.TrimSpace(input.Body.Secret)
	if secret == "" {
		// Generate random 16-byte hex secret (32 chars)
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, huma.Error500InternalServerError("Could not generate webhook secret; retry the request")
		}
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

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}

	vs := h.vault()
	if vs == nil {
		return nil, huma.Error500InternalServerError("Vault service is unavailable; retry the request")
	}
	meta, err := vs.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:          orgID,
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: "default",
		ItemType:       "webhook",
		Name:           "Webhook: " + name,
		Payload: map[string]any{
			"signingSecret": secret,
			"targetUrl":     url,
		},
		ActorUserID: h.auth.UserID(r),
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not secure webhook secret; retry the request")
	}

	hook := models.Webhook{
		OrgID:       orgID,
		Name:        name,
		URL:         url,
		VaultItemID: meta.ID,
		Events:      events,
		Enabled:     enabled,
	}

	if err := h.db.Create(&hook).Error; err != nil {
		_ = vs.DeleteSecret(ctx, orgID, meta.ID)
		return nil, huma.Error500InternalServerError("Could not save webhook; retry the request")
	}

	h.audit(r, "webhook.create", "webhook", hook.ID, map[string]any{"name": hook.Name, "url": hook.URL})
	return &CreateWebhookOutput{
		Body: WebhookCreateView{
			Webhook:   hook,
			Secret:    secret,
			SecretSet: true,
		},
	}, nil
}

type UpdateWebhookInputBody struct {
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	Secret  string `json:"secret,omitempty"`
	Events  string `json:"events,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type UpdateWebhookInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body UpdateWebhookInputBody
}

func (i *UpdateWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateWebhookOutput struct {
	Body WebhookView
}

func (h *Handler) updateWebhook(ctx context.Context, input *UpdateWebhookInput) (*UpdateWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}

	var hook models.Webhook
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&hook).Error != nil {
		return nil, huma.Error404NotFound("Webhook not found; verify the webhook identifier and try again")
	}

	name := strings.TrimSpace(input.Body.Name)
	url := strings.TrimSpace(input.Body.URL)
	if name == "" || url == "" {
		return nil, huma.Error400BadRequest("Webhook name and target URL are required; provide both fields")
	}

	hook.Name = name
	hook.URL = url
	if sec := strings.TrimSpace(input.Body.Secret); sec != "" {
		vs := h.vault()
		if vs == nil {
			return nil, huma.Error500InternalServerError("Vault service is unavailable; retry the request")
		}
		payload := map[string]any{
			"signingSecret": sec,
			"targetUrl":     url,
		}
		if hook.VaultItemID > 0 {
			_, err := vs.UpdateSecret(ctx, orgID, hook.VaultItemID, nil, nil, payload, nil, "webhook secret rotated", h.auth.UserID(r))
			if errors.Is(err, plugin.ErrSecretNotFound) {
				meta, putErr := vs.PutSecret(ctx, plugin.PutSecretRequest{
					OrgID:          orgID,
					Scope:          plugin.VaultScopeOrg,
					CollectionSlug: "default",
					ItemType:       "webhook",
					Name:           "Webhook: " + name,
					Payload:        payload,
					ActorUserID:    h.auth.UserID(r),
				})
				if putErr != nil {
					return nil, huma.Error500InternalServerError("Could not secure webhook secret; retry the request")
				}
				hook.VaultItemID = meta.ID
			} else if err != nil {
				return nil, huma.Error500InternalServerError("Could not secure webhook secret; retry the request")
			}
		} else {
			meta, err := vs.PutSecret(ctx, plugin.PutSecretRequest{
				OrgID:          orgID,
				Scope:          plugin.VaultScopeOrg,
				CollectionSlug: "default",
				ItemType:       "webhook",
				Name:           "Webhook: " + name,
				Payload:        payload,
				ActorUserID:    h.auth.UserID(r),
			})
			if err != nil {
				return nil, huma.Error500InternalServerError("Could not secure webhook secret; retry the request")
			}
			hook.VaultItemID = meta.ID
		}
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
	return &UpdateWebhookOutput{
		Body: WebhookView{
			Webhook:   hook,
			SecretSet: hook.VaultItemID > 0,
		},
	}, nil
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
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}

	var hook models.Webhook
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&hook).Error != nil {
		return nil, huma.Error404NotFound("Webhook not found; verify the webhook identifier and try again")
	}
	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).Delete(&models.Webhook{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("Webhook not found; verify the webhook identifier and try again")
	}
	if hook.VaultItemID > 0 {
		if vs := h.vault(); vs != nil {
			_ = vs.DeleteSecret(ctx, orgID, hook.VaultItemID)
		}
	}

	h.audit(r, "webhook.delete", "webhook", input.ID, nil)
	return &DeleteWebhookOutput{Body: map[string]bool{"ok": true}}, nil
}

type TestWebhookInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *TestWebhookInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type TestWebhookOutput struct {
	Body map[string]bool
}

// testWebhook fires one synthetic webhook.test event at the stored endpoint.
// Single attempt on purpose (not deliverWithRetry): a test that needs five
// retries is a broken endpoint, and the operator wants the first failure's
// reason, not the loop's dead-letter summary.
func (h *Handler) testWebhook(ctx context.Context, input *TestWebhookInput) (*TestWebhookOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, err
	}

	var hook models.Webhook
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&hook).Error != nil {
		return nil, huma.Error404NotFound("Webhook not found; verify the webhook identifier and try again")
	}
	if !hook.Enabled {
		return nil, huma.Error400BadRequest("Webhook is disabled; enable it before testing")
	}

	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := eventbus.TestSend(testCtx, orgID, hook.ID, hook.URL, hook.VaultItemID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	h.audit(r, "webhook.test", "webhook", hook.ID, map[string]any{"url": hook.URL})
	return &TestWebhookOutput{Body: map[string]bool{"ok": true}}, nil
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
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if _, ok := h.auth.AuthenticateRequest(r); !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	return &ListWebhookEventsOutput{Body: eventbus.EventGroups()}, nil
}
