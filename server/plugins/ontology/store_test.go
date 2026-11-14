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

// openDB returns a migrated database with the declaration applied, the state a
// booted server leaves behind.
func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	return openDocDB(t, ticketDoc)
}

// openDocDB is openDB for another declaration document.
func openDocDB(t *testing.T, doc string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&core.AppliedSpec{}); err != nil {
		t.Fatalf("migrate bookkeeping: %v", err)
	}
	specs, err := core.DecodeString(doc)
	if err != nil {
		t.Fatalf("decode declaration: %v", err)
	}
	if err := core.Apply(context.Background(), db, "sqlite", specs); err != nil {
		t.Fatalf("apply declaration: %v", err)
	}
	return db
}

func ticketEntity(t *testing.T) *entity {
	t.Helper()
	specs, err := core.DecodeString(ticketDoc)
	if err != nil {
		t.Fatalf("decode declaration: %v", err)
	}
	return newEntity(specs[0])
}

func tenant(t *testing.T, db *gorm.DB, orgID uint) *plugin.TenantDB {
	t.Helper()
	tdb, err := plugin.NewTenantDB(db, orgID)
	if err != nil {
		t.Fatalf("tenant db: %v", err)
	}
	return tdb
}

func TestStoreScopesReadsAndWritesToTheCaller(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	e := ticketEntity(t)
	first, second := tenant(t, db, 1), tenant(t, db, 2)

	row, err := store{}.create(ctx, first, e, map[string]any{"title": "Printer is down"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, _ := row[core.ColumnID].(string)
	if id == "" {
		t.Fatalf("created row has no id: %v", row)
	}

	// The other workspace must not see it, by list or by id.
	rows, total, err := store{}.list(ctx, second, e, listParams{limit: 10})
	if err != nil {
		t.Fatalf("list as other tenant: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("other tenant sees %d rows (total %d), want none", len(rows), total)
	}
	if _, err := (store{}).get(ctx, second, e, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get as other tenant = %v, want ErrNotFound", err)
	}
	if _, err := (store{}).update(ctx, second, e, id, map[string]any{"status": "closed"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update as other tenant = %v, want ErrNotFound", err)
	}
	if err := (store{}).delete(ctx, second, e, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete as other tenant = %v, want ErrNotFound", err)
	}

	// The owner still sees it, unchanged.
	got, err := (store{}).get(ctx, first, e, id)
	if err != nil {
		t.Fatalf("get as owner: %v", err)
	}
	if got["status"] != "open" {
		t.Fatalf("status = %v, want open: the refused update must not have landed", got["status"])
	}
	if got[core.ColumnOrgID] == nil || got[core.ColumnOrgID] == uint(2) {
		t.Fatalf("org_id = %v, want the owner's workspace", got[core.ColumnOrgID])
	}
}

// TestStoreWritesTheTenantColumnOnCreate pins the one isolation point GORM does
// not enforce: TableScoped attaches its predicate to SELECT, UPDATE and DELETE,
// but an INSERT ignores it, so create must write org_id from the handle.
func TestStoreWritesTheTenantColumnOnCreate(t *testing.T) {
	db := openDB(t)
	e := ticketEntity(t)

	row, err := store{}.create(context.Background(), tenant(t, db, 7), e, map[string]any{"title": "for org seven"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var orgID uint
	if err := db.Table(e.table).Select(core.ColumnOrgID).Where("id = ?", row[core.ColumnID]).Scan(&orgID).Error; err != nil {
		t.Fatalf("read org_id: %v", err)
	}
	if orgID != 7 {
		t.Fatalf("org_id = %d, want 7", orgID)
	}
}

func TestStoreRefusesWithoutTenantScope(t *testing.T) {
	e := ticketEntity(t)
	if _, _, err := (store{}).list(context.Background(), nil, e, listParams{limit: 1}); !errors.Is(err, ErrNoTenantScope) {
		t.Fatalf("list with no tenant = %v, want ErrNoTenantScope", err)
	}
}

func TestStoreRefusesUnknownSort(t *testing.T) {
	e := ticketEntity(t)
	if _, err := parseSort(e, "password"); !errors.Is(err, ErrUnknownSort) {
		t.Fatalf("parseSort = %v, want ErrUnknownSort", err)
	}
}
