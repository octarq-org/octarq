package app

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm/schema"
)

// preflightTableCollisions guards the single delayed AutoMigrate pass (core
// models + every plugin's Models(), see Run) against two distinct struct types
// silently mapping to the same table: AutoMigrate would apply both definitions
// in registration order and the later one would mutate the earlier one's schema.
//
// Table names are resolved the same way GORM's migrator resolves them — a
// TableName() string override wins, otherwise namer derives the name from the
// struct name — so the check sees exactly what AutoMigrate will see.
//
// One collision shape is deliberately allowed: a plugin mirroring an EXISTING
// core table with a local struct (e.g. `func (Setting) TableName() string {
// return "settings" }`) is the documented convention for plugins that read or
// extend core tables without importing internal/models, and must not fail
// startup. What fails is genuine ownership ambiguity: two different struct
// types from plugins (two plugins, or one plugin declaring both) claiming the
// same non-core table. Declaring the same struct type twice is harmless and
// tolerated (the pass is idempotent per type).
func preflightTableCollisions(namer schema.Namer, plugins []plugin.Plugin) error {
	if namer == nil {
		namer = schema.NamingStrategy{}
	}
	cache := &sync.Map{}
	tableOf := func(model any) (string, string, error) {
		s, err := schema.Parse(model, cache, namer)
		if err != nil {
			return "", "", err
		}
		return s.Table, s.ModelType.String(), nil
	}

	// Tables the core owns: plugin mirrors of these are allowed by convention.
	coreTables := make(map[string]bool)
	for _, m := range models.AllModels() {
		table, _, err := tableOf(m)
		if err != nil {
			return fmt.Errorf("preflight: parse core model %T: %w", m, err)
		}
		coreTables[table] = true
	}

	type owner struct {
		plugin string // Name() of the declaring plugin
		typ    string // fully qualified struct type
	}
	claimed := make(map[string]owner)
	for _, p := range plugins {
		for _, m := range p.Models() {
			table, typ, err := tableOf(m)
			if err != nil {
				return fmt.Errorf("preflight: parse model %T from plugin %q: %w", m, p.Name(), err)
			}
			if coreTables[table] {
				continue // mirroring a core table is the documented convention
			}
			if prev, ok := claimed[table]; ok && prev.typ != typ {
				return fmt.Errorf(
					"preflight: table %q is declared by two different model types: %s (plugin %q) and %s (plugin %q) — rename one model or its TableName() so each plugin-owned table has exactly one definition",
					table, prev.typ, prev.plugin, typ, p.Name(),
				)
			}
			claimed[table] = owner{plugin: p.Name(), typ: typ}
		}
	}
	return nil
}

// preflightNameCollisions refuses startup when two mounted plugins answer the
// same Name(). A plugin's name is its identity: Requires resolves against it,
// PluginEnabled falls back to it, MCP tools and log lines are attributed by it,
// and callers memoise per-plugin work under it. None of that has a defined
// meaning when two plugins share one — the second plugin silently inherits
// whatever the first registered.
//
// Plugins in a group share one FeatureKey and one toggle while keeping distinct
// names. Set Info.Group when multiple plugins belong to one feature toggle.
//
// Deliberately a hard failure, not a warning: the symptoms are silent
// wrong-content bugs that surface days later in a UI, and a name collision is
// always a composition mistake the operator can fix by not mounting both.
func preflightNameCollisions(plugins []plugin.Plugin) error {
	seen := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		name := p.Name()
		if seen[name] {
			return fmt.Errorf(
				"preflight: two mounted plugins are both named %q — a plugin name is its identity (Requires, enablement, MCP tools, per-plugin caches all key on it). "+
					"If they are two halves of one feature that should share a single toggle, give them distinct names and the same plugin.Info.Group instead",
				name,
			)
		}
		seen[name] = true
	}
	return nil
}

// preflightDependencies validates that every mounted plugin's Requires set is
// satisfied by the set of mounted plugins. Refuses startup if any required
// plugin is missing.
func preflightDependencies(plugins []plugin.Plugin) error {
	registered := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		registered[p.Name()] = true
	}
	for _, p := range plugins {
		info := plugin.Describe(p)
		for _, req := range info.Requires {
			if !registered[req] {
				return fmt.Errorf(
					"preflight: plugin %q requires plugin %q, which is not mounted (check build tags octarq_no%s / composition)",
					p.Name(), req, req,
				)
			}
		}
	}
	return nil
}

// ─── Route namespace preflight ──────────────────────────────────────────────
//
// plugin.Registry already refuses startup when two plugins Provide the same
// SERVICE name. Nothing guarded the same mistake for ROUTES, and the failure
// mode there is worse: http.ServeMux PANICS on a duplicate pattern, so two
// plugins claiming /api/products crashed the process at boot with a stack
// trace instead of a sentence naming the two plugins.
//
// routeRegistry sits between the plugins and the real mux. It records who
// claimed each pattern, refuses to forward a duplicate (which is what keeps
// ServeMux from panicking), and collects the collisions so Run can return them
// as a startup error in the same style as the duplicate-service one.

