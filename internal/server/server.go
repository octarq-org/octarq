// Package server wires the API, short-link redirector, and embedded SPA
// behind a single http.Handler.
//
// Routing:
//   - /api/*    → JSON API
//   - /admin/*  → embedded React dashboard (assets + SPA fallback)
//   - /         → redirect to /admin/
//   - /{slug}   → short-link redirect (the root namespace belongs to links)
package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/config"
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
}

// New builds the combined handler. webFS is the embedded dist directory.
// mounts are plugin-contributed static SPAs (plugin.Context.HandleStatic),
// each served under its own path prefix. rs supplies the DB-backed runtime
// settings for the edge middleware (rate limits, metrics token); zero value =
// built-in defaults.
func New(cfg *config.Config, apiHandler http.Handler, rootFallback http.Handler, webFS fs.FS, mounts []StaticMount, rs RuntimeSettings) (*Server, error) {
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

	// 2. Dashboard SPA under /admin (gated to the admin host when configured).
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

// dashboardAllowed reports whether the dashboard may be served for this host.
// When OCTARQ_ADMIN_HOST is set, the dashboard is restricted to that host so pure
// link hosts don't expose it; otherwise it is served on any host.
func (s *Server) dashboardAllowed(host string) bool {
	if s.cfg.AdminHost == "" {
		return true
	}
	return stripPort(host) == s.cfg.AdminHost
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

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}
