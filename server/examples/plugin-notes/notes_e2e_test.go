package notes_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/octarq-org/octarq/server/examples/plugin-notes"
	"github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/builtin"
	ontologyplugin "github.com/octarq-org/octarq/server/plugins/ontology"
)

// These tests boot the example the way a host does — the ontology plugin, then
// the module — and drive the surfaces the declaration claims. They are also the
// template a module author copies: every call here is exported API.

// fixture is the caller identity the middleware hands to every request. Changing
// orgID switches workspaces mid-test, which is how the isolation assertion works.
type fixture struct {
	server *httptest.Server
	orgID  uint
	userID uint
	role   string
}

func newFixture(t *testing.T) (*fixture, *gorm.DB) {
	t.Helper()
	// The operator's declaration file must not contribute entities here: this
	// test is about the one the package registered.
	t.Setenv(ontologyplugin.EntitiesEnv, "")
	t.Setenv(ontologyplugin.EntitiesFileEnv, "")

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notes.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ontology.AppliedSpec{}); err != nil {
		t.Fatalf("migrate bookkeeping: %v", err)
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Notes Example Test", "1.0.0"))

	f := &fixture{orgID: 1, userID: 1, role: "member"}
	// The identity has to be on the context before the plugin registers its
	// routes: huma bakes the middleware chain into an operation at registration.
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ctx := plugin.WithOrgID(plugin.WithTokenRole(context.Background(), f.role), f.orgID)
		next(huma.WithContext(c, plugin.WithUserID(ctx, f.userID)))
	})

	host := &plugin.TestHost{SessionMock: &plugin.TestSession{
		OrgIDFn:   func(*http.Request) uint { return f.orgID },
		UserIDFn:  func(*http.Request) uint { return f.userID },
		OrgRoleFn: func(*http.Request) string { return f.role },
	}}
	wiring := &plugin.Context{Huma: api, DB: db, Host: host}

	var mounted bool
	for _, p := range builtin.Default() {
		if p.Name() != "ontology" {
			continue
		}
		p.Mount(nil, wiring)
		validator, ok := p.(plugin.Validator)
		if !ok {
			t.Fatal("the ontology plugin is no longer a plugin.Validator")
		}
		if err := validator.Validate(context.Background(), host); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		mounted = true
	}
	if !mounted {
		t.Fatal("builtin.Default() no longer carries the ontology plugin")
	}

	notes.Plugin{}.Mount(nil, wiring)

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f, db
}

// do issues one request and returns the status and the decoded JSON body.
func (f *fixture) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

// TestDeclaredEntityIsServed walks the whole surface the declaration produces:
// the table, the catalog entry, create, list, and the workspace boundary.
func TestDeclaredEntityIsServed(t *testing.T) {
	f, db := newFixture(t)

	if !db.Migrator().HasTable(ontology.TableName("notes")) {
		t.Fatal("the declaration did not create its table")
	}

	status, catalog := f.do(t, http.MethodGet, "/api/data", "")
	if status != http.StatusOK {
		t.Fatalf("catalog = %d %v", status, catalog)
	}
	if !catalogLists(catalog, "notes") {
		t.Fatalf("catalog does not list notes: %v", catalog)
	}

	status, created := f.do(t, http.MethodPost, "/api/data/notes", `{"title":"First note"}`)
	t.Logf("POST /api/data/notes -> %d", status)
	if status != http.StatusOK {
		t.Fatalf("create = %d %v", status, created)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("create returned no id: %v", created)
	}

	// title is declared unique: the platform refuses the second row rather than
	// storing a copy.
	status, dup := f.do(t, http.MethodPost, "/api/data/notes", `{"title":"First note"}`)
	t.Logf("POST duplicate title -> %d", status)
	if status != http.StatusConflict {
		t.Fatalf("duplicate = %d %v, want 409", status, dup)
	}

	status, list := f.do(t, http.MethodGet, "/api/data/notes", "")
	if status != http.StatusOK {
		t.Fatalf("list = %d %v", status, list)
	}
	if total, _ := list["total"].(float64); total != 1 {
		t.Fatalf("total = %v, want 1", list["total"])
	}

	// A second workspace sees none of it: the handle binds org_id, the module
	// never filters by it.
	f.orgID = 2
	status, other := f.do(t, http.MethodGet, "/api/data/notes", "")
	if status != http.StatusOK {
		t.Fatalf("list in org 2 = %d %v", status, other)
	}
	if total, _ := other["total"].(float64); total != 0 {
		t.Fatalf("org 2 total = %v, want 0", other["total"])
	}
	status, _ = f.do(t, http.MethodGet, "/api/data/notes/"+id, "")
	t.Logf("org 2 GET one -> %d", status)
	if status != http.StatusNotFound {
		t.Fatalf("org 2 read of org 1 row = %d, want 404", status)
	}
}

// catalogLists reports whether the catalog body carries the slug. The body is
// {"entities":[{...}]}, decoded generically because the test is external.
func catalogLists(body map[string]any, slug string) bool {
	entities, _ := body["entities"].([]any)
	for _, raw := range entities {
		if entry, ok := raw.(map[string]any); ok && entry["slug"] == slug {
			return true
		}
	}
	return false
}