// thirdPartyNamespace is the path prefix reserved for out-of-tree plugins:
// /api/x/{plugin}/…  Everything under it belongs to the named plugin and can
// never collide with a core or Pro route, which is the point — an in-tree
// module is free to claim a new bare noun later without breaking somebody's
// third-party build.
const thirdPartyNamespace = "/api/x/"

// firstPartyModulePrefix identifies plugins shipped by the project itself
// (octarq core plugins and octarq-pro modules) by their Go package path. Only
// plugins from outside it are held to the namespace convention: the in-tree
// paths (/api/domains, /api/emails, /api/products …) predate the rule and are
// deliberately left alone.
const firstPartyModulePrefix = "github.com/octarq-org/"

// isThirdPartyPkg reports whether a Go package path belongs to an out-of-tree
// plugin. Split out from the plugin value so it is directly testable.
func isThirdPartyPkg(pkgPath string) bool {
	if pkgPath == "" {
		return false // unknown provenance: do not impose the rule
	}
	return !strings.HasPrefix(pkgPath, firstPartyModulePrefix)
}

// pluginIsThirdParty answers isThirdPartyPkg for a mounted plugin, using the
// package its concrete type was defined in.
func pluginIsThirdParty(p plugin.Plugin) bool {
	t := reflect.TypeOf(p)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		return false
	}
	return isThirdPartyPkg(t.PkgPath())
}

// routeRegistry tracks pattern ownership across every plugin mount.
type routeRegistry struct {
	mu    sync.Mutex
	owner map[string]string // normalised pattern -> plugin name
	errs  []error
}

func newRouteRegistry() *routeRegistry {
	return &routeRegistry{owner: make(map[string]string)}
}

// normalisePattern trims a ServeMux pattern to a comparable form. Patterns are
// "[METHOD ][HOST]/path"; only whitespace differs cosmetically.
func normalisePattern(pattern string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(pattern)), " ")
}

// patternPath returns just the path part of a ServeMux pattern.
func patternPath(pattern string) string {
	p := normalisePattern(pattern)
	if i := strings.Index(p, "/"); i >= 0 {
		return p[i:]
	}
	return p
}

// claim records pluginName as the owner of pattern. It returns false when the
// route must NOT be registered on the real mux — either because another plugin
// already owns it (registering it would panic) or because a third-party plugin
// left its reserved namespace.
func (r *routeRegistry) claim(pluginName, pattern string, thirdParty bool) bool {
	key := normalisePattern(pattern)
	r.mu.Lock()
	defer r.mu.Unlock()

	if prev, dup := r.owner[key]; dup {
		r.errs = append(r.errs, fmt.Errorf(
			"route %q is registered by two plugins: %q and %q — http.ServeMux panics on a duplicate pattern, so this is refused at startup. "+
				"Move one of them under its own namespace (%s{plugin}/…) or rename the path",
			key, prev, pluginName, thirdPartyNamespace,
		))
		return false
	}

	if thirdParty {
		path := patternPath(key)
		if strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, thirdPartyNamespace+pluginName+"/") && path != thirdPartyNamespace+pluginName {
			r.errs = append(r.errs, fmt.Errorf(
				"third-party plugin %q registered %q outside its reserved namespace — out-of-tree plugins must serve API routes under %s%s/ so a future core or Pro route cannot collide with them",
				pluginName, key, thirdPartyNamespace, pluginName,
			))
			return false
		}
	}

	r.owner[key] = pluginName
	return true
}

// Err reports the collisions recorded during mounting (nil if none), mirroring
// plugin.Registry.Err.
func (r *routeRegistry) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.errs) == 0 {
		return nil
	}
	if len(r.errs) == 1 {
		return fmt.Errorf("preflight: %w", r.errs[0])
	}
	return fmt.Errorf("preflight: %d route collisions, first: %w", len(r.errs), r.errs[0])
}

// recordingMux is the plugin.Mux (and huma sink) that consults the registry
// before letting a pattern reach the real mux.
type recordingMux struct {
	real       muxSink
	routes     *routeRegistry
	plugin     string
	thirdParty bool
}

// muxSink is the subset of *http.ServeMux the wrappers need, so recordingMux
// and gatedMux can nest in either order.
type muxSink interface {
	Handle(pattern string, handler http.Handler)
}

func (m *recordingMux) Handle(pattern string, h http.Handler) {
	if !m.routes.claim(m.plugin, pattern, m.thirdParty) {
		return
	}
	m.real.Handle(pattern, h)
}

func (m *recordingMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(h))
}
