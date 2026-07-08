package api

import (
	"net/http"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
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
	if err := h.db.Where("org_id = ? AND plugin = ?", orgID, featureKey).First(&ps).Error; err != nil {
		return false
	}
	return ps.Enabled
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
func (h *Handler) listPlugins(w http.ResponseWriter, r *http.Request) {
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
			f = &featureOut{Key: key, Title: info.Title, Enabled: enabled[key], Menus: []pluginMenuOut{}}
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
	writeJSON(w, http.StatusOK, out)
}

// updatePlugin enables or disables a feature for the caller's workspace. Only an
// owner or admin may change it, since it flips whole feature areas on or off.
// PUT /api/plugins/{name}  {"enabled": true}   (name is the feature key)
func (h *Handler) updatePlugin(w http.ResponseWriter, r *http.Request) {
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "owner or admin role required")
		return
	}
	key := r.PathValue("name")
	known := false
	for _, p := range h.plugins {
		if !plugin.Describe(p).Core && plugin.FeatureKey(p) == key {
			known = true
			break
		}
	}
	if !known {
		writeErr(w, http.StatusNotFound, "unknown feature")
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	orgID := h.orgID(r)
	ps := models.PluginSetting{OrgID: orgID, Plugin: key, Enabled: body.Enabled, UpdatedAt: time.Now()}
	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "plugin"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&ps).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save plugin setting")
		return
	}

	h.audit(r, "plugin.toggle", "plugin", 0, map[string]any{"feature": key, "enabled": body.Enabled})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
