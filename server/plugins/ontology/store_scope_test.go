package ontology

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// noteDoc is a user-scoped declaration: a note belongs to one member of the
// workspace, not to the workspace.
const noteDoc = `{"entities":[{
  "slug":"notes","label":"Note","scope":"user",
  "fields":[{"name":"body","type":"text","required":true,"listColumn":true}],
  "operations":{
    "list":{"roles":["member"]},
    "get":{"roles":["member"]},
    "create":{"roles":["member"]},
    "update":{"roles":["member"]},
    "delete":{"roles":["member"]}
  }
}]}`

// userScopedStore returns a store over a freshly created user-scoped table.
func userScopedStore(t *testing.T) (*entity, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&core.AppliedSpec{}); err != nil {
		t.Fatalf("migrate bookkeeping: %v", err)
	}
	specs, err := core.DecodeString(noteDoc)
	if err != nil {
		t.Fatalf("decode declaration: %v", err)
	}
	if err := core.Apply(context.Background(), db, "sqlite", specs); err != nil {
		t.Fatalf("apply declaration: %v", err)
	}
	return newEntity(specs[0]), db
}

// asUser returns a context carrying the caller's workspace and identity, the way
// the platform's auth middleware does. A zero uid models a transport that
// carries no identity.
func asUser(orgID, userID uint) context.Context {
	ctx := plugin.WithOrgID(context.Background(), orgID)
	if userID != 0 {
		ctx = plugin.WithUserID(ctx, userID)
	}
	return ctx
}

// TestUserScopedRowsBelongToOneMember pins the isolation this scope exists for:
// two members of the same workspace share org_id and still cannot see each
// other's records, by list or by id.
func TestUserScopedRowsBelongToOneMember(t *testing.T) {
	e, db := userScopedStore(t)
	alice := asUser(1, 1)
	bob := asUser(1, 2)
	org1, org2 := tenant(t, db, 1), tenant(t, db, 2)

	row, err := store{}.create(alice, org1, e, map[string]any{"body": "alice's note"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, _ := row[core.ColumnID].(string)
	if id == "" {
		t.Fatalf("created row has no id: %v", row)
	}
	// The owner column is written from the context, never from the body.
	if got, _ := row[core.ColumnUserID].(int64); got != 1 {
		t.Fatalf("stored %s = %v, want 1", core.ColumnUserID, row[core.ColumnUserID])
	}

	// The other member of the same workspace sees nothing.
	rows, total, err := store{}.list(bob, org1, e, listParams{limit: 10})
	if err != nil {
		t.Fatalf("list as the other member: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("the other member sees %d rows (total %d), want none", len(rows), total)
	}
	if _, err := (store{}).get(bob, org1, e, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get as the other member = %v, want ErrNotFound", err)
	}
	if _, err := (store{}).update(bob, org1, e, id, map[string]any{"body": "hijacked"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update as the other member = %v, want ErrNotFound", err)
	}
	if err := (store{}).delete(bob, org1, e, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete as the other member = %v, want ErrNotFound", err)
	}

	// Nor does another workspace, even for the same user id.
	rows, total, err = store{}.list(alice, org2, e, listParams{limit: 10})
	if err != nil {
		t.Fatalf("list as another workspace: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("another workspace sees %d rows (total %d), want none", len(rows), total)
	}

	// The owner still sees it, unchanged.
	got, err := (store{}).get(alice, org1, e, id)
	if err != nil {
		t.Fatalf("get as the owner: %v", err)
	}
	if got["body"] != "alice's note" {
		t.Fatalf("body = %v, want the original", got["body"])
	}
}

// TestUserScopedStoreRefusesATransportWithNoUser pins fail-closed. stdio MCP
// stamps a workspace and no identity; falling back to the whole workspace would
// hand one member another's records.
func TestUserScopedStoreRefusesATransportWithNoUser(t *testing.T) {
	e, db := userScopedStore(t)
	anonymous := asUser(1, 0)
	org1 := tenant(t, db, 1)

	if _, _, err := (store{}).list(anonymous, org1, e, listParams{limit: 10}); !errors.Is(err, ErrNoUserScope) {
		t.Fatalf("list = %v, want ErrNoUserScope", err)
	}
	if _, err := (store{}).create(anonymous, org1, e, map[string]any{"body": "x"}); !errors.Is(err, ErrNoUserScope) {
		t.Fatalf("create = %v, want ErrNoUserScope", err)
	}
	if _, err := (store{}).get(anonymous, org1, e, "any"); !errors.Is(err, ErrNoUserScope) {
		t.Fatalf("get = %v, want ErrNoUserScope", err)
	}
}
