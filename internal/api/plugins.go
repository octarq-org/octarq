package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/authz"
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
// Any member declaring the default is enough: siblings sharing a feature key
// toggle as one, so an opt-out half makes the whole feature opt-out.
func (h *Handler) featureDefaultEnabled(featureKey string) bool {
	for _, p := range h.plugins {
		if plugin.FeatureKey(p) == featureKey && plugin.Describe(p).EnabledByDefault {
			return true
		}
	}
	return false
}

// pluginActive reports whether a plugin's routes/menus should be live for the
// workspace: core plumbing is always on; every other plugin follows its
// feature's per-workspace toggle.
func (h *Handler) pluginActive(orgID uint, p plugin.Plugin) bool {
	key := plugin.FeatureKey(p)
	if plugin.FeatureIsCore(h.plugins, key) {
		return true
	}
	return h.PluginEnabled(orgID, key)
}

// dependencyConflictError is the 409 returned when a workspace tries to disable
// a feature that another enabled feature declares in its Requires set.
//
// It implements huma.StatusError so the dependent names ship as a real JSON
// field. The UI lists them ("Mail is using DNS — turn Mail off first"), and
// huma's generic `errors` array is a list of validation messages, not a place
// to hide structured data a client has to parse back out.
type dependencyConflictError struct {
	Status     int      `json:"status"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Feature    string   `json:"feature"`
	Dependents []string `json:"dependents"`
}

func (e *dependencyConflictError) GetStatus() int { return http.StatusConflict }

func (e *dependencyConflictError) Error() string {
	return fmt.Sprintf("%s is required by %s", e.Feature, strings.Join(e.Dependents, ", "))
}

// MarshalJSON fills the envelope fields lazily so callers only have to set
// Feature and Dependents.
func (e *dependencyConflictError) MarshalJSON() ([]byte, error) {
	type alias dependencyConflictError
	out := alias(*e)
	out.Status = http.StatusConflict
	out.Title = "Conflict"
	out.Detail = e.Error()
	return json.Marshal(out)
}

// pluginMenuOut mirrors a plugin menu link for the management UI.
type pluginMenuOut struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Order    int    `json:"order,omitempty"`
}

// featureOut is one toggleable feature in the plugin manager. Plugins sharing a
// group collapse into a single feature whose menus are the union of members'.
type featureOut struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Icon        string          `json:"icon,omitempty"`
	Category    string          `json:"category,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Enabled     bool            `json:"enabled"`
	Requires    []string        `json:"requires"`
	RequiredBy  []string        `json:"requiredBy"`
	Menus       []pluginMenuOut `json:"menus"`
}

