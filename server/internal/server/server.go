package server

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/origin"
	"gorm.io/gorm"
)

type StaticMount struct {
	Prefix string
	FS     fs.FS
}

type preparedMount struct {
	prefix  string
	handler http.Handler
	idx     []byte
	assets  fs.FS
}

type Server struct {
	cfg          *config.Config
	api          http.Handler
	rootFallback http.Handler
	static       http.Handler
	spaIdx       []byte
	assets       fs.FS
	devProxy     *httputil.ReverseProxy
	mounts       []preparedMount
	mw           *middleware
	origins      *origin.Resolver
}

func New(cfg *config.Config, db *gorm.DB, apiHandler http.Handler, rootFallback http.Handler, webFS fs.FS, mounts []StaticMount, rs RuntimeSettings) (*Server, error) {
	var devProxy *httputil.ReverseProxy
	if cfg != nil && cfg.DevWebProxy != "" {
		target, err := url.Parse(cfg.DevWebProxy)
		if err != nil || target.Scheme == "" || target.Host == "" {
			return nil, fmt.Errorf("invalid OCTARQ_DEV_WEB_PROXY URL %q: scheme and host required", cfg.DevWebProxy)
		}
		devProxy = &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.SetXForwarded()
			},
		}
		devProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("dev web proxy connection error", "target", target.String(), "path", r.URL.Path, "err", err)
			http.Error(w, fmt.Sprintf("OCTARQ_DEV_WEB_PROXY failed to connect to frontend dev server (%s): %v\nMake sure your frontend dev server is running (e.g. 'cd web && pnpm dev')", target.String(), err), http.StatusBadGateway)
		}
	}

	var idx []byte
	if devProxy == nil {
		if webFS == nil {
			return nil, fmt.Errorf("webFS is nil")
		}
		var err error
		idx, err = fs.ReadFile(webFS, "index.html")
		if err != nil {
			return nil, err
		}
	} else if webFS != nil {
		if iv, err := fs.ReadFile(webFS, "index.html"); err == nil {
			idx = iv
		} else {
			slog.Warn("dev web proxy: could not read index.html", "err", err)
		}
	}

	var staticHandler http.Handler
	if webFS != nil {
		staticHandler = http.StripPrefix("/admin/", http.FileServer(http.FS(webFS)))
	}

	if cfg != nil {
		trustProxy = cfg.TrustProxy
	}
	s := &Server{
		cfg:          cfg,
		api:          apiHandler,
		rootFallback: rootFallback,
		static:       staticHandler,
		spaIdx:       idx,
		assets:       webFS,
		devProxy:     devProxy,
		mw:           newMiddleware(rs),
		origins:      origin.NewResolver(db),
	}

	if s.mw.metricsEnabled() && s.mw.currentMetricsToken() == "" {
		slog.Warn("metrics are off: set OCTARQ_METRICS_TOKEN or the instance metrics token to serve /metrics")
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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mw.handle(w, r, s.route)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if strings.HasPrefix(path, "/api/") {
		s.api.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(path, "/auth/") {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		s.api.ServeHTTP(w, r)
		return
	}

	if isSpecPath(path) {
		s.api.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(path, "/v1/") {
		s.api.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(path, "/v1beta/") {
		s.api.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(path, "/.well-known/") {
		s.api.ServeHTTP(w, r)
		return
	}

	if path == "/robots.txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
		return
	}

	if path == "/admin" || strings.HasPrefix(path, "/admin/") {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		if s.devProxy != nil {
			s.devProxy.ServeHTTP(w, r)
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

	if path == "/status" || path == "/status/" {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		if s.devProxy != nil {
			s.devProxy.ServeHTTP(w, r)
			return
		}
		s.serveIndex(w)
		return
	}

	if path == "/instance" || strings.HasPrefix(path, "/instance/") {
		if !s.dashboardAllowed(r.Host) {
			http.NotFound(w, r)
			return
		}
		if s.devProxy != nil {
			s.devProxy.ServeHTTP(w, r)
			return
		}
		s.serveIndex(w)
		return
	}

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

	if path == "/" {
		if s.dashboardAllowed(r.Host) {
			http.Redirect(w, r, "/admin/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}

	if s.rootFallback != nil {
		s.rootFallback.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

var specPaths = map[string]bool{
	"/openapi.json":     true,
	"/openapi.yaml":     true,
	"/openapi-3.0.json": true,
	"/openapi-3.0.yaml": true,
	"/docs":             true,
}

func isSpecPath(path string) bool {
	if specPaths[path] {
		return true
	}
	return strings.HasPrefix(path, "/schemas/")
}

func (s *Server) dashboardAllowed(host string) bool {
	return !s.origins.ServesTraffic(host)
}

func (s *Server) assetExists(name string) bool {
	if s.assets == nil {
		return false
	}
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
