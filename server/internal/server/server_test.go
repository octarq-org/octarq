package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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

	reqAPI := httptest.NewRequest("GET", "/api/test", nil)
	recAPI := httptest.NewRecorder()
	srv.ServeHTTP(recAPI, reqAPI)
	if recAPI.Code != http.StatusOK || recAPI.Body.String() != "api response" {
		t.Errorf("api route failed: got %d %q", recAPI.Code, recAPI.Body.String())
	}

	reqOAuth := httptest.NewRequest("GET", "/auth/begin/google", nil)
	recOAuth := httptest.NewRecorder()
	srv.ServeHTTP(recOAuth, reqOAuth)
	if recOAuth.Code != http.StatusOK || recOAuth.Body.String() != "api response" {
		t.Errorf("oauth ingress route failed: got %d %q, want 200 api response", recOAuth.Code, recOAuth.Body.String())
	}

	reqOAuthBlocked := httptest.NewRequest("GET", "/auth/begin/google", nil)
	reqOAuthBlocked.Host = "links.example.com"
	recOAuthBlocked := httptest.NewRecorder()
	srv.ServeHTTP(recOAuthBlocked, reqOAuthBlocked)
	if recOAuthBlocked.Code != http.StatusNotFound {
		t.Errorf("oauth route on traffic host: got %d, want 404", recOAuthBlocked.Code)
	}

	reqAdmin := httptest.NewRequest("GET", "/admin/", nil)
	reqAdmin.Host = "admin.example.com:8080"
	recAdmin := httptest.NewRecorder()
	srv.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusOK || recAdmin.Body.String() != "index html" {
		t.Errorf("admin index route failed: got %d %q", recAdmin.Code, recAdmin.Body.String())
	}

	reqAsset := httptest.NewRequest("GET", "/admin/asset.css", nil)
	reqAsset.Host = "admin.example.com"
	recAsset := httptest.NewRecorder()
	srv.ServeHTTP(recAsset, reqAsset)
	if recAsset.Code != http.StatusOK || !strings.Contains(recAsset.Body.String(), "css style") {
		t.Errorf("admin asset route failed: got %d %q", recAsset.Code, recAsset.Body.String())
	}

	reqNotAllowed := httptest.NewRequest("GET", "/admin/", nil)
	reqNotAllowed.Host = "links.example.com"
	recNotAllowed := httptest.NewRecorder()
	srv.ServeHTTP(recNotAllowed, reqNotAllowed)
	if recNotAllowed.Code != http.StatusNotFound {
		t.Errorf("expected 404 for disallowed admin, got %d", recNotAllowed.Code)
	}

	reqRoot := httptest.NewRequest("GET", "/", nil)
	reqRoot.Host = "admin.example.com"
	recRoot := httptest.NewRecorder()
	srv.ServeHTTP(recRoot, reqRoot)
	if recRoot.Code != http.StatusFound || recRoot.Header().Get("Location") != "/admin/" {
		t.Errorf("expected 302 to /admin/, got %d Location %q", recRoot.Code, recRoot.Header().Get("Location"))
	}

	reqRoot404 := httptest.NewRequest("GET", "/", nil)
	reqRoot404.Host = "links.example.com"
	recRoot404 := httptest.NewRecorder()
	srv.ServeHTTP(recRoot404, reqRoot404)
	if recRoot404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for root when not allowed, got %d", recRoot404.Code)
	}

	reqSignup := httptest.NewRequest("GET", "/signup", nil)
	reqSignup.Host = "admin.example.com"
	recSignup := httptest.NewRecorder()
	srv.ServeHTTP(recSignup, reqSignup)
	if recSignup.Code != http.StatusFound || recSignup.Header().Get("Location") != "/admin/?mode=register" {
		t.Errorf("GET /signup: got %d Location %q, want 302 /admin/?mode=register", recSignup.Code, recSignup.Header().Get("Location"))
	}

	reqLogin := httptest.NewRequest("GET", "/login", nil)
	reqLogin.Host = "admin.example.com"
	recLogin := httptest.NewRecorder()
	srv.ServeHTTP(recLogin, reqLogin)
	if recLogin.Code != http.StatusFound || recLogin.Header().Get("Location") != "/admin/" {
		t.Errorf("GET /login: got %d Location %q, want 302 /admin/", recLogin.Code, recLogin.Header().Get("Location"))
	}

	for _, path := range []string{"/signup", "/login"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = "links.example.com"
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s on disallowed host: got %d, want 404", path, rec.Code)
		}
	}

	for _, host := range []string{"admin.example.com", "links.example.com", "anydomain.com"} {
		reqRobots := httptest.NewRequest("GET", "/robots.txt", nil)
		reqRobots.Host = host
		recRobots := httptest.NewRecorder()
		srv.ServeHTTP(recRobots, reqRobots)
		if recRobots.Code != http.StatusOK {
			t.Errorf("GET /robots.txt on host %s: got %d, want 200", host, recRobots.Code)
		}
		if ct := recRobots.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
			t.Errorf("GET /robots.txt content-type: got %q, want text/plain", ct)
		}
		expectedBody := "User-agent: *\nDisallow: /\n"
		if recRobots.Body.String() != expectedBody {
			t.Errorf("GET /robots.txt body: got %q, want %q", recRobots.Body.String(), expectedBody)
		}
	}
}

