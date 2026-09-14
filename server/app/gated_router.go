package app

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// isNonWorkspaceRoute reports whether a path represents an instance-level administration
// route or a public/entrypoint route that is not scoped to a specific workspace's plugin toggle.
func isNonWorkspaceRoute(path string) bool {
	if path == "/api/instance-settings" || path == "/api/instance/link-settings" || path == "/instance/link-settings" {
		return true
	}
	if strings.Contains(path, "/instance/") || strings.Contains(path, "/storage-instance/") || strings.HasPrefix(path, "/api/instance/") || strings.HasPrefix(path, "/instance/") {
		return true
	}
	if strings.HasPrefix(path, "/api/sso/") {
		if path == "/api/sso/login" || strings.HasSuffix(path, "/login") || strings.HasSuffix(path, "/callback") {
			return true
		}
	}
	if strings.HasPrefix(path, "/api/customer/") ||
		strings.HasPrefix(path, "/api/portal/") ||
		strings.HasPrefix(path, "/api/storefront") ||
		strings.HasPrefix(path, "/api/delivery/") ||
		strings.HasPrefix(path, "/api/updates/") ||
		strings.HasPrefix(path, "/api/webhook/") ||
		strings.HasPrefix(path, "/api/license") ||
		strings.HasPrefix(path, "/api/update/") {
		return true
	}
	return false
}

// pluginGate builds the per-workspace feature check the gated mux and adapter
// share. Non-core plugin routes answer 404 when the caller's workspace has the
// feature disabled.
//
// The org-0 case returns scoped=false, i.e. "not a workspace-scoped request, do
// not gate it" — the route is then served. That is deliberate and load-bearing:
// public plugin routes (payment webhooks, the buyer portal, license activation)
// carry no session, so there is no workspace whose toggle could be consulted.
// Returning scoped=true here to look "fail-closed" would 404 every incoming
// Stripe webhook and every buyer — payments would stop, quietly, and the failure
// would surface as missing revenue rather than as an error.
//
// Instance-level administration routes (e.g. /api/sso/instance/*) and public
// entrypoints also return scoped=false because they are deployment-wide or
// host trust decisions rather than properties of the caller's current workspace.
func (a *App) pluginGate(apiHandler interface{ PluginEnabled(uint, string) bool }) func(*http.Request, string) (allowed, scoped bool) {
	return func(r *http.Request, featureKey string) (allowed, scoped bool) {
		if r == nil {
			return false, false
		}
		if isNonWorkspaceRoute(r.URL.Path) {
			return false, false
		}
		oid := a.auth.OrgID(r)
		if oid == 0 {
			return false, false // no workspace in session (webhooks, portal) → not gated
		}
		return apiHandler.PluginEnabled(oid, featureKey), true
	}
}

// recoverWriter wraps http.ResponseWriter to track whether response headers
// have already been written. This lets the panic-recovery path avoid a
// superfluous WriteHeader call (and the warning log it would produce) when a
// handler panics after partially writing the response.
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

// Flush forwards to the underlying ResponseWriter if it implements
// http.Flusher, keeping SSE / streaming responses working.
func (rw *recoverWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// gatedMux wraps the shared API mux so every route a plugin registers is guarded
// by a per-workspace "plugin enabled" check. It satisfies plugin.Mux; when the
// caller's workspace has the plugin disabled, the wrapped handler answers 404
// before running. Requests with no workspace in session (public plugin routes
// such as payment webhooks and the customer portal) pass through unchanged —
// they aren't workspace-scoped and can't be org-gated here.
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

// recordingAPI wraps the shared huma API for a CORE plugin: core routes are not
// feature-gated, but they still have to be checked for collisions — a core
// plugin and a Pro module claiming the same path panics exactly the same way.
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
	plugin  string
	enabled func(r *http.Request, plugin string) (allowed, scoped bool)
	// routes is the same collision guard the mux wrapper uses: huma routes end
	// up on the same ServeMux and panic on a duplicate just the same.
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
				// Best-effort 500 response. If the handler already wrote
				// headers through the huma context, WriteHeader is a no-op
				// with a harmless "superfluous" log — acceptable on a panic
				// path that is already exceptional.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		handler(ctx)
	})
}
