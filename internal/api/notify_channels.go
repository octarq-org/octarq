package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/plugin"
)

type NotificationChannelType struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type ListNotificationChannelTypesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListNotificationChannelTypesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListNotificationChannelTypesOutput struct {
	Body []NotificationChannelType
}

func (h *Handler) listNotificationChannelTypes(ctx context.Context, input *ListNotificationChannelTypesInput) (*ListNotificationChannelTypesOutput, error) {
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
	allDescs := notify.Descriptors()
	var result []NotificationChannelType

	for _, d := range allDescs {
		if d.PluginName != "" {
			p := h.findPlugin(d.PluginName)
			if p == nil || !h.pluginActive(orgID, p) {
				continue
			}
		}
		result = append(result, NotificationChannelType{
			Type:        d.Type,
			Title:       d.Title,
			Description: d.Description,
			Icon:        d.Icon,
		})
	}
	return &ListNotificationChannelTypesOutput{Body: result}, nil
}

func (h *Handler) findPlugin(name string) plugin.Plugin {
	for _, p := range h.plugins {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

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
	for i := range channels {
		channels[i].Config = redactConfigSecrets(channels[i].Config)
	}
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
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	d := models.NotificationChannel{
		OrgID:   orgID,
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
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&ch).Error != nil {
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
		newCfgStr := *d.Config
		if ch.Config != "" && strings.Contains(newCfgStr, "[REDACTED]") {
			ch.Config = mergeConfigPreservingSecrets(ch.Config, newCfgStr)
		} else {
			ch.Config = newCfgStr
		}
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
	out := ch
	out.Config = redactConfigSecrets(out.Config)
	return &UpdateNotificationChannelOutput{Body: out}, nil
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
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).Delete(&models.NotificationChannel{}); res.RowsAffected == 0 {
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
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&ch).Error != nil {
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

func redactConfigSecrets(cfgJSON string) string {
	if strings.TrimSpace(cfgJSON) == "" {
		return "{}"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(cfgJSON), &m); err != nil {
		return cfgJSON
	}
	redactMap(m)
	b, err := json.Marshal(m)
	if err != nil {
		return cfgJSON
	}
	return string(b)
}

func redactMap(m map[string]any) {
	for k, v := range m {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") {
			if s, ok := v.(string); ok && s != "" {
				m[k] = "[REDACTED]"
			}
		} else if child, ok := v.(map[string]any); ok {
			redactMap(child)
		}
	}
}

func mergeConfigPreservingSecrets(oldJSON, newJSON string) string {
	var oldMap, newMap map[string]any
	if json.Unmarshal([]byte(oldJSON), &oldMap) != nil {
		return newJSON
	}
	if json.Unmarshal([]byte(newJSON), &newMap) != nil {
		return newJSON
	}
	mergeMapsPreservingSecrets(oldMap, newMap)
	b, err := json.Marshal(newMap)
	if err != nil {
		return newJSON
	}
	return string(b)
}

func mergeMapsPreservingSecrets(oldMap, newMap map[string]any) {
	for k, v := range newMap {
		if s, ok := v.(string); ok && s == "[REDACTED]" {
			if oldVal, exists := oldMap[k]; exists {
				if oldStr, isStr := oldVal.(string); isStr && oldStr != "" {
					newMap[k] = oldStr
				}
			}
		} else if newChild, ok := v.(map[string]any); ok {
			if oldChild, exists := oldMap[k].(map[string]any); exists {
				mergeMapsPreservingSecrets(oldChild, newChild)
			}
		}
	}
}
