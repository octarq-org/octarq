package api

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
)

type ListNotificationChannelsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListNotificationChannelsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListNotificationChannelsOutput struct {
	Body []models.NotificationChannel
}

func (h *Handler) listNotificationChannels(ctx context.Context, input *ListNotificationChannelsInput) (*ListNotificationChannelsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var channels []models.NotificationChannel
	h.orgDB(r).Order("created_at DESC").Find(&channels)
	return &ListNotificationChannelsOutput{Body: channels}, nil
}

type CreateNotificationChannelInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Config  string `json:"config"`
		Enabled *bool  `json:"enabled,omitempty"`
	}
}

func (i *CreateNotificationChannelInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateNotificationChannelOutput struct {
	Body models.NotificationChannel
}

func (h *Handler) createNotificationChannel(ctx context.Context, input *CreateNotificationChannelInput) (*CreateNotificationChannelOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(input.Body.Name)
	typ := strings.ToLower(strings.TrimSpace(input.Body.Type))
	if name == "" || typ == "" {
		return nil, huma.Error400BadRequest("name and type are required")
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	d := models.NotificationChannel{
		OrgID:   h.orgID(r),
		Name:    name,
		Type:    typ,
		Config:  input.Body.Config,
		Enabled: enabled,
	}
	if err := h.db.Create(&d).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to create")
	}
	h.audit(r, "notification.create", "notification_channel", d.ID, map[string]any{"name": d.Name, "type": d.Type})
	return &CreateNotificationChannelOutput{Body: d}, nil
}

type UpdateNotificationChannelInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Name    *string `json:"name,omitempty"`
		Type    *string `json:"type,omitempty"`
		Config  *string `json:"config,omitempty"`
		Enabled *bool   `json:"enabled,omitempty"`
	}
}

func (i *UpdateNotificationChannelInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateNotificationChannelOutput struct {
	Body models.NotificationChannel
}

func (h *Handler) updateNotificationChannel(ctx context.Context, input *UpdateNotificationChannelInput) (*UpdateNotificationChannelOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&ch).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	d := input.Body
	if d.Name != nil {
		ch.Name = *d.Name
	}
	if d.Type != nil {
		ch.Type = strings.ToLower(strings.TrimSpace(*d.Type))
	}
	if d.Config != nil {
		ch.Config = *d.Config
	}
	if d.Enabled != nil {
		ch.Enabled = *d.Enabled
	}
	h.db.Save(&ch)
	meta := make(map[string]any)
	if d.Name != nil {
		meta["name"] = *d.Name
	}
	if d.Type != nil {
		meta["type"] = *d.Type
	}
	if d.Config != nil {
		meta["config"] = "[REDACTED]"
	}
	if d.Enabled != nil {
		meta["enabled"] = *d.Enabled
	}
	h.audit(r, "notification.update", "notification_channel", ch.ID, meta)
	return &UpdateNotificationChannelOutput{Body: ch}, nil
}

type DeleteNotificationChannelInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteNotificationChannelInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteNotificationChannelOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) deleteNotificationChannel(ctx context.Context, input *DeleteNotificationChannelInput) (*DeleteNotificationChannelOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).Delete(&models.NotificationChannel{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	h.audit(r, "notification.delete", "notification_channel", input.ID, nil)
	out := &DeleteNotificationChannelOutput{}
	out.Body.OK = true
	return out, nil
}

type TestNotificationChannelInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *TestNotificationChannelInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type TestNotificationChannelOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) testNotificationChannel(ctx context.Context, input *TestNotificationChannelInput) (*TestNotificationChannelOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&ch).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	ctxTimeout, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := notify.Send(ctxTimeout, ch.Type, ch.Config, "🔔 Test notification from octarq!"); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	out := &TestNotificationChannelOutput{}
	out.Body.OK = true
	return out, nil
}
