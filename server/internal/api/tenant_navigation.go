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
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Area     string `json:"area,omitempty"`
	Order    int    `json:"order,omitempty"`
	// Mirrors plugin.MenuItem.RequiredRole.
	RequiredRole string `json:"requiredRole,omitempty"`
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

// listMenus aggregates core menus and all plugin-registered menus.
// GET /api/menus
func (h *Handler) listMenus(ctx context.Context, input *ListMenusInput) (*ListMenusOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Core default navigation items — ONLY the pages the core itself serves
	// (plus the Infrastructure asset placeholders it owns). The backend is the
	// source of truth for which paths are "real", so the frontend drops any
	// composed menu whose path no backend half announces (see the sidebar merge
	// in web/src/App.tsx). Feature plugins (links, mail, dns, …) announce their
	// own entries via MenuProvider below, so a disabled plugin's path is never
	// offered.
	menus := []MenuItem{
		{ID: "overview", Label: "Overview", Path: "/overview", Icon: "layout-dashboard", Category: "Workspace", Area: "operations"},
		{ID: "inbox", Label: "Inbox", Path: "/notifications", Icon: "bell", Category: "Messaging", Area: "operations"},

		{ID: "audit", Label: "Audit Log", Path: "/audit", Icon: "scroll-text", Category: "System", Area: "operations"},
		{ID: "abuse", Label: "Abuse Reports", Path: "/abuse", Icon: "shield-alert", Category: "Security", Area: "operations"},
	}

	// Query from plugin providers if they satisfy MenuProvider — but only for
	// features the caller's workspace has active (core plumbing is always on;
	// everything else follows its per-workspace toggle).
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
				menus = append(menus, MenuItem{
					ID:           m.ID,
					Label:        m.Label,
					Path:         m.Path,
					Icon:         m.Icon,
					Category:     m.Category,
					Area:         m.Area,
					Order:        m.Order,
					RequiredRole: m.RequiredRole,
				})
			}
		}
	}

	return &ListMenusOutput{Body: menus}, nil
}

type Action struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Order    int    `json:"order,omitempty"`
	// Mirrors plugin.Action.RequiredRole.
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

// listActions aggregates plugin-registered actions for global create menu.
// GET /api/actions
func (h *Handler) listActions(ctx context.Context, input *ListActionsInput) (*ListActionsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
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

// getUserSettings returns the current user's preference key/value pairs.
// GET /api/user/settings
//
// The only key in use today is onboarding_dismissed, read by the Overview page.
func (h *Handler) getUserSettings(ctx context.Context, input *GetUserSettingsInput) (*GetUserSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	var settings []models.UserSetting
	if err := h.db.Where("user_id = ?", uid).Find(&settings).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to query user settings")
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

// updateUserSettings sets or updates a single user preference.
// PUT /api/user/settings  {"key": "onboarding_dismissed", "value": "true"}
func (h *Handler) updateUserSettings(ctx context.Context, input *UpdateUserSettingsInput) (*UpdateUserSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	key := strings.TrimSpace(input.Body.Key)
	if key == "" {
		return nil, huma.Error400BadRequest("key is required")
	}

	s := models.UserSetting{
		UserID:    uid,
		Key:       key,
		Value:     input.Body.Value,
		UpdatedAt: time.Now(),
	}
	if err := h.db.Save(&s).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save user setting")
	}
	out := &UpdateUserSettingsOutput{}
	out.Body.OK = true
	return out, nil
}
