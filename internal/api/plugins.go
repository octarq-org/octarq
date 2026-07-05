package api

import (
	"net/http"
	"time"

	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/plugin"
	"gorm.io/gorm/clause"
)

// PluginEnabled reports whether the named plugin is enabled for the given
// workspace. Plugins are opt-in: a missing row means disabled. Used both by the
// route gate (app wraps every plugin handler with this check) and menu filter.
func (h *Handler) PluginEnabled(orgID uint, name string) bool {
	if orgID == 0 {
		return false
	}
	var ps models.PluginSetting
	if err := h.db.Where("org_id = ? AND plugin = ?", orgID, name).First(&ps).Error; err != nil {
		return false
	}
	return ps.Enabled
}

// pluginMenuOut mirrors a plugin menu link for the management UI.
type pluginMenuOut struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
}

// listPlugins returns every registered plugin with its per-workspace enabled
// state and the menu links it owns (so the UI can both toggle it and know which
// sidebar items to hide when off).
// GET /api/plugins
func (h *Handler) listPlugins(w http.ResponseWriter, r *http.Request) {
	orgID := h.orgID(r)

	enabled := map[string]bool{}
	var rows []models.PluginSetting
	h.db.Where("org_id = ?", orgID).Find(&rows)
	for _, row := range rows {
		enabled[row.Plugin] = row.Enabled
	}

	type pluginOut struct {
		Name    string          `json:"name"`
		Enabled bool            `json:"enabled"`
		Menus   []pluginMenuOut `json:"menus"`
	}
	out := []pluginOut{}
	for _, p := range h.plugins {
		po := pluginOut{Name: p.Name(), Enabled: enabled[p.Name()], Menus: []pluginMenuOut{}}
		if mp, ok := p.(plugin.MenuProvider); ok {
			for _, m := range mp.Menus() {
				po.Menus = append(po.Menus, pluginMenuOut{ID: m.ID, Label: m.Label, Path: m.Path, Icon: m.Icon, Category: m.Category})
			}
		}
		out = append(out, po)
	}
	writeJSON(w, http.StatusOK, out)
}

// updatePlugin enables or disables a plugin for the caller's workspace. Only an
// owner or admin may change it, since it flips whole feature areas on or off.
// PUT /api/plugins/{name}  {"enabled": true}
func (h *Handler) updatePlugin(w http.ResponseWriter, r *http.Request) {
	if role := h.callerOrgRole(r); role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "owner or admin role required")
		return
	}
	name := r.PathValue("name")
	known := false
	for _, p := range h.plugins {
		if p.Name() == name {
			known = true
			break
		}
	}
	if !known {
		writeErr(w, http.StatusNotFound, "unknown plugin")
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
	ps := models.PluginSetting{OrgID: orgID, Plugin: name, Enabled: body.Enabled, UpdatedAt: time.Now()}
	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "plugin"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&ps).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save plugin setting")
		return
	}

	h.audit(r, "plugin.toggle", "plugin", 0, map[string]any{"plugin": name, "enabled": body.Enabled})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
