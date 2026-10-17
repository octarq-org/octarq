package api

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/plugin"
)

type NotificationChannelType struct {
	Type         string          `json:"type"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Icon         string          `json:"icon"`
	ConfigSchema json.RawMessage `json:"configSchema,omitempty"`
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
	spiChannels := notification.DefaultRouter().ListChannels()
	var result []NotificationChannelType
	for _, sp := range spiChannels {
		if d, ok := notification.DefaultRouter().GetDescriptor(sp.Name()); ok {
			if d.PluginName != "" {
				p := h.findPlugin(d.PluginName)
				if p == nil || !h.pluginActive(orgID, p) {
					continue
				}
			}
			icon := d.Icon
			if icon == "" {
				icon = "bell"
				if sp.Name() == "email" {
					icon = "mail"
				}
			}
			title := d.Title
			if title == "" {
				title = sp.DisplayName()
			}
			desc := d.Description
			if desc == "" {
				desc = sp.DisplayName()
			}
			result = append(result, NotificationChannelType{
				Type:         sp.Name(),
				Title:        title,
				Description:  desc,
				Icon:         icon,
				ConfigSchema: sp.ConfigSchema(),
			})
		} else {
			icon := "bell"
			if sp.Name() == "email" {
				icon = "mail"
			}
			result = append(result, NotificationChannelType{
				Type:         sp.Name(),
				Title:        sp.DisplayName(),
				Description:  sp.DisplayName(),
				Icon:         icon,
				ConfigSchema: sp.ConfigSchema(),
			})
		}
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
	Ctx        huma.Context `hidden:"true"`
	Registered bool         `query:"registered" doc:"Return all registered SPI notification channels with Schemas"`
	Types      bool         `query:"types" doc:"Alias for registered"`
	Schema     bool         `query:"schema" doc:"Alias for registered"`
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
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	spiChannels := notification.DefaultRouter().ListChannels()
	if input.Registered || input.Types || input.Schema {
		var res []models.NotificationChannel
		for _, sp := range spiChannels {
			res = append(res, models.NotificationChannel{
				Name:         sp.DisplayName(),
				Type:         sp.Name(),
				ConfigSchema: sp.ConfigSchema(),
				Enabled:      true,
			})
		}
		return &ListNotificationChannelsOutput{Body: res}, nil
	}

	var channels []models.NotificationChannel
	h.orgDB(r).Order("created_at DESC").Find(&channels)
	for i := range channels {
		plain, err := h.channelConfigPlaintext(channels[i].Config)
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not decrypt channel configuration; retry the request")
		}
		channels[i].Config = redactConfigSecrets(plain)
		for _, sp := range spiChannels {
			if sp.Name() == channels[i].Type {
				channels[i].ConfigSchema = sp.ConfigSchema()
				break
			}
		}
	}
	return &ListNotificationChannelsOutput{Body: channels}, nil
}

type CreateNotificationChannelInputBody struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Config  *string `json:"config,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

type CreateNotificationChannelInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body CreateNotificationChannelInputBody
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
	name := strings.TrimSpace(input.Body.Name)
	typ := strings.ToLower(strings.TrimSpace(input.Body.Type))
	if name == "" || typ == "" {
		return nil, huma.Error400BadRequest("Channel name and type are required; provide both fields")
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	config := ""
	if input.Body.Config != nil {
		config = *input.Body.Config
	}
	encConfig, err := h.encryptChannelConfig(config)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not encrypt channel configuration; retry the request")
	}
	d := models.NotificationChannel{
		OrgID:   orgID,
		Name:    name,
		Type:    typ,
		Config:  encConfig,
		Enabled: enabled,
	}
	if err := h.db.Create(&d).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not create notification channel; retry the request")
	}
	h.audit(r, "notification.create", "notification_channel", d.ID, map[string]any{"name": d.Name, "type": d.Type})
	out := d
	out.Config = redactConfigSecrets(config)
	return &CreateNotificationChannelOutput{Body: out}, nil
}

