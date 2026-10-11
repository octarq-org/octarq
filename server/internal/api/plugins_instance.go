package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

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

// listInstancePlugins returns all registered plugins for instance administrators.
// GET /api/instance/plugins
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

		key := plugin.FeatureKey(p)
		out = append(out, instancePluginOut{
			Name:       p.Name(),
			FeatureKey: key,
			Title:      info.Title,
			Category:   cat,
			// Effective, not declared. This table's Kind column is what an
			// instance admin reads to answer "can a workspace turn this off",
			// and the answer belongs to the feature: a plugin that shares its
			// key with a Core sibling is always on however its own Info reads.
			// Showing info.Core here would label it "Opt-in" and be wrong.
			Core:             plugin.FeatureIsCore(h.plugins, key),
			EnabledByDefault: h.FeatureDefaultEnabled(key),
			Requires:         reqs,
			HasUI:            hasUI,
		})
	}

	return &ListInstancePluginsOutput{Body: out}, nil
}
