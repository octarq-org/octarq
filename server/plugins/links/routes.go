package links

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/plugin"
)

// Menus announces this plugin's sidebar entry so /api/menus only offers it
// when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "links", Label: "Links", Path: "/links", Icon: "link-2", Category: "Marketing", Area: "operations", Order: 10},
	}
}

// Actions announces this plugin's global create affordances so /api/actions
// only offers them when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Actions() []plugin.Action {
	return []plugin.Action{
		{ID: "create-link", Label: "New Link", Path: "/links?create=1", Icon: "link-2", Category: "Marketing", Order: 10},
	}
}

func (p *Plugin) registerRoutes(ctx *plugin.Context) {
	api := ctx.Huma
	if api != nil {
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links", Summary: "List Links", Tags: []string{"Links"}}, p.listLinks)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links", Summary: "Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, p.createLink)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links/quick", Summary: "Quick Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, p.quickCreateLink)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/metadata", Summary: "Link Metadata", Tags: []string{"Links"}}, p.linkMetadata)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/capabilities", Summary: "Link Capabilities", Tags: []string{"Links"}}, p.linksCapabilities)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/geo-status", Summary: "Link Geo Status", Tags: []string{"Links"}}, p.linksCapabilities)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}", Summary: "Get Link", Tags: []string{"Links"}}, p.getLink)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/links/{id}", Summary: "Update Link", Tags: []string{"Links"}}, p.updateLink)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/links/{id}", Summary: "Delete Link", Tags: []string{"Links"}}, p.deleteLink)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/stats", Summary: "Link Stats", Tags: []string{"Links"}}, p.linkStats)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/qr", Summary: "Link QR", Tags: []string{"Links"}}, p.linkQR)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/export.csv", Summary: "Export Links", Tags: []string{"Links"}}, p.exportLinksCSV)
	}
	if err := plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[DeclarativeLinkInput, DeclarativeLinkOutput]{
		Name:        "create_shortlink",
		MCPToolName: "octarq_network__create_shortlink",
		Method:      "POST",
		Path:        "/api/links/declarative",
		Summary:     "Create Short Link Declarative",
		Description: "Create a new short link via the declarative dual endpoint specification",
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.createDeclarativeLink,
	}); err != nil {
		log.Printf("links: failed to register declarative endpoint: %v", err)
	}
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

type LinksCapabilitiesOutput struct {
	Body struct {
		GeoReady bool   `json:"geoReady"`
		Mode     string `json:"mode"`
	}
}

func (p *Plugin) isGeoReady() bool {
	if p.ctx == nil || p.ctx.GeoLookup == nil {
		return false
	}
	country, _, _ := p.ctx.GeoLookup("8.8.8.8")
	return country != ""
}

func (p *Plugin) linksCapabilities(ctx context.Context, _ *struct{}) (*LinksCapabilitiesOutput, error) {
	ready := p.isGeoReady()
	mode := "unavailable"
	if ready {
		mode = "configured"
	}
	out := &LinksCapabilitiesOutput{}
	out.Body.GeoReady = ready
	out.Body.Mode = mode
	return out, nil
}
