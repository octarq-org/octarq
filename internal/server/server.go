// Package server wires the API, short-link redirector, and embedded SPA
// behind a single http.Handler with host-aware routing.
package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/jungley/led/config"
	"github.com/jungley/led/internal/shortlink"
)

// Server is the top-level HTTP handler.
type Server struct {
	cfg    *config.Config
	api    http.Handler
	short  *shortlink.Service
	static http.Handler
	spaIdx []byte
	assets fs.FS
}

// New builds the combined handler. webFS is the embedded dist directory.
func New(cfg *config.Config, apiHandler http.Handler, short *shortlink.Service, webFS fs.FS) (*Server, error) {
	idx, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:    cfg,
		api:    apiHandler,
		short:  short,
		static: http.FileServer(http.FS(webFS)),
		spaIdx: idx,
		assets: webFS,
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 1. API and inbound webhook.
	if strings.HasPrefix(path, "/api/") {
		s.api.ServeHTTP(w, r)
		return
	}

	// 2. Short-link redirect: single-segment GET that isn't the dashboard.
	if r.Method == http.MethodGet && isSlugPath(path) && !s.isAdminHost(r.Host) {
		slug := strings.TrimPrefix(path, "/")
		if link, ok := s.short.Lookup(r.Host, slug); ok {
			s.short.Handle(w, r, link)
			return
		}
		// fall through to SPA (e.g. a 404 page) if no slug matches
	}

	// 3. Static asset if it exists, else SPA index (client-side routing).
	if s.assetExists(path) {
		s.static.ServeHTTP(w, r)
		return
	}
	s.serveIndex(w)
}

// isAdminHost reports whether the request targets the dashboard host. When
// LED_ADMIN_HOST is unset, every host may serve the dashboard, so slugs are
// still tried first (lookup miss falls through to the SPA).
func (s *Server) isAdminHost(host string) bool {
	if s.cfg.AdminHost == "" {
		return false
	}
	return stripPort(host) == s.cfg.AdminHost
}

// isSlugPath matches "/something" with no further slashes and no file extension.
func isSlugPath(p string) bool {
	if p == "/" || !strings.HasPrefix(p, "/") {
		return false
	}
	rest := p[1:]
	if rest == "" || strings.Contains(rest, "/") {
		return false
	}
	// Skip obvious static files and reserved SPA roots.
	if strings.Contains(rest, ".") {
		return false
	}
	switch rest {
	case "assets", "login", "overview", "links", "domains", "mail", "settings", "favicon.ico":
		return false
	}
	return true
}

func (s *Server) assetExists(path string) bool {
	if path == "/" {
		return false
	}
	name := strings.TrimPrefix(path, "/")
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

func (s *Server) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, strings.NewReader(string(s.spaIdx)))
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}
