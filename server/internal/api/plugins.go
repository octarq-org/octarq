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
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handler) PluginEnabled(orgID uint, featureKey string) bool {
	if orgID == 0 {
		return false
	}
	var ps models.PluginSetting
	err := h.db.Where("org_id = ? AND plugin = ?", orgID, featureKey).First(&ps).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return h.FeatureDefaultEnabled(featureKey)
	}
	if err != nil {
		return false
	}
	return ps.Enabled
}

func (h *Handler) FeatureDefaultEnabled(featureKey string) bool {
	for _, p := range h.plugins {
		if plugin.FeatureKey(p) == featureKey && plugin.Describe(p).EnabledByDefault {
			return true
		}
	}
	return false
}

func (h *Handler) pluginActive(orgID uint, p plugin.Plugin) bool {
	return h.featureActive(orgID, plugin.FeatureKey(p))
}

func (h *Handler) featureActive(orgID uint, featureKey string) bool {
	if plugin.FeatureIsCore(h.plugins, featureKey) {
		return true
	}
	return h.PluginEnabled(orgID, featureKey)
}

func (h *Handler) activePlugins(orgID uint) []plugin.Plugin {
	if orgID == 0 {
		return nil
	}
	out := make([]plugin.Plugin, 0, len(h.plugins))
	for _, p := range h.plugins {
		if h.pluginActive(orgID, p) {
			out = append(out, p)
		}
	}
	return out
}

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

func (e *dependencyConflictError) MarshalJSON() ([]byte, error) {
	type alias dependencyConflictError
	out := alias(*e)
	out.Status = http.StatusConflict
	out.Title = "Conflict"
	out.Detail = e.Error()
	return json.Marshal(out)
}

type pluginMenuOut struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`
	Category string `json:"category"`
	Area     string `json:"area,omitempty"`
	Order    int    `json:"order,omitempty"`
	External bool   `json:"external"`
}

type featureOut struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Icon        string          `json:"icon,omitempty"`
	Category    string          `json:"category,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Enabled     bool            `json:"enabled"`
	Core        bool            `json:"core"`
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

type ListPluginsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListPluginsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type pluginsOutputBody struct {
	Plugins    []featureOut          `json:"plugins"`
	Categories []plugin.CategoryInfo `json:"categories"`
}

type ListPluginsOutput struct {
	Body pluginsOutputBody
}

func (h *Handler) listPlugins(ctx context.Context, input *ListPluginsInput) (*ListPluginsOutput, error) {
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
			effectiveEnabled[key] = h.FeatureDefaultEnabled(key)
		}
	}

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
	type ownedMenu struct {
		feature string
		menu    pluginMenuOut
	}
	var ownedMenus []ownedMenu
	for _, p := range h.plugins {
		info := plugin.Describe(p)
		key := plugin.FeatureKey(p)
		isCore := plugin.FeatureIsCore(h.plugins, key)
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
				Enabled:     isCore || effectiveEnabled[key],
				Core:        isCore,
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
				owner := m.Feature
				if owner == "" {
					owner = key
				}
				ownedMenus = append(ownedMenus, ownedMenu{
					feature: owner,
					menu: pluginMenuOut{
						ID:       m.ID,
						Label:    m.Label,
						Path:     m.Path,
						Icon:     m.Icon,
						Category: m.Category,
						Area:     m.Area,
						Order:    m.Order,
						External: m.External,
					},
				})
			}
		}
	}

	for _, om := range ownedMenus {
		if f := byKey[om.feature]; f != nil {
			f.Menus = append(f.Menus, om.menu)
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
	return &ListPluginsOutput{Body: pluginsOutputBody{Plugins: out, Categories: plugin.PluginCategories()}}, nil
}

type UpdatePluginInputBody struct {
	Enabled bool `json:"enabled"`
}

type UpdatePluginInput struct {
	Ctx  huma.Context `hidden:"true"`
	Name string       `path:"name"`
	Body UpdatePluginInputBody
}

func (i *UpdatePluginInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdatePluginOutputBody struct {
	OK bool `json:"ok"`
}

type UpdatePluginOutput struct {
	Body UpdatePluginOutputBody
}

func (h *Handler) updatePlugin(ctx context.Context, input *UpdatePluginInput) (*UpdatePluginOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if err := h.requireRole(r, authz.RoleAdmin); err != nil {
		return nil, huma.Error403Forbidden("Workspace owner or administrator role required; contact a workspace administrator")
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
		return nil, huma.Error404NotFound("Feature not found; verify the feature name and try again")
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
		enabledMap[plugin.FeatureKey(p)] = h.FeatureDefaultEnabled(plugin.FeatureKey(p))
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
			sort.Strings(dependents)
			return nil, &dependencyConflictError{Feature: key, Dependents: dependents}
		}

		ps := models.PluginSetting{OrgID: orgID, Plugin: key, Enabled: false, UpdatedAt: time.Now()}
		if err := h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "org_id"}, {Name: "plugin"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
		}).Create(&ps).Error; err != nil {
			return nil, huma.Error500InternalServerError("Could not save feature configuration; retry the request")
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
			return nil, huma.Error500InternalServerError("Could not save feature configuration; retry the request")
		}
	}

	out := &UpdatePluginOutput{}
	out.Body.OK = true
	return out, nil
}
