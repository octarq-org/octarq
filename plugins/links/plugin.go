package links

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type Plugin struct {
	db   *gorm.DB
	auth struct {
		UserID func(r *http.Request) uint
		OrgID  func(r *http.Request) uint
	}
	audit               func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	getGlobalSetting    func(key string) string
	getWorkspaceSetting func(orgID uint, key string) string
	enqueue             func(ctx context.Context, taskType string, payload []byte) error
	deleteCache         func(ctx context.Context, key string) error
}

var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string          { return "links" }
func (p *Plugin) Describe() plugin.Info { return plugin.Info{Title: "Short Links", Core: true} }
func (p *Plugin) Models() []any {
	return []any{&Link{}, &LinkEvent{}}
}

func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.auth.OrgID(r))
}

func (p *Plugin) orgID(r *http.Request) uint {
	return p.auth.OrgID(r)
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.db = ctx.DB
	p.auth = struct {
		UserID func(r *http.Request) uint
		OrgID  func(r *http.Request) uint
	}{
		UserID: ctx.UserID,
		OrgID:  ctx.OrgID,
	}
	p.audit = ctx.Audit
	p.getGlobalSetting = ctx.GetGlobalSetting
	p.getWorkspaceSetting = ctx.GetWorkspaceSetting
	p.enqueue = ctx.Enqueue
	p.deleteCache = ctx.DeleteCache
	api := ctx.Huma

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links", Summary: "List Links", Tags: []string{"Links"}}, p.listLinks)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links", Summary: "Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, p.createLink)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/metadata", Summary: "Link Metadata", Tags: []string{"Links"}}, p.linkMetadata)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}", Summary: "Get Link", Tags: []string{"Links"}}, p.getLink)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/links/{id}", Summary: "Update Link", Tags: []string{"Links"}}, p.updateLink)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/links/{id}", Summary: "Delete Link", Tags: []string{"Links"}}, p.deleteLink)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/stats", Summary: "Link Stats", Tags: []string{"Links"}}, p.linkStats)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/qr", Summary: "Link QR", Tags: []string{"Links"}}, p.linkQR)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/export.csv", Summary: "Export Links", Tags: []string{"Links"}}, p.exportLinksCSV)

}

var builtinReservedSlugs = map[string]bool{
	"admin":  true,
	"api":    true,
	"assets": true,
	"portal": true,
}

func (p *Plugin) isReservedSlug(slug string) bool {
	slug = strings.ToLower(slug)
	if builtinReservedSlugs[slug] {
		return true
	}
	if p.getGlobalSetting != nil {
		for _, res := range splitList(p.getGlobalSetting("reserved_slugs")) {
			if res == slug {
				return true
			}
		}
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
