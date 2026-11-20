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
	EntitiesEnv     = "OCTARQ_ENTITIES"
	EntitiesFileEnv = "OCTARQ_ENTITIES_FILE"
)

type Plugin struct {
	entities  []*entity
	specs     []core.Spec
	registry  *core.Registry
	loaded    bool
	db        *gorm.DB
	host      plugin.Host
	ctx       *plugin.Context
	err       error
	schemaErr error
}

var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.Validator    = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.MCPProvider  = (*Plugin)(nil)
)

var ErrNoRegistry = errors.New("ontology: the plugin was constructed without a declaration registry")

func New() *Plugin { return NewWithRegistry(core.Default()) }

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

func (p *Plugin) Models() []any { return []any{&core.AppliedSpec{}} }

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
	if err := p.resolveOwners(); err != nil {
		p.err = err
		return
	}
	if len(p.specs) == 0 {
		return
	}

	if p.db == nil {
		p.schemaErr = ErrNoDatabase
		return
	}
	if err := core.Apply(context.Background(), p.db, p.db.Name(), p.specs); err != nil {
		if errors.Is(err, core.ErrBookkeepingMissing) {
			p.schemaErr = err
			return
		}
		p.err = err
		return
	}

	p.registerRoutes(ctx)

	if ctx.Provide != nil {
		ctx.Provide(plugin.PurgeServiceName(p.Name()), plugin.PurgeFunc(p.purge))
	}
	p.provideWriter(ctx)
}

func (p *Plugin) provideWriter(ctx *plugin.Context) {
	if ctx.Provide == nil {
		return
	}
	ctx.Provide(core.ServiceRecords, recordWriter{p: p})
}

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
		p.err = ErrNoRegistry
		return p.err
	}
	codeSpecs := p.registry.Declared()

	for _, code := range codeSpecs {
		for _, file := range fileSpecs {
			if code.Slug == file.Slug {
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

func (p *Plugin) Validate(context.Context, plugin.Host) error { return p.ensureLoaded() }

func (p *Plugin) resolveOwners() error {
	var first *entity
	for _, e := range p.entities {
		if e.owned() {
			first = e
			break
		}
	}
	if first == nil {
		return nil
	}
	if p.ctx == nil || p.ctx.ActivePlugins == nil {
		return fmt.Errorf("ontology: entity %q is declared by plugin %q, and this host cannot list the composed plugins; the entity would have no switch", first.spec.Slug, first.spec.Owner)
	}
	if p.ctx.FeatureActive == nil {
		return fmt.Errorf("ontology: entity %q is declared by plugin %q, and this host cannot answer whether a feature is on; the entity would have no switch", first.spec.Slug, first.spec.Owner)
	}

	byName := make(map[string]plugin.Plugin, len(p.entities))
	for _, pl := range p.ctx.ActivePlugins() {
		byName[pl.Name()] = pl
	}
	for _, e := range p.entities {
		if !e.owned() {
			continue
		}
		pl, ok := byName[e.spec.Owner]
		if !ok {
			return fmt.Errorf("ontology: entity %q is declared by plugin %q, which is not in this build; mount the plugin or drop the declaration", e.spec.Slug, e.spec.Owner)
		}
		e.featureKey = plugin.FeatureKey(pl)
		e.active = p.ctx.FeatureActive
	}
	return nil
}

func (p *Plugin) Menus() []plugin.MenuItem {
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
			Feature:  e.featureKey,
		}
		if role := minRole(e.acl(core.OpList).Roles); role != "member" {
			item.RequiredRole = role
		}
		out = append(out, item)
	}
	return out
}

func menuID(spec core.Spec) string { return "data-" + spec.Slug }

func pagePath(spec core.Spec) string { return "/data/" + spec.Slug }

func apiPath(spec core.Spec) string { return "/api/data/" + spec.Slug }

func minRole(roles []string) string {
	best := ""
	for _, r := range roles {
		if rank := roleRank(r); rank > 0 && (best == "" || rank < roleRank(best)) {
			best = r
		}
	}
	return best
}

func aclAllows(roles []string, role string) bool {
	min := minRole(roles)
	if min == "" || roleRank(role) == 0 {
		return false
	}
	return roleRank(role) >= roleRank(min)
}

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
