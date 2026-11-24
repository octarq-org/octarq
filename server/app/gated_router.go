package app

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

func isPlatformReservedPath(path string) bool {
	switch path {
	case "/api/instance-settings", "/api/health", "/api/status", "/api/mcp", "/mcp":
		return true
	}
	if strings.Contains(path, "/instance/") || strings.Contains(path, "/storage-instance/") ||
		strings.HasPrefix(path, "/api/instance/") || strings.HasPrefix(path, "/instance/") {
		return true
	}
	if strings.HasPrefix(path, "/api/mcp/") || strings.HasPrefix(path, "/mcp/") {
		return true
	}
	if path == "/api/auth/login" {
		return true
	}
	if strings.HasPrefix(path, "/api/auth/") && (strings.HasSuffix(path, "/login") || strings.HasSuffix(path, "/callback")) {
		return true
	}
	if strings.HasPrefix(path, "/api/sso/") {
		if path == "/api/sso/login" || strings.HasSuffix(path, "/login") || strings.HasSuffix(path, "/callback") {
			return true
		}
	}
	if strings.HasPrefix(path, "/auth/begin/") || strings.HasPrefix(path, "/auth/callback/") {
		return true
	}
	return false
}

type unscopedMatcher struct {
	exact    map[string]bool
	prefixes []string
}

func newUnscopedMatcher(plugins []plugin.Plugin) (*unscopedMatcher, error) {
	decls, err := plugin.CollectUnscopedDeclarations(plugins)
	if err != nil {
		return nil, err
	}
	m := &unscopedMatcher{exact: make(map[string]bool)}
	for _, d := range decls {
		if strings.HasSuffix(d.Path, "/") {
			m.prefixes = append(m.prefixes, d.Path)
			continue
		}
		m.exact[d.Path] = true
	}
	return m, nil
}

func (m *unscopedMatcher) Matches(path string) bool {
	if m == nil {
		return false
	}
	if m.exact[path] {
		return true
	}
	for _, prefix := range m.prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func unscopedDeclarationMatches(decl, path string) bool {
	if strings.HasSuffix(decl, "/") {
		return strings.HasPrefix(path, decl)
	}
	return path == decl
}

func (a *App) pluginGate(apiHandler interface{ PluginEnabled(uint, string) bool }, unscoped *unscopedMatcher) func(*http.Request, string) (allowed, scoped bool) {
	return func(r *http.Request, featureKey string) (allowed, scoped bool) {
		if r == nil {
			return false, false
		}
		if isPlatformReservedPath(r.URL.Path) {
			return false, false
		}
		if unscoped.Matches(r.URL.Path) {
			return false, false
		}
		oid := a.auth.OrgID(r)
		if oid == 0 {
			return false, false
		}
		return apiHandler.PluginEnabled(oid, featureKey), true
	}
}

type recoverWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (rw *recoverWriter) WriteHeader(code int) {
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recoverWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *recoverWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type gatedMux struct {
	real    muxSink
	plugin  string
	enabled func(r *http.Request, plugin string) (allowed, scoped bool)
}

func (g *gatedMux) Handle(pattern string, h http.Handler) {
	g.real.Handle(pattern, g.wrap(h))
}

func (g *gatedMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	g.real.Handle(pattern, g.wrap(http.HandlerFunc(h)))
}

func (g *gatedMux) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed, scoped := g.enabled(r, g.plugin); scoped && !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"plugin not enabled for this workspace"}`))
			return
		}
		rw := &recoverWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("plugin handler panic",
					"plugin", g.plugin,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				if !rw.wroteHeader {
					rw.ResponseWriter.Header().Set("Content-Type", "application/json")
					rw.ResponseWriter.WriteHeader(http.StatusInternalServerError)
					_, _ = rw.ResponseWriter.Write([]byte(`{"error":"internal server error"}`))
				}
			}
		}()
		h.ServeHTTP(rw, r)
	})
}

type gatedAPI struct {
	huma.API
	gAdapter huma.Adapter
}

func (g *gatedAPI) Adapter() huma.Adapter {
	return g.gAdapter
}

type recordingAPI struct {
	huma.API
	rAdapter huma.Adapter
}

func (r *recordingAPI) Adapter() huma.Adapter { return r.rAdapter }

type recordingAdapter struct {
	huma.Adapter
	routes *routeRegistry
	owner  string
}

func (r *recordingAdapter) Handle(op *huma.Operation, handler func(ctx huma.Context)) {
	if r.routes != nil && !r.routes.claim(r.owner, op.Method+" "+op.Path, false) {
		return
	}
	r.Adapter.Handle(op, handler)
}

type gatedAdapter struct {
	huma.Adapter
	plugin     string
	enabled    func(r *http.Request, plugin string) (allowed, scoped bool)
	routes     *routeRegistry
	owner      string
	thirdParty bool
}

func (g *gatedAdapter) Handle(op *huma.Operation, handler func(ctx huma.Context)) {
	if g.routes != nil && !g.routes.claim(g.owner, op.Method+" "+op.Path, g.thirdParty) {
		return
	}
	g.Adapter.Handle(op, func(ctx huma.Context) {
		r, w := humago.Unwrap(ctx)
		if allowed, scoped := g.enabled(r, g.plugin); scoped && !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"plugin not enabled for this workspace"}`))
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("plugin huma handler panic",
					"plugin", g.plugin,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		handler(ctx)
	})
}