func TestMarketingEntryForwardsQueryString(t *testing.T) {
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("index html")}}
	srv, err := New(&config.Config{}, domainsDB(t), mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	req := httptest.NewRequest("GET", "/signup?plan=pro&utm_source=launch", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin/?mode=register&plan=pro&utm_source=launch" {
		t.Errorf("GET /signup?plan=pro: got %d Location %q, want 302 with query forwarded", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest("GET", "/login?next=%2Fsettings", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin/?next=%2Fsettings" {
		t.Errorf("GET /login?next: got %d Location %q, want 302 with query forwarded", rec.Code, rec.Header().Get("Location"))
	}
}

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
		{"/portal", "portal index", http.StatusOK},
		{"/portal/subscriptions", "portal index", http.StatusOK},
		{"/portal/assets/app.js", "portal js", http.StatusOK},
		{"/nope", "", http.StatusNotFound},
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
		{host: "dashboard.example.net", serve: true},
		{host: "app.acme.example", serve: true},
		{host: "old.acme.example", serve: true},
		{host: "go.acme.example", serve: false},
		{host: "go.acme.example:8080", serve: false},
		{host: "GO.Acme.Example", serve: false},
		{host: "mx.mail.example", serve: false},
		{host: "short.example", serve: false},
		{host: "sub.short.example", serve: true},
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

func TestStatusPageServesIndex(t *testing.T) {
	const indexBody = "index html"
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(indexBody)}}
	srv, err := New(&config.Config{}, domainsDB(t), mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	for _, path := range []string{"/status", "/status/"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, rec.Code)
		}
		if body := rec.Body.String(); body != indexBody {
			t.Errorf("GET %s: body %q, want %q", path, body, indexBody)
		}
	}
}

func TestInstanceConsoleServesIndex(t *testing.T) {
	const indexBody = "index html"
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(indexBody)}}
	srv, err := New(&config.Config{}, domainsDB(t), mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	for _, path := range []string{"/instance", "/instance/", "/instance/wizard", "/instance/settings"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, rec.Code)
		}
		if body := rec.Body.String(); body != indexBody {
			t.Errorf("GET %s: body %q, want %q", path, body, indexBody)
		}
	}
}

func TestStatusPageDisallowedHost(t *testing.T) {
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("index html")}}
	db := domainsDB(t, testDomain{
		OrgID:     1,
		Name:      "example.com",
		ForLink:   true,
		LinkHosts: `[{"host":"links.example.com","enabled":true}]`,
	})
	srv, err := New(&config.Config{}, db, mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	req := httptest.NewRequest("GET", "/status", nil)
	req.Host = "links.example.com"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /status on disallowed host: got %d, want 404", rec.Code)
	}
}

func TestStatusTierFor(t *testing.T) {
	for _, path := range []string{"/status", "/status/"} {
		req := httptest.NewRequest("GET", path, nil)
		if got := tierFor(req); got != tierAPI {
			t.Errorf("tierFor(%q) = %d, want %d (tierAPI)", path, got, tierAPI)
		}
	}
}

func TestAgentAPIPathReachesTheAPIMux(t *testing.T) {
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("index html")}}
	srv, err := New(&config.Config{}, domainsDB(t), mockAPI{}, nil, webFS, nil, RuntimeSettings{})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	for _, path := range []string{"/v1/models", "/v1/chat/completions", "/v1/responses", "/.well-known/opencode",
		"/v1beta/models", "/v1beta/models/gemini-2.5-pro:generateContent"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "api response") {
			t.Fatalf("%s reached %d %q, want the API mux", path, rec.Code, rec.Body.String())
		}
	}
}
