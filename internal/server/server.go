// Package server wires the API, short-link redirector, and embedded SPA
// behind a single http.Handler.
//
// Routing:
//   - /api/*    → JSON API
//   - /admin/*  → embedded React dashboard (assets + SPA fallback)
//   - /status   → public status page (same SPA, gated by dashboardAllowed)
//   - /instance → instance operator console (same SPA, gated by dashboardAllowed)
//   - /         → redirect to /admin/
//   - /{slug}   → short-link redirect (the root namespace belongs to links)
package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/origin"
	"gorm.io/gorm"
)

// StaticMount is an embedded single-page app served under Prefix (e.g.
// "/portal") on behalf of a plugin — the backing for plugin.Context.HandleStatic.
// FS is the built dist directory and must contain index.html. The core no
// longer knows anything about the buyer portal specifically; a plugin (in a
// composed edition) registers its own frontend here, and an OSS build that
// composes no such plugin serves nothing under the prefix.
type StaticMount struct {
	Prefix string // absolute path prefix, no trailing slash (e.g. "/portal")
	FS     fs.FS  // built dist dir; must contain index.html
}

// preparedMount is a StaticMount resolved into the handlers Server serves.
type preparedMount struct {
	prefix  string
	handler http.Handler
	idx     []byte
	assets  fs.FS
}

// Server is the top-level HTTP handler.
type Server struct {
	cfg          *config.Config
	api          http.Handler
	rootFallback http.Handler
	static       http.Handler
	spaIdx       []byte
	assets       fs.FS
	mounts       []preparedMount
	mw           *middleware
	origins      *origin.Resolver
}

// New builds the combined handler. webFS is the embedded dist directory.
// db backs the host lookup that decides where the dashboard is served.
// mounts are plugin-contributed static SPAs (plugin.Context.HandleStatic),
// each served under its own path prefix. rs supplies the DB-backed runtime
// settings for the edge middleware (rate limits, metrics token); zero value =
// built-in defaults.
func New(cfg *config.Config, db *gorm.DB, apiHandler http.Handler, rootFallback http.Handler, webFS fs.FS, mounts []StaticMount, rs RuntimeSettings) (*Server, error) {
	idx, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		return nil, err
	}
	trustProxy = cfg.TrustProxy
	s := &Server{
		cfg:          cfg,
		api:          apiHandler,
		rootFallback: rootFallback,
		static:       http.StripPrefix("/admin/", http.FileServer(http.FS(webFS))),
		spaIdx:       idx,
		assets:       webFS,
		mw:           newMiddleware(rs),
		origins:      origin.NewResolver(db),
	}

	for _, m := range mounts {
		mIdx, err := fs.ReadFile(m.FS, "index.html")
		if err != nil {
			return nil, err
		}
		s.mounts = append(s.mounts, preparedMount{
			prefix:  m.Prefix,
			handler: http.StripPrefix(m.Prefix+"/", http.FileServer(http.FS(m.FS))),
			idx:     mIdx,
			assets:  m.FS,
		})
	}

	return s, nil
}

// ServeHTTP applies the edge middleware (request IDs, rate limiting, metrics,
// access logging) and then dispatches to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mw.handle(w, r, s.route)
}

