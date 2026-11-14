package ontology

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// httpFixture serves the plugin's routes the way the composition root does: a
// huma API over a mux, with a middleware that stamps the caller's workspace and
// role onto the request context — the part the platform's auth middleware plays.
type httpFixture struct {
	plugin *Plugin
	db     *gorm.DB
	server *httptest.Server

	// audits collects what the handlers reported, so a test can assert that a
	// write reached the ledger with the record id in its metadata.
	mu     sync.Mutex
	audits []string
}

func (f *httpFixture) auditLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.audits...)
}

func newHTTPFixture(t *testing.T, role string, orgID uint) *httpFixture {
	t.Helper()
	db := openDB(t)
	t.Setenv(EntitiesEnv, ticketDoc)
	t.Setenv(EntitiesFileEnv, "")

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test", "1.0"))
	// Registered before the plugin mounts: huma bakes the chain into each
	// operation when it is registered.
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ctx := plugin.WithOrgID(plugin.WithTokenRole(context.Background(), role), orgID)
		next(huma.WithContext(c, ctx))
	})

	session := &plugin.TestSession{
		OrgRoleFn: func(r *http.Request) string {
			if plugin.OrgIDFromContext(r.Context()) == 0 {
				return ""
			}
			return role
		},
	}
	f := &httpFixture{db: db}
	p := New()
	p.Mount(nil, &plugin.Context{
		Huma: api,
		DB:   db,
		Host: &plugin.TestHost{SessionMock: session},
		Audit: func(_ *http.Request, action, targetType string, _ uint, meta map[string]any) {
			id, _ := meta["id"].(string)
			f.mu.Lock()
			f.audits = append(f.audits, action+" "+targetType+" "+id)
			f.mu.Unlock()
		},
	})
	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	f.plugin = p

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.server = srv
	return f
}

func (f *httpFixture) do(t *testing.T, method, path, body string) (int, map[string]any) {
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
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// TestHTTPRoutesServeOneRecordThroughItsLifecycle walks the six routes the page
// and an API client use. Every one of them is registered from the declaration,
// so a regression here is a declaration that no longer produces a working page.
func TestHTTPRoutesServeOneRecordThroughItsLifecycle(t *testing.T) {
	f := newHTTPFixture(t, "owner", 1)

	status, catalog := f.do(t, http.MethodGet, "/api/data", "")
	if status != http.StatusOK {
		t.Fatalf("catalog = %d %v", status, catalog)
	}
	entities, _ := catalog["entities"].([]any)
	if len(entities) != 1 {
		t.Fatalf("catalog = %v, want the one declared entity", catalog)
	}

	status, created := f.do(t, http.MethodPost, "/api/data/tickets", `{"title":"Printer is down","priority":2}`)
	if status != http.StatusOK {
		t.Fatalf("create = %d %v", status, created)
	}
	id, _ := created["id"].(string)
	if id == "" || created["status"] != "open" {
		t.Fatalf("created = %v, want an id and the declared default", created)
	}
	// The server writes the platform columns and ignores nothing.
	if created["created_at"] == nil || created["updated_at"] == nil {
		t.Fatalf("created = %v, want platform timestamps", created)
	}

	status, list := f.do(t, http.MethodGet, "/api/data/tickets?limit=10", "")
	if status != http.StatusOK || list["total"].(float64) != 1 {
		t.Fatalf("list = %d %v, want one row", status, list)
	}

	status, got := f.do(t, http.MethodGet, "/api/data/tickets/"+id, "")
	if status != http.StatusOK || got["title"] != "Printer is down" {
		t.Fatalf("get = %d %v", status, got)
	}

	status, patched := f.do(t, http.MethodPatch, "/api/data/tickets/"+id, `{"status":"closed"}`)
	if status != http.StatusOK || patched["status"] != "closed" {
		t.Fatalf("update = %d %v", status, patched)
	}

	status, deleted := f.do(t, http.MethodDelete, "/api/data/tickets/"+id, "")
	if status != http.StatusOK || deleted["ok"] != true {
		t.Fatalf("delete = %d %v", status, deleted)
	}
	if status, _ := f.do(t, http.MethodGet, "/api/data/tickets/"+id, ""); status != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", status)
	}

	// Every write reached the ledger with the record id in its metadata: a UUID
	// does not fit the ledger's numeric target.
	want := []string{"data.create tickets " + id, "data.update tickets " + id, "data.delete tickets " + id}
	if got := f.auditLog(); !slices.Equal(got, want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

// TestHTTPRoutesRefuseWhatTheDeclarationRefuses pins the status each refusal
// carries. The numbers matter: a 500 where the caller made a mistake is a bug
// report nobody can act on.
func TestHTTPRoutesRefuseWhatTheDeclarationRefuses(t *testing.T) {
	f := newHTTPFixture(t, "owner", 1)

	status, created := f.do(t, http.MethodPost, "/api/data/tickets", `{"title":"Only one"}`)
	if status != http.StatusOK {
		t.Fatalf("create = %d %v", status, created)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"unknown field", http.MethodPost, "/api/data/tickets", `{"title":"x","nope":1}`, http.StatusUnprocessableEntity},
		{"reserved field", http.MethodPost, "/api/data/tickets", `{"title":"x","org_id":2}`, http.StatusUnprocessableEntity},
		{"missing required", http.MethodPost, "/api/data/tickets", `{"status":"open"}`, http.StatusUnprocessableEntity},
		{"duplicate key", http.MethodPost, "/api/data/tickets", `{"title":"a","title":"b"}`, http.StatusUnprocessableEntity},
		{"unknown sort", http.MethodGet, "/api/data/tickets?sort=nope", "", http.StatusUnprocessableEntity},
		{"unique violation", http.MethodPost, "/api/data/tickets", `{"title":"Only one"}`, http.StatusConflict},
		{"missing record", http.MethodGet, "/api/data/tickets/00000000-0000-0000-0000-000000000000", "", http.StatusNotFound},
		{"update a missing record", http.MethodPatch, "/api/data/tickets/00000000-0000-0000-0000-000000000000", `{"status":"closed"}`, http.StatusNotFound},
		{"delete a missing record", http.MethodDelete, "/api/data/tickets/00000000-0000-0000-0000-000000000000", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := f.do(t, tc.method, tc.path, tc.body)
			if status != tc.want {
				t.Fatalf("%s %s = %d %v, want %d", tc.method, tc.path, status, body, tc.want)
			}
		})
	}
}

