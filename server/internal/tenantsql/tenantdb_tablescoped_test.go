package tenantsql

import (
	"errors"
	"testing"
)

func TestTenantDB_TableScopedKeepsTenantsApart(t *testing.T) {
	db := setupTestDB(t)

	if err := db.Exec(`CREATE TABLE ont_widget (id TEXT PRIMARY KEY, org_id INTEGER NOT NULL, name TEXT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	for _, row := range []struct {
		id    string
		orgID uint
		name  string
	}{
		{"a", 1, "one"},
		{"b", 2, "two"},
	} {
		if err := db.Exec(`INSERT INTO ont_widget (id, org_id, name) VALUES (?, ?, ?)`,
			row.id, row.orgID, row.name).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	org1, err := NewTenantDB(db, 1)
	if err != nil {
		t.Fatalf("NewTenantDB: %v", err)
	}
	org2, err := NewTenantDB(db, 2)
	if err != nil {
		t.Fatalf("NewTenantDB: %v", err)
	}

	read := func(tdb *TenantDB) []map[string]any {
		t.Helper()
		q, err := tdb.TableScoped("ont_widget", "org_id")
		if err != nil {
			t.Fatalf("TableScoped: %v", err)
		}
		var rows []map[string]any
		if err := q.Find(&rows).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		return rows
	}

	rows1 := read(org1)
	if len(rows1) != 1 || rows1[0]["name"] != "one" {
		t.Fatalf("org 1 saw %v, want only its own row", rows1)
	}
	rows2 := read(org2)
	if len(rows2) != 1 || rows2[0]["name"] != "two" {
		t.Fatalf("org 2 saw %v, want only its own row", rows2)
	}

	// A delete through the scoped handle must not reach the other tenant.
	q, err := org1.TableScoped("ont_widget", "org_id")
	if err != nil {
		t.Fatalf("TableScoped: %v", err)
	}
	if err := q.Delete(nil).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	var count int64
	if err := db.Table("ont_widget").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("remaining rows = %d, want 1 (org 2's)", count)
	}
}

func TestTenantDB_TableScopedRejectsBadIdentifiers(t *testing.T) {
	db := setupTestDB(t)
	tdb, err := NewTenantDB(db, 1)
	if err != nil {
		t.Fatalf("NewTenantDB: %v", err)
	}

	for _, tc := range []struct {
		name   string
		table  string
		column string
		want   error
	}{
		{"sql in table", "ont_widget; DROP TABLE users", "org_id", ErrInvalidTable},
		{"empty table", "", "org_id", ErrInvalidTable},
		{"quoted table", `"ont_widget"`, "org_id", ErrInvalidTable},
		{"empty column", "ont_widget", "", ErrInvalidTenantColumn},
		{"sql in column", "ont_widget", "org_id = org_id OR 1=1", ErrInvalidTenantColumn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tdb.TableScoped(tc.table, tc.column)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if q != nil {
				t.Fatal("expected nil handle on rejection")
			}
		})
	}

	var nilDB *TenantDB
	if _, err := nilDB.TableScoped("ont_widget", "org_id"); !errors.Is(err, ErrNilDB) {
		t.Fatalf("nil handle err = %v, want ErrNilDB", err)
	}
}