func (h *Handler) getFeatureDeps() (map[string]string, map[string][]string) {
	nameToFeatureKey := make(map[string]string)
	for _, p := range h.plugins {
		nameToFeatureKey[p.Name()] = plugin.FeatureKey(p)
	}
	requires := make(map[string][]string)
	for _, p := range h.plugins {
		fKey := plugin.FeatureKey(p)
		for _, reqName := range plugin.Describe(p).Requires {
			if reqKey, ok := nameToFeatureKey[reqName]; ok {
				if reqKey != fKey {
					found := false
					for _, existing := range requires[fKey] {
						if existing == reqKey {
							found = true
							break
						}
					}
					if !found {
						requires[fKey] = append(requires[fKey], reqKey)
					}
				}
			}
		}
	}
	return nameToFeatureKey, requires
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
	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	enabled := map[string]bool{}
	var rows []models.PluginSetting
	h.db.Where("org_id = ?", orgID).Find(&rows)
	for _, row := range rows {
		enabled[row.Plugin] = row.Enabled
	}

	_, requires := h.getFeatureDeps()
	effectiveEnabled := make(map[string]bool)
	for _, p := range h.plugins {
		key := plugin.FeatureKey(p)
		if isOn, toggled := enabled[key]; toggled {
			effectiveEnabled[key] = isOn
		} else {
			effectiveEnabled[key] = h.featureDefaultEnabled(key)
		}
	}

	// requiredBy counts only *enabled* dependents — a disabled feature must not
	// lock one the workspace is free to turn off. Sorted because map iteration
	// order would otherwise reshuffle the list on every request.
	requiredBy := make(map[string][]string)
	for fKey, reqs := range requires {
		if effectiveEnabled[fKey] {
			for _, r := range reqs {
				requiredBy[r] = append(requiredBy[r], fKey)
			}
		}
	}
	for k := range requiredBy {
		sort.Strings(requiredBy[k])
	}

	order := []string{}
	byKey := map[string]*featureOut{}
	for _, p := range h.plugins {
		info := plugin.Describe(p)
		key := plugin.FeatureKey(p)
		if plugin.FeatureIsCore(h.plugins, key) {
			continue
		}
		f := byKey[key]
		cat := info.Category
		if cat != "" && !plugin.ValidCategories[cat] {
			cat = plugin.CategoryUtilities
		}
		if cat == "" {
			cat = plugin.CategoryUtilities
		}

		if f == nil {
			fReqs := requires[key]
			if fReqs == nil {
				fReqs = []string{}
			}
			fReqBy := requiredBy[key]
			if fReqBy == nil {
				fReqBy = []string{}
			}

			f = &featureOut{
				Key:         key,
				Title:       info.Title,
				Description: info.Description,
				Icon:        info.Icon,
				Category:    cat,
				Tags:        info.Tags,
				Enabled:     effectiveEnabled[key],
				Requires:    fReqs,
				RequiredBy:  fReqBy,
				Menus:       []pluginMenuOut{},
			}
			byKey[key] = f
			order = append(order, key)
		} else {
			if f.Title == "" {
				f.Title = info.Title
			}
			if f.Description == "" {
				f.Description = info.Description
			}
			if f.Icon == "" {
				f.Icon = info.Icon
			}
			if f.Category == "" {
				f.Category = cat
			}
			if len(f.Tags) == 0 && len(info.Tags) > 0 {
				f.Tags = info.Tags
			}
		}
		if mp, ok := p.(plugin.MenuProvider); ok {
			for _, m := range mp.Menus() {
				f.Menus = append(f.Menus, pluginMenuOut{ID: m.ID, Label: m.Label, Path: m.Path, Icon: m.Icon, Category: m.Category, Order: m.Order})
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
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("owner or admin role required")
	}
	key := input.Name
	known := false
	if !plugin.FeatureIsCore(h.plugins, key) {
		for _, p := range h.plugins {
			if plugin.FeatureKey(p) == key {
				known = true
				break
			}
		}
	}
	if !known {
		return nil, huma.Error404NotFound("unknown feature")
	}

	orgID, err := h.requireOrg(r)
	if err != nil {
		return nil, err
	}

	_, requires := h.getFeatureDeps()
	var rows []models.PluginSetting
	h.db.Where("org_id = ?", orgID).Find(&rows)
	enabledMap := make(map[string]bool)
	for _, p := range h.plugins {
		enabledMap[plugin.FeatureKey(p)] = h.featureDefaultEnabled(plugin.FeatureKey(p))
	}
	for _, row := range rows {
		enabledMap[row.Plugin] = row.Enabled
	}

	if !input.Body.Enabled {
		var dependents []string
		for dependent, reqs := range requires {
			if enabledMap[dependent] {
				for _, req := range reqs {
					if req == key {
						dependents = append(dependents, dependent)
						break
					}
				}
			}
		}
		if len(dependents) > 0 {
			// Sorted so the message is stable across requests (map iteration
			// order is not), and carried as a typed `dependents` field rather
			// than smuggled through huma's validation-error list — the UI
			// renders the names, so they need to survive as data.
			sort.Strings(dependents)
			return nil, &dependencyConflictError{Feature: key, Dependents: dependents}
		}

		ps := models.PluginSetting{OrgID: orgID, Plugin: key, Enabled: false, UpdatedAt: time.Now()}
		if err := h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "org_id"}, {Name: "plugin"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
		}).Create(&ps).Error; err != nil {
			return nil, huma.Error500InternalServerError("failed to save plugin setting")
		}
		h.audit(r, "plugin.toggle", "plugin", 0, map[string]any{"feature": key, "enabled": false})
	} else {
		err := h.db.Transaction(func(tx *gorm.DB) error {
			visited := make(map[string]bool)
			var enableRecursive func(string) error
			enableRecursive = func(fKey string) error {
				if visited[fKey] {
					return nil
				}
				visited[fKey] = true
				for _, req := range requires[fKey] {
					if err := enableRecursive(req); err != nil {
						return err
					}
				}
				if !enabledMap[fKey] || fKey == key {
					ps := models.PluginSetting{OrgID: orgID, Plugin: fKey, Enabled: true, UpdatedAt: time.Now()}
					if err := tx.Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "org_id"}, {Name: "plugin"}},
						DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
					}).Create(&ps).Error; err != nil {
						return err
					}
					enabledMap[fKey] = true
					auditProps := map[string]any{"feature": fKey, "enabled": true}
					if fKey != key {
						auditProps["cascade"] = true
					}
					h.audit(r, "plugin.toggle", "plugin", 0, auditProps)
				}
				return nil
			}
			return enableRecursive(key)
		})
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to save plugin settings")
		}
	}

	out := &UpdatePluginOutput{}
	out.Body.OK = true
	return out, nil
}

type instancePluginOut struct {
	Name             string   `json:"name"`
	FeatureKey       string   `json:"featureKey"`
	Title            string   `json:"title"`
	Category         string   `json:"category"`
	Core             bool     `json:"core"`
	EnabledByDefault bool     `json:"enabledByDefault"`
	Requires         []string `json:"requires"`
	HasUI            bool     `json:"hasUI"`
}

type ListInstancePluginsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListInstancePluginsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListInstancePluginsOutput struct {
	Body []instancePluginOut
}

func (h *Handler) listInstancePlugins(ctx context.Context, input *ListInstancePluginsInput) (*ListInstancePluginsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin required")
	}

	var out []instancePluginOut
	for _, p := range h.plugins {
		info := plugin.Describe(p)

		cat := info.Category
		if cat != "" && !plugin.ValidCategories[cat] {
			cat = plugin.CategoryUtilities
		}
		if cat == "" {
			cat = plugin.CategoryUtilities
		}

		_, hasUI := p.(plugin.MenuProvider)

		reqs := info.Requires
		if reqs == nil {
			reqs = []string{}
		}

		out = append(out, instancePluginOut{
			Name:             p.Name(),
			FeatureKey:       plugin.FeatureKey(p),
			Title:            info.Title,
			Category:         cat,
			Core:             info.Core,
			EnabledByDefault: info.EnabledByDefault,
			Requires:         reqs,
			HasUI:            hasUI,
		})
	}

	return &ListInstancePluginsOutput{Body: out}, nil
}