// TestHTTPCatalogHidesOperationsTheRoleCannotRun pins the page's buttons to the
// ACL: the catalog is what decides whether a member sees Delete.
func TestHTTPCatalogHidesOperationsTheRoleCannotRun(t *testing.T) {
	member := newHTTPFixture(t, "member", 1)
	status, catalog := member.do(t, http.MethodGet, "/api/data", "")
	if status != http.StatusOK {
		t.Fatalf("catalog = %d %v", status, catalog)
	}
	ops := catalogOperations(t, catalog)
	if strings.Contains(ops, "delete") {
		t.Fatalf("operations = %q, want delete hidden from a member", ops)
	}
	if !strings.Contains(ops, "list") || !strings.Contains(ops, "create") {
		t.Fatalf("operations = %q, want list and create", ops)
	}

	// The refusal is the route's, not only the catalog's: delete is admin.
	if status, body := member.do(t, http.MethodDelete, "/api/data/tickets/x", ""); status != http.StatusForbidden {
		t.Fatalf("delete as member = %d %v, want 403", status, body)
	}
}

// TestHTTPRoutesReportAFailedQueryAs500 pins the last line of the error map: a
// database that cannot answer is the server's problem, and the caller is told so
// without the driver's message.
func TestHTTPRoutesReportAFailedQueryAs500(t *testing.T) {
	f := newHTTPFixture(t, "owner", 1)
	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	status, body := f.do(t, http.MethodGet, "/api/data/tickets", "")
	if status != http.StatusInternalServerError {
		t.Fatalf("list on a closed db = %d %v, want 500", status, body)
	}
	if msg, _ := body["message"].(string); strings.Contains(msg, "sql") || strings.Contains(msg, "closed") {
		t.Fatalf("message = %q, want the generic text, not the driver's", msg)
	}
}

// catalogOperations returns the operation list of the single entity in a
// catalog response.
func catalogOperations(t *testing.T, catalog map[string]any) string {
	t.Helper()
	entities, _ := catalog["entities"].([]any)
	if len(entities) != 1 {
		t.Fatalf("catalog = %v, want one entity", catalog)
	}
	entity, _ := entities[0].(map[string]any)
	ops, _ := entity["operations"].([]any)
	names := make([]string, 0, len(ops))
	for _, op := range ops {
		name, _ := op.(string)
		names = append(names, name)
	}
	return strings.Join(names, ",")
}
