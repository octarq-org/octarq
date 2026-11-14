// Package ontology turns a declaration into a working data surface: one table,
// one REST resource, one set of MCP tools and one sidebar entry per declared
// entity.
//
// A declaration comes from the operator's document or from a module that
// registered an entity in Go (see server/ontology.Registry). Both are read at
// Mount, validated as one set, and applied before any route or tool is
// published. A declaration that cannot be applied exactly as written refuses the
// boot; nothing here falls back to a weaker interpretation of it.
package ontology

import (
	"context"
	"errors"
	"fmt"
	"os"

	core "github.com/octarq-org/octarq/server/ontology"
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
	// registry holds the entities a module declared in Go. It is read once, with
	// the declaration file, so both sources are validated and applied together.
	registry *core.Registry
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

// ErrNoRegistry is returned by a plugin constructed without one. Every module's
// declarations live there, so a nil registry is a composition that would apply
// the file and silently drop the code — refused rather than half-applied.
var ErrNoRegistry = errors.New("ontology: the plugin was constructed without a declaration registry")

// New returns the plugin, reading the process-wide registry a module registers
// its entities in. Declarations are read on first use, by Mount or by Validate,
// whichever runs first in the composition root.
func New() *Plugin { return NewWithRegistry(core.Default()) }

// NewWithRegistry returns the plugin reading r instead of the process-wide set.
// Tests use it to keep their declarations apart; a composition root uses it when
// it wants its entities owned by one binary rather than by the process.
func NewWithRegistry(r *core.Registry) *Plugin { return &Plugin{registry: r} }

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

	// A workspace deletion has to take its declared rows with it, and nothing
	// else knows these tables exist. Provided after Apply, so the tables this
	// purge walks are the ones that were actually created.
	if ctx.Provide != nil {
		ctx.Provide(plugin.PurgeServiceName(p.Name()), plugin.PurgeFunc(p.purge))
	}
}

// ensureLoaded reads and validates the declarations once.
//
// Validate calls it as well as Mount, because the order differs per composition
// root: app.Run mounts first and validates after, while `octarq mcp` validates
// before the stdio mount. Reading the declaration only in Mount would let the
// CLI start with a declaration it never checked and publish no tool at all.
//
// Two sources meet here: the operator's file and the entities a module
// registered in Go. Neither is optional — a build can ship both, and an
// installation can declare neither. They are validated as one set, so a slug
// claimed twice is refused before a single statement runs.
func (p *Plugin) ensureLoaded() error {
	if p.err != nil {
		return p.err
	}
	if p.loaded {
		return nil
	}
	p.loaded = true

	fileSpecs, err := core.Load(os.Getenv(EntitiesEnv), os.Getenv(EntitiesFileEnv))
	if err != nil {
		p.err = err
		return err
	}
	if p.registry == nil {
		// A composition that constructs the plugin without a registry has no way
		// to see a module's declarations, and silently ignoring them would apply
		// half of what the binary declares.
		p.err = ErrNoRegistry
		return p.err
	}
	codeSpecs := p.registry.Declared()

	for _, code := range codeSpecs {
		for _, file := range fileSpecs {
			if code.Slug == file.Slug {
				// The set-wide validation below would catch this too, but it can
				// only name the index. This is the message a module author needs.
				p.err = fmt.Errorf("ontology: entity %q is declared both in the declaration file and in code; declare it once", code.Slug)
				return p.err
			}
		}
	}

	specs := make([]core.Spec, 0, len(fileSpecs)+len(codeSpecs))
	specs = append(specs, fileSpecs...)
	specs = append(specs, codeSpecs...)
	if len(specs) == 0 {
		return nil
	}
	if err := core.ValidateAll(specs); err != nil {
		p.err = err
		return err
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
