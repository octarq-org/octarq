package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDomain mirrors the dns plugin's domains table, which is what now decides
// where the dashboard is served (see dashboardAllowed). The server package
// cannot import the plugin, and neither can this test.
type testDomain struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Name      string
	ForLink   bool
	ForMail   bool
	LinkHosts string
	MailHosts string
}

func (testDomain) TableName() string { return "domains" }

// domainsDB returns a database whose domains table contains rows, or no
// domains table at all when none are given (an instance composed without the
// dns plugin).
func domainsDB(t *testing.T, rows ...testDomain) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if len(rows) == 0 {
		return db
	}
	if err := db.AutoMigrate(&testDomain{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed domain: %v", err)
		}
	}
	return db
}

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

	// links.example.com serves this org's short links, so it must not serve the
	// dashboard. admin.example.com is registered by nobody and therefore does.
	db := domainsDB(t, testDomain{
		OrgID:     1,
		Name:      "example.com",
		ForLink:   true,
		LinkHosts: `[{"host":"links.example.com","enabled":true}]`,
	})

	srv, err := New(&config.Config{}, db, mockAPI{}, nil, webFS, nil, RuntimeSettings{})
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
	srv, err := New(&config.Config{}, domainsDB(t), mockAPI{}, nil, webFS,
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

// TestDashboardHostFollowsRegisteredDomains pins where the operator console is
// served now that OCTARQ_ADMIN_HOST is gone.
//
// A hostname a workspace registered for short links or for mail exists to serve
// that workspace's public traffic; putting a login form on it hands visitors an
// unexpected credential prompt on a domain they associate with the tenant. Every
// other hostname serves the dashboard, which is what the old variable's
// documentation always claimed for its empty value but never did.
func TestDashboardHostFollowsRegisteredDomains(t *testing.T) {
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("index html")}}

	db := domainsDB(t,
		testDomain{
			OrgID:     1,
			Name:      "acme.example",
			ForLink:   true,
			LinkHosts: `[{"host":"go.acme.example","enabled":true},{"host":"old.acme.example","enabled":false}]`,
		},
		testDomain{
			OrgID:     2,
			Name:      "mail.example",
			ForMail:   true,
			MailHosts: `[{"host":"mx.mail.example","enabled":true}]`,
		},
		// Toggle on, no host list: the apex itself serves the short links.
		testDomain{OrgID: 3, Name: "short.example", ForLink: true},
	)
	srv, err := New(&config.Config{}, db, mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	cases := []struct {
		host  string
		serve bool
	}{
		{host: "dashboard.example.net", serve: true}, // registered by nobody
		{host: "app.acme.example", serve: true},      // a subdomain that serves nothing
		{host: "old.acme.example", serve: true},      // link host, but disabled
		{host: "go.acme.example", serve: false},      // serves short links
		{host: "go.acme.example:8080", serve: false}, // ...port and case included
		{host: "GO.Acme.Example", serve: false},      //
		{host: "mx.mail.example", serve: false},      // receives mail
		{host: "short.example", serve: false},        // apex serves its own links
		{host: "sub.short.example", serve: true},     // but only the apex does
	}
	for _, tc := range cases {
		req := httptest.NewRequest("GET", "/admin/", nil)
		req.Host = tc.host
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		got := rec.Code == http.StatusOK
		if got != tc.serve {
			t.Errorf("GET /admin/ on %q: code %d (serving=%v), want serving=%v", tc.host, rec.Code, got, tc.serve)
		}
	}
}
