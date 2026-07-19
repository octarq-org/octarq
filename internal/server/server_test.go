package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/octarq-org/octarq/config"
)

type mockAPI struct{}

func (mockAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("api response"))
}

func TestServer(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("index html")},
		"asset.css":  &fstest.MapFile{Data: []byte("css style")},
	}

	cfg := &config.Config{
		AdminHost: "admin.example.com",
	}

	srv, err := New(cfg, mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("expected no error building server, got %v", err)
	}

	// 1. API Route
	reqAPI := httptest.NewRequest("GET", "/api/test", nil)
	recAPI := httptest.NewRecorder()
	srv.ServeHTTP(recAPI, reqAPI)
	if recAPI.Code != http.StatusOK || recAPI.Body.String() != "api response" {
		t.Errorf("api route failed: got %d %q", recAPI.Code, recAPI.Body.String())
	}

	// 2. Admin index fallback (when allowed)
	reqAdmin := httptest.NewRequest("GET", "/admin/", nil)
	reqAdmin.Host = "admin.example.com:8080" // test with port
	recAdmin := httptest.NewRecorder()
	srv.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusOK || recAdmin.Body.String() != "index html" {
		t.Errorf("admin index route failed: got %d %q", recAdmin.Code, recAdmin.Body.String())
	}

	// 3. Admin asset route (when allowed)
	reqAsset := httptest.NewRequest("GET", "/admin/asset.css", nil)
	reqAsset.Host = "admin.example.com"
	recAsset := httptest.NewRecorder()
	srv.ServeHTTP(recAsset, reqAsset)
	if recAsset.Code != http.StatusOK || !strings.Contains(recAsset.Body.String(), "css style") {
		t.Errorf("admin asset route failed: got %d %q", recAsset.Code, recAsset.Body.String())
	}

	// 4. Admin not allowed
	reqNotAllowed := httptest.NewRequest("GET", "/admin/", nil)
	reqNotAllowed.Host = "links.example.com"
	recNotAllowed := httptest.NewRecorder()
	srv.ServeHTTP(recNotAllowed, reqNotAllowed)
	if recNotAllowed.Code != http.StatusNotFound {
		t.Errorf("expected 404 for disallowed admin, got %d", recNotAllowed.Code)
	}

	// 5. Root route redirects to /admin/ when allowed
	reqRoot := httptest.NewRequest("GET", "/", nil)
	reqRoot.Host = "admin.example.com"
	recRoot := httptest.NewRecorder()
	srv.ServeHTTP(recRoot, reqRoot)
	if recRoot.Code != http.StatusFound || recRoot.Header().Get("Location") != "/admin/" {
		t.Errorf("expected 302 to /admin/, got %d Location %q", recRoot.Code, recRoot.Header().Get("Location"))
	}

	// 6. Root route returns 404 when not allowed
	reqRoot404 := httptest.NewRequest("GET", "/", nil)
	reqRoot404.Host = "links.example.com"
	recRoot404 := httptest.NewRecorder()
	srv.ServeHTTP(recRoot404, reqRoot404)
	if recRoot404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for root when not allowed, got %d", recRoot404.Code)
	}
}

// TestStaticMounts exercises the plugin.Context.HandleStatic seam: a mounted SPA
// serves a real asset, falls back to its own index.html for unknown sub-paths
// (client-side routing), and an unmounted prefix 404s.
func TestStaticMounts(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("dash index")},
	}
	portalFS := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("portal index")},
		"assets/app.js": &fstest.MapFile{Data: []byte("portal js")},
	}
	srv, err := New(&config.Config{}, mockAPI{}, nil, webFS,
		[]StaticMount{{Prefix: "/portal", FS: portalFS}}, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	cases := []struct {
		path, wantBody string
		wantCode       int
	}{
		{"/portal", "portal index", http.StatusOK},               // prefix root → index
		{"/portal/subscriptions", "portal index", http.StatusOK}, // SPA route → index fallback
		{"/portal/assets/app.js", "portal js", http.StatusOK},    // real asset
		{"/nope", "", http.StatusNotFound},                       // unmounted prefix
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.wantCode {
			t.Errorf("%s: got code %d, want %d", c.path, rec.Code, c.wantCode)
		}
		if c.wantBody != "" && !strings.Contains(rec.Body.String(), c.wantBody) {
			t.Errorf("%s: body %q does not contain %q", c.path, rec.Body.String(), c.wantBody)
		}
	}
}
