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

// TestTenantDB_TableScopedByBindsTheSecondDimension pins the owner-level
// isolation: two members of one workspace share org_id and are told apart by the
// predicate the handle binds, not by a filter the caller remembered to add.
func TestTenantDB_TableScopedByBindsTheSecondDimension(t *testing.T) {
	db := setupTestDB(t)

	if err := db.Exec(`CREATE TABLE ont_note (
		id TEXT PRIMARY KEY, org_id INTEGER NOT NULL, user_id INTEGER NOT NULL, body TEXT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	for _, row := range []struct {
		id     string
		orgID  uint
		userID uint
		body   string
	}{
		{"a", 1, 1, "org 1, user 1"},
		{"b", 1, 2, "org 1, user 2"},
		{"c", 2, 1, "org 2, user 1"},
	} {
		if err := db.Exec(`INSERT INTO ont_note (id, org_id, user_id, body) VALUES (?, ?, ?, ?)`,
			row.id, row.orgID, row.userID, row.body).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	org1, err := NewTenantDB(db, 1)
	if err != nil {
		t.Fatalf("NewTenantDB: %v", err)
	}
	read := func(userID uint) []map[string]any {
		t.Helper()
		q, err := org1.TableScopedBy("ont_note", "org_id", Scope{Column: "user_id", Value: userID})
		if err != nil {
			t.Fatalf("TableScopedBy: %v", err)
		}
		var rows []map[string]any
		if err := q.Find(&rows).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		return rows
	}

	if rows := read(1); len(rows) != 1 || rows[0]["body"] != "org 1, user 1" {
		t.Fatalf("user 1 saw %v, want only its own row", rows)
	}
	if rows := read(2); len(rows) != 1 || rows[0]["body"] != "org 1, user 2" {
		t.Fatalf("user 2 saw %v, want only its own row", rows)
	}

	// With no extra predicate it is TableScoped.
	q, err := org1.TableScopedBy("ont_note", "org_id")
	if err != nil {
		t.Fatalf("TableScopedBy: %v", err)
	}
	var rows []map[string]any
	if err := q.Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("workspace-wide read saw %d rows, want the workspace's 2", len(rows))
	}
}

// TestTenantDB_TableScopedByRefusesAnEmptyScope pins that a scope value the
// caller failed to resolve is an error, not a predicate matching nothing: an
// empty page would look like "no records" instead of "no identity".
func TestTenantDB_TableScopedByRefusesAnEmptyScope(t *testing.T) {
	db := setupTestDB(t)
	tdb, err := NewTenantDB(db, 1)
	if err != nil {
		t.Fatalf("NewTenantDB: %v", err)
	}

	for _, tc := range []struct {
		name  string
		scope Scope
		want  error
	}{
		{"zero user", Scope{Column: "user_id", Value: uint(0)}, ErrEmptyScopeValue},
		{"nil value", Scope{Column: "user_id"}, ErrEmptyScopeValue},
		{"empty key", Scope{Column: "scope_key", Value: ""}, ErrEmptyScopeValue},
		{"empty column", Scope{Value: uint(1)}, ErrInvalidScopeColumn},
		{"sql in column", Scope{Column: "user_id = user_id OR 1=1", Value: uint(1)}, ErrInvalidScopeColumn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tdb.TableScopedBy("ont_note", "org_id", tc.scope)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if q != nil {
				t.Fatal("expected nil handle on rejection")
			}
		})
	}
}
