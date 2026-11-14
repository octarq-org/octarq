// Package ontology turns an operator's declaration file into a working data
// surface: one table, one REST resource, one set of MCP tools and one sidebar
// entry per declared entity.
//
// The declaration is the only source of truth. It is read from the environment
// at Mount, validated as a whole, and applied before any route or tool is
// published. A declaration that cannot be applied exactly as written refuses the
// boot; nothing here falls back to a weaker interpretation of it.
package ontology

import (
	"context"
	"errors"
	"os"

	core "github.com/octarq-org/octarq/server/internal/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

const (
	// EntitiesEnv carries the declaration document inline, the same shape
	// OCTARQ_PLUGINS carries the plugin manifest in.
	EntitiesEnv = "OCTARQ_ENTITIES"
	// EntitiesFileEnv names a declaration file explicitly.
	EntitiesFileEnv = "OCTARQ_ENTITIES_FILE"
)

// Plugin is the generated data surface.
type Plugin struct {
	entities []*entity
	specs    []core.Spec
	loaded   bool
	db       *gorm.DB
	host     plugin.Host
	ctx      *plugin.Context
	// err refuses the boot: a declaration that cannot be parsed, validated or
	// applied exactly as written. Every composition root runs Validate, so the
	// same declaration is refused by the server, by `octarq mcp`, and by a
	// downstream binary that composes this plugin.
	err error
	// schemaErr is set when the generated tables cannot be used at all — no
	// database in this composition, or migrations never ran. Tools report it
	// instead of running SQL against a table that does not exist.
	schemaErr error
}

var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.Validator    = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.MCPProvider  = (*Plugin)(nil)
)

// New returns the plugin. Declarations are read on first use, by Mount or by
// Validate, whichever runs first in the composition root.
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "ontology" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Data",
		Description:      "Entities declared in octarq.entities.json: tables, REST routes and MCP tools.",
		Category:         plugin.CategoryUtilities,
		Icon:             "database",
		Core:             true,
		EnabledByDefault: true,
	}
}

// Models is the bookkeeping table. It is not a definition store: declarations
// live in the file, and this row only records what was applied, so the next boot
// can tell an unchanged declaration from a changed one.
func (p *Plugin) Models() []any { return []any{&core.AppliedSpec{}} }

// Mount loads the declarations, applies them, and registers the HTTP surface.
//
// Everything is read and checked before the first statement runs: a declaration
// refused half way through would leave a schema no declaration describes.
func (p *Plugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	if ctx == nil {
		p.err = errors.New("ontology: plugin context is required")
		return
	}
	p.ctx = ctx
	p.db = ctx.DB
	p.host = ctx.Host

	if err := p.ensureLoaded(); err != nil {
		return
	}
	if len(p.specs) == 0 {
		// No declaration anywhere: the feature is off. An installation that
		// declares no entities is a valid installation, not a broken one.
		return
	}

	if p.db == nil {
		p.schemaErr = ErrNoDatabase
		return
	}
	// The declarations are applied by whichever process opened the database.
	// `octarq mcp` may be the only process an installation ever runs, so leaving
	// the schema to the HTTP server would leave the tools unusable there
	// forever. Apply plans before it writes and is idempotent.
	//
	// Name() is the dialector's: gorm embeds the Dialector in Config, so the
	// promoted method answers with the driver the connection was opened with.
	if err := core.Apply(context.Background(), p.db, p.db.Name(), p.specs); err != nil {
		if errors.Is(err, core.ErrBookkeepingMissing) {
			// Migrations have not run in this process. The declarations are
			// valid; the tables simply do not exist yet.
			p.schemaErr = err
			return
		}
		p.err = err
		return
	}

	p.registerRoutes(ctx)
}

// ensureLoaded reads and validates the declaration once.
//
// Validate calls it as well as Mount, because the order differs per composition
// root: app.Run mounts first and validates after, while `octarq mcp` validates
// before the stdio mount. Reading the declaration only in Mount would let the
// CLI start with a declaration it never checked and publish no tool at all.
func (p *Plugin) ensureLoaded() error {
	if p.err != nil {
		return p.err
	}
	if p.loaded {
		return nil
	}
	p.loaded = true

	specs, err := core.Load(os.Getenv(EntitiesEnv), os.Getenv(EntitiesFileEnv))
	if err != nil {
		p.err = err
		return err
	}
	if len(specs) == 0 {
		return nil
	}

	entities := make([]*entity, 0, len(specs))
	for _, spec := range specs {
		e := newEntity(spec)
		if err := e.validate(); err != nil {
			p.err = err
			return err
		}
		entities = append(entities, e)
	}
	p.specs = specs
	p.entities = entities
	return nil
}

// Validate reports a declaration the boot must not continue with.
func (p *Plugin) Validate(context.Context, plugin.Host) error { return p.ensureLoaded() }

// Menus publishes one sidebar entry per declared entity.
func (p *Plugin) Menus() []plugin.MenuItem {
	// A schema that was never applied has no route behind it, so an entry would
	// open a 404.
	if p.schemaErr != nil {
		return nil
	}
	out := make([]plugin.MenuItem, 0, len(p.entities))
	for i, e := range p.entities {
		item := plugin.MenuItem{
			ID:       menuID(e.spec),
			Label:    pluralLabel(e.spec),
			Path:     pagePath(e.spec),
			Icon:     iconFor(e.spec),
			Category: "Workspace",
			Area:     "operations",
			Order:    50 + i,
		}
		// The entry opens the list. Hide it from roles that could not list the
		// entity, so the sidebar never leads to a permission error.
		if role := minRole(e.acl(core.OpList).Roles); role != "member" {
			item.RequiredRole = role
		}
		out = append(out, item)
	}
	return out
}

// menuID is the sidebar id. It is stable per slug because the frontend keys its
// translations on it (nav.<id>), falling back to the label.
func menuID(spec core.Spec) string { return "data-" + spec.Slug }

// pagePath is the dashboard route the generic page is served from.
func pagePath(spec core.Spec) string { return "/data/" + spec.Slug }

// apiPath is the REST collection path for one entity.
func apiPath(spec core.Spec) string { return "/api/data/" + spec.Slug }

// minRole returns the least privileged role an ACL grants. Role checks are
// minimum-role checks, so a grant of ["admin", "owner"] is a grant of admin.
func minRole(roles []string) string {
	best := ""
	for _, r := range roles {
		if rank := roleRank(r); rank > 0 && (best == "" || rank < roleRank(best)) {
			best = r
		}
	}
	return best
}

// aclAllows reports whether role may perform an operation. An unknown or empty
// role ranks below every declared role, so it is refused rather than defaulted.
func aclAllows(roles []string, role string) bool {
	min := minRole(roles)
	if min == "" || roleRank(role) == 0 {
		return false
	}
	return roleRank(role) >= roleRank(min)
}

// roleRank orders the platform's three built-in roles. Zero means "no role",
// which every check refuses.
func roleRank(role string) int {
	switch role {
	case "owner":
		return 3
	case "admin":
		return 2
	case "member":
		return 1
	default:
		return 0
	}
}
