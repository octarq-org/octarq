package notes_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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

type fixture struct {
	server    *httptest.Server
	orgID     uint
	userID    uint
	role      string
	featureOn atomic.Bool
	data      *ontologyplugin.Plugin
}

func newFixture(t *testing.T) (*fixture, *gorm.DB) {
	t.Helper()
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
	f.featureOn.Store(true)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ctx := plugin.WithOrgID(plugin.WithTokenRole(context.Background(), f.role), f.orgID)
		next(huma.WithContext(c, plugin.WithUserID(ctx, f.userID)))
	})

	host := &plugin.TestHost{SessionMock: &plugin.TestSession{
		OrgIDFn:   func(*http.Request) uint { return f.orgID },
		UserIDFn:  func(*http.Request) uint { return f.userID },
		OrgRoleFn: func(*http.Request) string { return f.role },
	}}
	composed := append(append([]plugin.Plugin{}, builtin.Default()...), notes.Plugin{})
	wiring := &plugin.Context{
		Huma:          api,
		DB:            db,
		Host:          host,
		ActivePlugins: func() []plugin.Plugin { return composed },
		FeatureActive: func(_ uint, key string) bool {
			return key != "notes" || f.featureOn.Load()
		},
	}

	var mounted bool
	for _, p := range composed {
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
		f.data, _ = p.(*ontologyplugin.Plugin)
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

func TestTurningThePluginOffRemovesItsSurface(t *testing.T) {
	f, _ := newFixture(t)

	status, created := f.do(t, http.MethodPost, "/api/data/notes", `{"title":"Before the switch"}`)
	if status != http.StatusOK {
		t.Fatalf("create = %d %v", status, created)
	}
	id, _ := created["id"].(string)

	status, list := f.do(t, http.MethodGet, "/api/data/notes", "")
	if status != http.StatusOK {
		t.Fatalf("list while on = %d %v", status, list)
	}
	if total, _ := list["total"].(float64); total != 1 {
		t.Fatalf("total while on = %v, want 1", list["total"])
	}

	f.featureOn.Store(false)

	status, catalog := f.do(t, http.MethodGet, "/api/data", "")
	if status != http.StatusOK {
		t.Fatalf("catalog while off = %d %v", status, catalog)
	}
	if catalogLists(catalog, "notes") {
		t.Fatalf("catalog still lists notes while the plugin is off: %v", catalog)
	}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/data/notes", ""},
		{http.MethodGet, "/api/data/notes/" + id, ""},
		{http.MethodPost, "/api/data/notes", `{"title":"After the switch"}`},
	} {
		status, resp := f.do(t, tc.method, tc.path, tc.body)
		t.Logf("%s %s while off -> %d %v", tc.method, tc.path, status, resp["detail"])
		if status != http.StatusNotFound {
			t.Fatalf("%s %s while off = %d %v, want 404", tc.method, tc.path, status, resp)
		}
	}

	f.featureOn.Store(true)
	status, list = f.do(t, http.MethodGet, "/api/data/notes", "")
	if status != http.StatusOK {
		t.Fatalf("list after turning it back on = %d %v", status, list)
	}
	if total, _ := list["total"].(float64); total != 1 {
		t.Fatalf("total after turning it back on = %v, want 1", list["total"])
	}
}

func TestTheMenuEntryCarriesItsOwner(t *testing.T) {
	f, _ := newFixture(t)

	mp, ok := any(f.data).(plugin.MenuProvider)
	if !ok {
		t.Fatal("the ontology plugin is no longer a plugin.MenuProvider")
	}
	menus := mp.Menus()
	if len(menus) != 1 {
		t.Fatalf("menus = %v, want one entry for notes", menus)
	}
	if menus[0].Path != "/data/notes" {
		t.Fatalf("menu path = %q, want /data/notes", menus[0].Path)
	}
	if menus[0].Feature != "notes" {
		t.Fatalf("menu Feature = %q, want notes", menus[0].Feature)
	}
}

func catalogLists(body map[string]any, slug string) bool {
	entities, _ := body["entities"].([]any)
	for _, raw := range entities {
		if entry, ok := raw.(map[string]any); ok && entry["slug"] == slug {
			return true
		}
	}
	return false
}
