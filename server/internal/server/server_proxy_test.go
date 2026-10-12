package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/config"
)

func TestDevWebProxy(t *testing.T) {
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Dev-Proxy", "active")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxied: " + r.URL.Path))
	}))
	defer frontend.Close()

	cfg := &config.Config{
		DevWebProxy: frontend.URL,
	}
	db := domainsDB(t)
	srv, err := New(cfg, db, mockAPI{}, nil, nil, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server with dev proxy: %v", err)
	}

	tests := []struct {
		name       string
		path       string
		wantCode   int
		wantBody   string
		wantHeader bool
	}{
		{
			name:       "admin root",
			path:       "/admin/",
			wantCode:   http.StatusOK,
			wantBody:   "proxied: /admin/",
			wantHeader: true,
		},
		{
			name:       "admin subpath",
			path:       "/admin/dashboard",
			wantCode:   http.StatusOK,
			wantBody:   "proxied: /admin/dashboard",
			wantHeader: true,
		},
		{
			name:       "status page",
			path:       "/status",
			wantCode:   http.StatusOK,
			wantBody:   "proxied: /status",
			wantHeader: true,
		},
		{
			name:       "instance console",
			path:       "/instance/nodes",
			wantCode:   http.StatusOK,
			wantBody:   "proxied: /instance/nodes",
			wantHeader: true,
		},
		{
			name:       "api endpoint not proxied",
			path:       "/api/test",
			wantCode:   http.StatusOK,
			wantBody:   "api response",
			wantHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("path %s: code = %d, want %d", tt.path, rec.Code, tt.wantCode)
			}
			if rec.Body.String() != tt.wantBody {
				t.Errorf("path %s: body = %q, want %q", tt.path, rec.Body.String(), tt.wantBody)
			}
			hasHeader := rec.Header().Get("X-Dev-Proxy") == "active"
			if hasHeader != tt.wantHeader {
				t.Errorf("path %s: X-Dev-Proxy header = %v, want %v", tt.path, hasHeader, tt.wantHeader)
			}
		})
	}
}
