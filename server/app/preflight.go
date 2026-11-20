package app

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm/schema"
)

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

	coreTables := make(map[string]bool)
	for _, m := range models.AllModels() {
		table, _, err := tableOf(m)
		if err != nil {
			return fmt.Errorf("preflight: parse core model %T: %w", m, err)
		}
		coreTables[table] = true
	}

	type owner struct {
		plugin string
		typ    string
	}
	claimed := make(map[string]owner)
	for _, p := range plugins {
		for _, m := range p.Models() {
			table, typ, err := tableOf(m)
			if err != nil {
				return fmt.Errorf("preflight: parse model %T from plugin %q: %w", m, p.Name(), err)
			}
			if coreTables[table] {
				continue
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

const thirdPartyNamespace = "/api/x/"

const firstPartyModulePrefix = "github.com/octarq-org/"

func isThirdPartyPkg(pkgPath string) bool {
	if pkgPath == "" {
		return false
	}
	return !strings.HasPrefix(pkgPath, firstPartyModulePrefix)
}

func pluginIsThirdParty(p plugin.Plugin) bool {
	t := reflect.TypeOf(p)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return false
	}
	return isThirdPartyPkg(t.PkgPath())
}

type routeRegistry struct {
	mu    sync.Mutex
	owner map[string]string
	errs  []error
}

func newRouteRegistry() *routeRegistry {
	return &routeRegistry{owner: make(map[string]string)}
}

func normalisePattern(pattern string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(pattern)), " ")
}

func patternPath(pattern string) string {
	p := normalisePattern(pattern)
	if i := strings.Index(p, "/"); i >= 0 {
		return p[i:]
	}
	return p
}

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

type recordingMux struct {
	real       muxSink
	routes     *routeRegistry
	plugin     string
	thirdParty bool
}

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
