package ontology

import (
	"net/http"
	"testing"

	core "github.com/octarq-org/octarq/server/ontology"
)

// TestUserScopedRoutesKeepMembersApart walks the REST surface as two members of
// one workspace. The declaration says the rows belong to a member; neither the
// page nor the API may show one member another's record.
func TestUserScopedRoutesKeepMembersApart(t *testing.T) {
	alice := newHTTPFixtureDoc(t, "member", 1, 1, noteDoc)
	bob := newHTTPFixtureDoc(t, "member", 1, 2, noteDoc)

	status, created := alice.do(t, http.MethodPost, "/api/data/notes", `{"body":"alice's note"}`)
	if status != http.StatusOK {
		t.Fatalf("create = %d %v", status, created)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("created = %v, want an id", created)
	}
	// The owner column is written from the request context, and the record
	// contract does not expose it.
	if _, exposed := created["user_id"]; exposed {
		t.Fatalf("created = %v, want no platform isolation column in the record", created)
	}
	var stored int64
	if err := alice.db.Table(core.TableName("notes")).Where("id = ?", id).
		Pluck("user_id", &stored).Error; err != nil {
		t.Fatalf("read the stored row: %v", err)
	}
	if stored != 1 {
		t.Fatalf("stored user_id = %d, want the caller's", stored)
	}

	if status, list := alice.do(t, http.MethodGet, "/api/data/notes", ""); status != http.StatusOK || list["total"] != float64(1) {
		t.Fatalf("alice list = %d %v, want her one row", status, list)
	}
	if status, list := bob.do(t, http.MethodGet, "/api/data/notes", ""); status != http.StatusOK || list["total"] != float64(0) {
		t.Fatalf("bob list = %d %v, want none of alice's rows", status, list)
	}

	// 404, not 403: the answer must not confirm that the id exists somewhere in
	// the workspace.
	if status, _ := bob.do(t, http.MethodGet, "/api/data/notes/"+id, ""); status != http.StatusNotFound {
		t.Fatalf("bob get = %d, want 404", status)
	}
	if status, _ := bob.do(t, http.MethodPatch, "/api/data/notes/"+id, `{"body":"hijacked"}`); status != http.StatusNotFound {
		t.Fatalf("bob update = %d, want 404", status)
	}
	if status, _ := bob.do(t, http.MethodDelete, "/api/data/notes/"+id, ""); status != http.StatusNotFound {
		t.Fatalf("bob delete = %d, want 404", status)
	}

	// The owner still reads her own record unchanged.
	status, got := alice.do(t, http.MethodGet, "/api/data/notes/"+id, "")
	if status != http.StatusOK || got["body"] != "alice's note" {
		t.Fatalf("alice get = %d %v", status, got)
	}
}

// TestUserScopedRoutesRefuseTheOwnerInTheBody pins that the owner column is
// server-side truth: a caller cannot write a row into another member's set.
func TestUserScopedRoutesRefuseTheOwnerInTheBody(t *testing.T) {
	f := newHTTPFixtureDoc(t, "member", 1, 1, noteDoc)

	status, body := f.do(t, http.MethodPost, "/api/data/notes", `{"body":"x","user_id":2}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("create with user_id = %d %v, want 422", status, body)
	}
	if status, list := f.do(t, http.MethodGet, "/api/data/notes", ""); status != http.StatusOK || list["total"] != float64(0) {
		t.Fatalf("list = %d %v, want nothing written", status, list)
	}
}

// TestUserScopedRoutesRefuseATransportWithNoUser pins fail-closed for stdio MCP,
// which stamps a workspace and no identity.
func TestUserScopedRoutesRefuseATransportWithNoUser(t *testing.T) {
	f := newHTTPFixtureDoc(t, "member", 1, 0, noteDoc)

	status, body := f.do(t, http.MethodGet, "/api/data/notes", "")
	if status != http.StatusForbidden {
		t.Fatalf("list = %d %v, want 403", status, body)
	}
	if detail, _ := body["detail"].(string); detail == "" {
		t.Fatalf("body = %v, want the refusal explained", body)
	}
}

// TestCatalogReportsTheScope pins that the page can tell a member which records
// are their own.
func TestCatalogReportsTheScope(t *testing.T) {
	f := newHTTPFixtureDoc(t, "member", 1, 1, noteDoc)

	status, catalog := f.do(t, http.MethodGet, "/api/data", "")
	if status != http.StatusOK {
		t.Fatalf("catalog = %d %v", status, catalog)
	}
	entities, _ := catalog["entities"].([]any)
	if len(entities) != 1 {
		t.Fatalf("catalog = %v, want the one declared entity", catalog)
	}
	entry, _ := entities[0].(map[string]any)
	if entry["scope"] != "user" {
		t.Fatalf("scope = %v, want user", entry["scope"])
	}
}