type UpdateNotificationChannelInputBody struct {
	Name    *string `json:"name,omitempty"`
	Type    *string `json:"type,omitempty"`
	Config  *string `json:"config,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

type UpdateNotificationChannelInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body UpdateNotificationChannelInputBody
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
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&ch).Error != nil {
		return nil, huma.Error404NotFound("Notification channel not found; verify the channel identifier and try again")
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
		oldPlaintext, err := h.channelConfigPlaintext(ch.Config)
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not decrypt channel configuration; retry the request")
		}
		var targetPlaintext string
		if oldPlaintext != "" && strings.Contains(newCfgStr, "[REDACTED]") {
			targetPlaintext = mergeConfigPreservingSecrets(oldPlaintext, newCfgStr)
		} else {
			targetPlaintext = newCfgStr
		}
		enc, err := h.encryptChannelConfig(targetPlaintext)
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not encrypt channel configuration; retry the request")
		}
		ch.Config = enc
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
	plain, err := h.channelConfigPlaintext(ch.Config)
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not decrypt channel configuration; retry the request")
	}
	out.Config = redactConfigSecrets(plain)
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

type DeleteNotificationChannelOutputBody struct {
	OK bool `json:"ok"`
}

type DeleteNotificationChannelOutput struct {
	Body DeleteNotificationChannelOutputBody
}

func (h *Handler) deleteNotificationChannel(ctx context.Context, input *DeleteNotificationChannelInput) (*DeleteNotificationChannelOutput, error) {
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
	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).Delete(&models.NotificationChannel{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("Notification channel not found; verify the channel identifier and try again")
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

type TestNotificationChannelOutputBody struct {
	OK bool `json:"ok"`
}

type TestNotificationChannelOutput struct {
	Body TestNotificationChannelOutputBody
}

func (h *Handler) testNotificationChannel(ctx context.Context, input *TestNotificationChannelInput) (*TestNotificationChannelOutput, error) {
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
	var ch models.NotificationChannel
	if h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&ch).Error != nil {
		return nil, huma.Error404NotFound("Notification channel not found; verify the channel identifier and try again")
	}
	ctxTimeout, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if sp, ok := notification.DefaultRouter().GetChannel(ch.Type); ok {
		var cfgMap map[string]any
		plain, err := h.channelConfigPlaintext(ch.Config)
		if err != nil {
			return nil, huma.Error400BadRequest("Could not decrypt stored channel configuration; re-enter the configuration")
		}
		if plain != "" {
			if err := json.Unmarshal([]byte(plain), &cfgMap); err != nil {
				return nil, huma.Error400BadRequest("Stored channel configuration is not valid JSON; re-enter the configuration")
			}
		}
		if cfgMap == nil {
			cfgMap = make(map[string]any)
		}
		if ch.Type == "email" {
			if emailVal, _ := cfgMap["email"].(string); strings.TrimSpace(emailVal) == "" {
				var u models.User
				if err := h.db.First(&u, h.auth.UserID(r)).Error; err == nil && u.Email != "" {
					cfgMap["email"] = u.Email
				}
			}
		}
		err = sp.Send(ctxTimeout, plugin.NotificationRecipient{
			UserID: strconv.FormatUint(uint64(h.auth.UserID(r)), 10),
			OrgID:  strconv.FormatUint(uint64(orgID), 10),
			Config: cfgMap,
		}, plugin.NotificationPayload{
			Title:     "🔔 Test notification from Octarq",
			Body:      "Your notification channel is configured correctly.",
			EventType: "test",
			Priority:  "normal",
		})
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		out := &TestNotificationChannelOutput{}
		out.Body.OK = true
		return out, nil
	}

	return nil, huma.Error400BadRequest("Unsupported notification channel type " + ch.Type + "; choose an available channel type")
}
