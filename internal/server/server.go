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
	"github.com/octarq-org/octarq/internal/shortlink"
)

// Server is the top-level HTTP handler.
type Server struct {
	cfg          *config.Config
	api          http.Handler
	short        *shortlink.Service
	static       http.Handler
	spaIdx       []byte
	assets       fs.FS
	portalStatic http.Handler
	portalIdx    []byte
	portalAssets fs.FS
	mw           *middleware
}

// New builds the combined handler. webFS is the embedded dist directory.
// rs supplies the DB-backed runtime settings for the edge middleware (rate
// limits, metrics token); zero value = built-in defaults.
func New(cfg *config.Config, apiHandler http.Handler, short *shortlink.Service, webFS fs.FS, rs RuntimeSettings) (*Server, error) {
	idx, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		return nil, err
	}
	trustProxy = cfg.TrustProxy
	s := &Server{
		cfg:    cfg,
		api:    apiHandler,
		short:  short,
		static: http.StripPrefix("/admin/", http.FileServer(http.FS(webFS))),
		spaIdx: idx,
		assets: webFS,
		mw:     newMiddleware(rs),
	}

	pSub, err := fs.Sub(webFS, "portal")
	if err == nil {
		if pIdx, perr := fs.ReadFile(pSub, "index.html"); perr == nil {
			s.portalStatic = http.StripPrefix("/portal/", http.FileServer(http.FS(pSub)))
			s.portalIdx = pIdx
			s.portalAssets = pSub
		}
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

	// 2.5 Customer Portal SPA under /portal
	if path == "/portal" || strings.HasPrefix(path, "/portal/") {
		if s.portalStatic == nil {
			http.Error(w, "Portal frontend is not built or available", http.StatusNotFound)
			return
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(path, "/portal"), "/")
		if rest != "" && s.portalAssetExists(rest) {
			s.portalStatic.ServeHTTP(w, r)
			return
		}
		s.servePortalIndex(w)
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

	// 4. Everything else in the root namespace is a short link.
	if r.Method == http.MethodGet {
		slug := strings.TrimPrefix(path, "/")
		if slug != "" && !strings.Contains(slug, "/") {
			if link, ok := s.short.Lookup(r.Host, slug); ok {
				s.short.Handle(w, r, link)
				return
			}
		}
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

func (s *Server) portalAssetExists(name string) bool {
	if s.portalAssets == nil {
		return false
	}
	f, err := s.portalAssets.Open(name)
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, strings.NewReader(string(s.spaIdx)))
}

func (s *Server) servePortalIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, strings.NewReader(string(s.portalIdx)))
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}
