package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PluginEnabled reports whether the given feature key is enabled for the
// workspace. Features are opt-in: a missing row means disabled. Used by the
// route gate (app wraps every plugin handler with this check) and the menu
// filter. The key is a plugin's group, or its name when ungrouped.
func (h *Handler) PluginEnabled(orgID uint, featureKey string) bool {
	if orgID == 0 {
		return false
	}
	var ps models.PluginSetting
	err := h.db.Where("org_id = ? AND plugin = ?", orgID, featureKey).First(&ps).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Never toggled: fall back to the feature's declared default.
		return h.featureDefaultEnabled(featureKey)
	}
	if err != nil {
		return false
	}
	return ps.Enabled
}

// featureDefaultEnabled returns the pre-toggle default (Info.EnabledByDefault)
// for the feature identified by key, or false if no such plugin is registered.
func (h *Handler) featureDefaultEnabled(featureKey string) bool {
	for _, p := range h.plugins {
		if plugin.FeatureKey(p) == featureKey {
			return plugin.Describe(p).EnabledByDefault
		}
	}
	return false
}

// pluginActive reports whether a plugin's routes/menus should be live for the
// workspace: core plumbing is always on; every other plugin follows its
// feature's per-workspace toggle.
func (h *Handler) pluginActive(orgID uint, p plugin.Plugin) bool {
	if plugin.Describe(p).Core {
		return true
	}
	return h.PluginEnabled(orgID, plugin.FeatureKey(p))
}

// pluginMenuOut mirrors a plugin menu link for the management UI.
type pluginMenuOut struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
}

// featureOut is one toggleable feature in the plugin manager. Plugins sharing a
// group collapse into a single feature whose menus are the union of members'.
type featureOut struct {
	Key     string          `json:"key"`
	Title   string          `json:"title"`
	Enabled bool            `json:"enabled"`
	Menus   []pluginMenuOut `json:"menus"`
}

// listPlugins returns the toggleable features for the caller's workspace: every
// non-core plugin, grouped by feature key, with its per-workspace enabled state
// and the menu links it owns (so the UI can toggle it and the sidebar can hide
// the right items). Core plumbing plugins are omitted — they're always on.
// GET /api/plugins
type ListPluginsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListPluginsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListPluginsOutput struct {
	Body []featureOut
}

func (h *Handler) listPlugins(ctx context.Context, input *ListPluginsInput) (*ListPluginsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgID := h.orgID(r)

	enabled := map[string]bool{}
	var rows []models.PluginSetting
	h.db.Where("org_id = ?", orgID).Find(&rows)
	for _, row := range rows {
		enabled[row.Plugin] = row.Enabled
	}

	order := []string{}
	byKey := map[string]*featureOut{}
	for _, p := range h.plugins {
		info := plugin.Describe(p)
		if info.Core {
			continue
		}
		key := plugin.FeatureKey(p)
		f := byKey[key]
		if f == nil {
			// Effective state: an explicit row wins; otherwise the declared default.
			isOn, toggled := enabled[key]
			if !toggled {
				isOn = info.EnabledByDefault
			}
			f = &featureOut{Key: key, Title: info.Title, Enabled: isOn, Menus: []pluginMenuOut{}}
			byKey[key] = f
			order = append(order, key)
		} else if f.Title == "" {
			f.Title = info.Title
		}
		if mp, ok := p.(plugin.MenuProvider); ok {
			for _, m := range mp.Menus() {
				f.Menus = append(f.Menus, pluginMenuOut{ID: m.ID, Label: m.Label, Path: m.Path, Icon: m.Icon, Category: m.Category})
			}
		}
	}

	out := []featureOut{}
	for _, k := range order {
		f := byKey[k]
		if f.Title == "" {
			f.Title = f.Key
		}
		out = append(out, *f)
	}
	return &ListPluginsOutput{Body: out}, nil
}

// updatePlugin enables or disables a feature for the caller's workspace. Only an
// owner or admin may change it, since it flips whole feature areas on or off.
// PUT /api/plugins/{name}  {"enabled": true}   (name is the feature key)
type UpdatePluginInput struct {
	Ctx  huma.Context `hidden:"true"`
	Name string       `path:"name"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

func (i *UpdatePluginInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdatePluginOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (h *Handler) updatePlugin(ctx context.Context, input *UpdatePluginInput) (*UpdatePluginOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		return nil, huma.Error403Forbidden("owner or admin role required")
	}
	key := input.Name
	known := false
	for _, p := range h.plugins {
		if !plugin.Describe(p).Core && plugin.FeatureKey(p) == key {
			known = true
			break
		}
	}
	if !known {
		return nil, huma.Error404NotFound("unknown feature")
	}

	orgID := h.orgID(r)
	ps := models.PluginSetting{OrgID: orgID, Plugin: key, Enabled: input.Body.Enabled, UpdatedAt: time.Now()}
	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "plugin"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&ps).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save plugin setting")
	}

	h.audit(r, "plugin.toggle", "plugin", 0, map[string]any{"feature": key, "enabled": input.Body.Enabled})
	out := &UpdatePluginOutput{}
	out.Body.OK = true
	return out, nil
}
