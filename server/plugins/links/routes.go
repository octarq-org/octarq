package links

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

// Menus announces this plugin's sidebar entry so /api/menus only offers it
// when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "links", Label: "Links", Path: "/links", Icon: "link-2", Category: "Marketing", Order: 10},
	}
}

// Actions announces this plugin's global create affordances so /api/actions
// only offers them when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Actions() []plugin.Action {
	return []plugin.Action{
		{ID: "create-link", Label: "New Link", Path: "/links?create=1", Icon: "link-2", Category: "Marketing", Order: 10},
	}
}

// InstanceMenus announces this plugin's deployment-wide settings page so
// /api/instance/menus only offers it when the plugin is mounted. The
// reserved-slug list is one config per deployment (GET/PUT
// /api/instance-settings), so its editor lives in the /instance console —
// the tenant Links shell carries no instance-scope settings.
func (p *Plugin) InstanceMenus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "links-instance-settings", Label: "Short Link Settings", Path: "/link-settings", Icon: "link-2"},
	}
}

func (p *Plugin) registerRoutes(ctx *plugin.Context) {
	api := ctx.Huma
	if api != nil {
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links", Summary: "List Links", Tags: []string{"Links"}}, p.listLinks)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links", Summary: "Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, p.createLink)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links/quick", Summary: "Quick Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, p.quickCreateLink)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/metadata", Summary: "Link Metadata", Tags: []string{"Links"}}, p.linkMetadata)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}", Summary: "Get Link", Tags: []string{"Links"}}, p.getLink)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/links/{id}", Summary: "Update Link", Tags: []string{"Links"}}, p.updateLink)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/links/{id}", Summary: "Delete Link", Tags: []string{"Links"}}, p.deleteLink)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/stats", Summary: "Link Stats", Tags: []string{"Links"}}, p.linkStats)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/qr", Summary: "Link QR", Tags: []string{"Links"}}, p.linkQR)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/export.csv", Summary: "Export Links", Tags: []string{"Links"}}, p.exportLinksCSV)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/instance/link-settings", Summary: "Get Instance Link Settings", Tags: []string{"Settings"}}, p.getInstanceLinkSettings)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/instance/link-settings", Summary: "Update Instance Link Settings", Tags: []string{"Settings"}}, p.updateInstanceLinkSettings)
	}
	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[DeclarativeLinkInput, DeclarativeLinkOutput]{
		Name:        "create_shortlink",
		Method:      "POST",
		Path:        "/api/links/declarative",
		Summary:     "Create Short Link Declarative",
		Description: "Create a new short link via the declarative dual endpoint specification",
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.createDeclarativeLink,
	})
	if ctx.HandleRoot != nil {
		ctx.HandleRoot(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := strings.TrimPrefix(r.URL.Path, "/")
			if slug == "" {
				http.NotFound(w, r)
				return
			}
			link, ok := p.engine.Lookup(r.Host, slug)
			if !ok {
				http.NotFound(w, r)
				return
			}
			p.engine.Handle(w, r, link)
		}))
	}
}

func (p *Plugin) isInstanceAdminUser(r *http.Request) bool {
	if p.isInstanceAdmin == nil {
		return false
	}
	return p.isInstanceAdmin(r)
}

type GetInstanceLinkSettingsInput struct {
	Ctx huma.Context `json:"-"` // hidden
}

func (i *GetInstanceLinkSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type InstanceLinkSettingsBody struct {
	ReservedSlugs   string   `json:"reservedSlugs"`
	BuiltinReserved []string `json:"builtinReserved"`
}

type GetInstanceLinkSettingsOutput struct {
	Body InstanceLinkSettingsBody
}

func (p *Plugin) getInstanceLinkSettings(ctx context.Context, input *GetInstanceLinkSettingsInput) (*GetInstanceLinkSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if !p.isInstanceAdminUser(r) {
		return nil, huma.Error403Forbidden("instance admin required")
	}
	var reserved string
	if p.getGlobalSetting != nil {
		reserved = p.getGlobalSetting("reserved_slugs")
	}
	out := &GetInstanceLinkSettingsOutput{
		Body: InstanceLinkSettingsBody{
			ReservedSlugs:   reserved,
			BuiltinReserved: []string{"admin", "api", "assets", "portal"},
		},
	}
	return out, nil
}

type UpdateInstanceLinkSettingsInputBody struct {
	ReservedSlugs *string `json:"reservedSlugs,omitempty"`
}

type UpdateInstanceLinkSettingsInput struct {
	Ctx  huma.Context `json:"-"`
	Body UpdateInstanceLinkSettingsInputBody
}

func (i *UpdateInstanceLinkSettingsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateInstanceLinkSettingsOutput struct {
	Body InstanceLinkSettingsBody
}

func (p *Plugin) updateInstanceLinkSettings(ctx context.Context, input *UpdateInstanceLinkSettingsInput) (*UpdateInstanceLinkSettingsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if !p.isInstanceAdminUser(r) {
		return nil, huma.Error403Forbidden("instance admin required")
	}
	if input.Body.ReservedSlugs != nil {
		if p.ctx != nil && p.ctx.SetGlobalSetting != nil {
			_ = p.ctx.SetGlobalSetting("reserved_slugs", strings.Join(splitList(*input.Body.ReservedSlugs), "\n"))
		}
	}
	var reserved string
	if p.getGlobalSetting != nil {
		reserved = p.getGlobalSetting("reserved_slugs")
	}
	return &UpdateInstanceLinkSettingsOutput{
		Body: InstanceLinkSettingsBody{
			ReservedSlugs:   reserved,
			BuiltinReserved: []string{"admin", "api", "assets", "portal"},
		},
	}, nil
}