// route performs the actual path-based dispatch.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 1. API and inbound webhook.
	if strings.HasPrefix(path, "/api/") {
		s.api.ServeHTTP(w, r)
		return
	}

	// 1.5 The API's own generated reference. huma registers these on the API
	// mux from the real handler registrations, so this document cannot drift
	// from what the instance serves — it IS what the instance serves.
	//
	// They live outside /api/ because huma puts them there, and without this
	// branch every one of them fell through to the short-link router and 404ed:
	// the one spec that cannot go stale was the one nobody could fetch. The
	// published website artifact is generated from this same document.
	//
	// Deliberately reachable without a session: an API reference a developer
	// must first log in to read is a reference they will not read, and it
	// describes only the shape of the API, never any workspace's data.
	if isSpecPath(path) {
		s.api.ServeHTTP(w, r)
		return
	}

	// 2. Dashboard SPA under /admin (gated by dashboardAllowed).
	if path == "/admin" || strings.HasPrefix(path, "/admin/") {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(path, "/admin"), "/")
		if rest != "" && s.assetExists(rest) {
			s.static.ServeHTTP(w, r)
			return
		}
		s.serveIndex(w)
		return
	}

	// 2.25 Public status page. Serves the same SPA index.html — the React
	// client reads window.location.pathname and renders StatusPage when it
	// sees /status. Assets load from /admin/assets/ (the Vite base) so
	// nothing extra is needed. Gated by dashboardAllowed like the admin
	// console: a host registered for short links / mail has no reason to
	// expose the instance's operational status.
	if path == "/status" || path == "/status/" {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		s.serveIndex(w)
		return
	}

	// 2.3 Instance operator console. Same SPA index.html again — the router
	// picks its basename from the path (main.tsx) and renders the console.
	// It lives OUTSIDE /admin on purpose: operating the deployment is not a
	// workspace activity, and the two surfaces should not read as one.
	// Serving it here also keeps /instance out of the root namespace, where
	// an unrouted path is looked up as a short-link slug.
	// Authorisation is the API's job — every /api/instance/* endpoint requires
	// an instance admin; this only decides which HTML the browser gets.
	if path == "/instance" || strings.HasPrefix(path, "/instance/") {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		s.serveIndex(w)
		return
	}

	// 2.5 Plugin-contributed static SPAs (e.g. the buyer portal under /portal),
	// registered via plugin.Context.HandleStatic. Each serves an asset when it
	// exists and otherwise its own index.html for client-side routing. An OSS
	// build composes no such plugin, so this loop is empty and the prefix falls
	// through to the root namespace (404).
	for i := range s.mounts {
		m := &s.mounts[i]
		if path == m.prefix || strings.HasPrefix(path, m.prefix+"/") {
			rest := strings.TrimPrefix(strings.TrimPrefix(path, m.prefix), "/")
			if rest != "" && mountAssetExists(m.assets, rest) {
				m.handler.ServeHTTP(w, r)
				return
			}
			serveHTMLIndex(w, m.idx)
			return
		}
	}

	// 2.75 Marketing-entry shortcuts to the dashboard's auth views. The
	// marketing site links these directly; both are gated by the same
	// dashboardAllowed check as /admin, so a link/mail host can't be used to
	// reach the login or register forms. Query strings pass through so a
	// campaign param (?plan=pro) survives to the client.
	switch path {
	case "/signup":
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		target := "/admin/?mode=register"
		if q := r.URL.RawQuery; q != "" {
			target += "&" + q
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	case "/login":
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		target := "/admin/"
		if q := r.URL.RawQuery; q != "" {
			target += "?" + q
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	// 3. Root → dashboard.
	if path == "/" {
		if s.dashboardAllowed(r.Host) {
			http.Redirect(w, r, "/admin/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}

	// 4. Everything else in the root namespace is handled by the rootFallback handler (if any).
	if s.rootFallback != nil {
		s.rootFallback.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

// specPaths are the documentation endpoints huma registers on the API mux (see
// huma.DefaultConfig: OpenAPIPath "/openapi", DocsPath "/docs", SchemasPath
// "/schemas"). Listing them exactly — rather than prefix-matching — keeps the
// root namespace intact: /openapi-notes stays a short-link slug.
var specPaths = map[string]bool{
	"/openapi.json":     true,
	"/openapi.yaml":     true,
	"/openapi-3.0.json": true,
	"/openapi-3.0.yaml": true,
	"/docs":             true,
}

// isSpecPath reports whether path is one of the generated reference endpoints.
func isSpecPath(path string) bool {
	if specPaths[path] {
		return true
	}
	// Individual JSON Schemas the document $refs into. Without these the
	// reference renders with dangling references.
	return strings.HasPrefix(path, "/schemas/")
}

// dashboardAllowed reports whether the dashboard may be served for this host.
//
// A hostname a workspace registered for short links or for mail exists to serve
// that workspace's public traffic; the operator console has no business
// answering there, and hiding it removes a login form from a domain whose
// visitors have no reason to see one. Every other hostname serves the
// dashboard.
//
// This is the rule OCTARQ_ADMIN_HOST's documentation always claimed ("empty =
// serve dashboard on any non-link host") but never implemented — unset, it
// served the dashboard everywhere. The domains table is the better source
// anyway: an instance legitimately answers on several hostnames for different
// purposes, which one admin host could not express.
func (s *Server) dashboardAllowed(host string) bool {
	return !s.origins.ServesTraffic(host)
}

func (s *Server) assetExists(name string) bool {
	f, err := s.assets.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.IsDir() {
		return false
	}
	return true
}

// mountAssetExists reports whether name resolves to a (non-directory) file in a
// plugin static mount's FS.
func mountAssetExists(assets fs.FS, name string) bool {
	if assets == nil {
		return false
	}
	f, err := assets.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.IsDir() {
		return false
	}
	return true
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	serveHTMLIndex(w, s.spaIdx)
}

func serveHTMLIndex(w http.ResponseWriter, idx []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, strings.NewReader(string(idx)))
}
