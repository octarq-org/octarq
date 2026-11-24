package api

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type MenuItem struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Path         string `json:"path"`
	Icon         string `json:"icon"`
	Category     string `json:"category"`
	Area         string `json:"area,omitempty"`
	Order        int    `json:"order,omitempty"`
	RequiredRole string `json:"requiredRole,omitempty"`
	External     bool   `json:"external"`
}

type ListMenusInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListMenusInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListMenusOutput struct {
	Body []MenuItem
}

func (h *Handler) listMenus(ctx context.Context, input *ListMenusInput) (*ListMenusOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	menus := []MenuItem{
		{ID: "overview", Label: "Overview", Path: "/overview", Icon: "layout-dashboard", Category: "Workspace", Area: "operations"},
		{ID: "inbox", Label: "Inbox", Path: "/notifications", Icon: "bell", Category: "Messaging", Area: "operations"},
		{ID: "abuse", Label: "Abuse Reports", Path: "/abuse", Icon: "shield-alert", Category: "Security", Area: "operations"},
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	for _, p := range h.plugins {
		if !h.pluginActive(orgID, p) {
			continue
		}
		if mp, ok := p.(plugin.MenuProvider); ok {
			for _, m := range mp.Menus() {
				if m.Feature != "" && !h.featureActive(orgID, m.Feature) {
					continue
				}
				menus = append(menus, MenuItem{
					ID:           m.ID,
					Label:        m.Label,
					Path:         m.Path,
					Icon:         m.Icon,
					Category:     m.Category,
					Area:         m.Area,
					Order:        m.Order,
					RequiredRole: m.RequiredRole,
					External:     m.External,
				})
			}
		}
	}

	return &ListMenusOutput{Body: menus}, nil
}

type Action struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Description  string `json:"description,omitempty"`
	Path         string `json:"path"`
	Icon         string `json:"icon"`
	Category     string `json:"category"`
	Order        int    `json:"order,omitempty"`
	RequiredRole string `json:"requiredRole,omitempty"`
}

type ListActionsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListActionsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListActionsOutput struct {
	Body []Action
}

func (h *Handler) listActions(ctx context.Context, input *ListActionsInput) (*ListActionsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	actions := make([]Action, 0)

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}
	for _, p := range h.plugins {
		if !h.pluginActive(orgID, p) {
			continue
		}
		if ap, ok := p.(plugin.ActionProvider); ok {
			for _, a := range ap.Actions() {
				actions = append(actions, Action{
					ID:           a.ID,
					Label:        a.Label,
					Description:  a.Description,
					Path:         a.Path,
					Icon:         a.Icon,
					Category:     a.Category,
					Order:        a.Order,
					RequiredRole: a.RequiredRole,
				})
			}
		}
	}

	return &ListActionsOutput{Body: actions}, nil
}

type GetUserSettingsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetUserSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetUserSettingsOutput struct {
	Body map[string]string
}

func (h *Handler) getUserSettings(ctx context.Context, input *GetUserSettingsInput) (*GetUserSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	uid := h.auth.UserID(r)
	var settings []models.UserSetting
	if err := h.db.Where("user_id = ?", uid).Find(&settings).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not load user settings; retry the request")
	}

	out := make(map[string]string)
	for _, s := range settings {
		out[s.Key] = s.Value
	}
	return &GetUserSettingsOutput{Body: out}, nil
}

type UpdateUserSettingsInputBody struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UpdateUserSettingsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateUserSettingsInputBody
}

func (i *UpdateUserSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateUserSettingsOutputBody struct {
	OK bool `json:"ok"`
}

type UpdateUserSettingsOutput struct {
	Body UpdateUserSettingsOutputBody
}

func (h *Handler) updateUserSettings(ctx context.Context, input *UpdateUserSettingsInput) (*UpdateUserSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}

	uid := h.auth.UserID(r)
	key := strings.TrimSpace(input.Body.Key)
	if key == "" {
		return nil, huma.Error400BadRequest("Setting key is required; provide a setting key")
	}

	s := models.UserSetting{
		UserID:    uid,
		Key:       key,
		Value:     input.Body.Value,
		UpdatedAt: time.Now(),
	}
	if err := h.db.Save(&s).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not save user setting; retry the request")
	}
	out := &UpdateUserSettingsOutput{}
	out.Body.OK = true
	return out, nil
}
