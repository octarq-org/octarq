package links

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type Plugin struct {
	db     *gorm.DB
	engine *Engine
	auth   struct {
		UserID func(r *http.Request) uint
		OrgID  func(r *http.Request) uint
	}
	audit               func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	getGlobalSetting    func(key string) string
	getWorkspaceSetting func(orgID uint, key string) string
	enqueue             func(ctx context.Context, taskType string, payload []byte) error
	deleteCache         func(ctx context.Context, key string) error
	publishEvent        func(orgID uint, event string, data any)
	requireRole         func(r *http.Request, min string) bool
	isInstanceAdmin     func(r *http.Request) bool
	ctx                 *plugin.Context
}

var (
	_ plugin.Plugin               = (*Plugin)(nil)
	_ plugin.Describer            = (*Plugin)(nil)
	_ plugin.MenuProvider         = (*Plugin)(nil)
	_ plugin.InstanceMenuProvider = (*Plugin)(nil)
	_ plugin.HelpDocsFS           = (*Plugin)(nil)
	_ plugin.Starter              = (*Plugin)(nil)
)

// Compile-time service contract assertions: these methods are provided to the
// registry under the named contract types in Mount. A signature drift here
// fails the build instead of silently breaking consumers' LookupServiceAs.
var (
	_ plugin.ExportFunc   = (*Plugin)(nil).exportData
	_ plugin.PurgeFunc    = (*Plugin)(nil).purge
	_ plugin.OverviewFunc = (*Plugin)(nil).overview
	_ plugin.LinkResolver = (*Plugin)(nil).resolveSlug
	_ plugin.MCPExporter  = (*Plugin)(nil).mcpExportLinks
	_ plugin.CleanupFunc  = (*Plugin)(nil).cleanupEvents
)

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "links" }
func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Short Links",
		Description:      "Short link creation, custom domain routing, and click analytics.",
		Category:         plugin.CategoryMarketing,
		Tags:             []string{"url", "analytics", "routing"},
		EnabledByDefault: true,
		Requires:         []string{"dns"},
	}
}
func (p *Plugin) Models() []any {
	return []any{&Link{}, &LinkEvent{}}
}

// docs is this plugin's documentation directory. Adding a page means adding
// "docs/<slug>.mdx" (plus its "<slug>.zh.mdx" translation) — the file name is
// the slug and the frontmatter carries the rest; see plugin.HelpDocsFS.
//
//go:embed docs
var docs embed.FS

func (p *Plugin) HelpDocsFS() fs.FS { return docs }

func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.auth.OrgID(r))
}

func (p *Plugin) orgID(r *http.Request) uint {
	return p.auth.OrgID(r)
}

var builtinReservedSlugs = map[string]bool{
	"admin":      true,
	"api":        true,
	"assets":     true,
	"portal":     true,
	"robots.txt": true,
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

// hasRole reports whether the caller holds at least the given workspace role.
//
// A host that never wired RequireRole is refused rather than waved through. The
// gate protects destructive and credential-bearing operations, so "the host did
// not tell us who this is" has to mean no, not yes — an unwired seam would
// otherwise silently disable every role check in this plugin.
func (p *Plugin) hasRole(r *http.Request, min string) bool {
	if p.requireRole == nil {
		return false
	}
	return p.requireRole(r, min)
}
