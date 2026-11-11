package tenantsql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// The query-builder methods on TenantDB are the surface plugins call. Each one
// must keep the tenant predicate, including the ones that look like pure
// pass-throughs: a dropped WHERE here is a cross-tenant read.

func seedTenantRows(t *testing.T, db *gorm.DB) (own, foreign mockLegacyModel) {
	t.Helper()
	own = mockLegacyModel{OwnerID: 1, Name: "own"}
	foreign = mockLegacyModel{OwnerID: 2, Name: "foreign"}
	if err := db.Create(&own).Error; err != nil {
		t.Fatalf("seed own row: %v", err)
	}
	if err := db.Create(&foreign).Error; err != nil {
		t.Fatalf("seed foreign row: %v", err)
	}
	return own, foreign
}

func tenantDBFor(t *testing.T, db *gorm.DB, orgID uint, opts ...TenantDBOption) *TenantDB {
	t.Helper()
	tdb, err := NewTenantDB(db, orgID, opts...)
	if err != nil {
		t.Fatalf("NewTenantDB(%d): %v", orgID, err)
	}
	return tdb
}

func TestTenantDB_BuildersKeepTenantPredicate(t *testing.T) {
	db := setupTestDB(t)
	tdb := tenantDBFor(t, db, 7)
	rows := []mockLegacyModel{}

	builders := map[string]func() *gorm.DB{
		"Where":    func() *gorm.DB { return tdb.Where("name = ?", "x") },
		"Select":   func() *gorm.DB { return tdb.Select("name") },
		"Order":    func() *gorm.DB { return tdb.Order("name") },
		"Limit":    func() *gorm.DB { return tdb.Limit(5) },
		"Offset":   func() *gorm.DB { return tdb.Offset(5) },
		"Joins":    func() *gorm.DB { return tdb.Joins("JOIN mock_org_models ON 1 = 1") },
		"Preload":  func() *gorm.DB { return tdb.Preload("Details") },
		"Omit":     func() *gorm.DB { return tdb.Omit("name") },
		"Distinct": func() *gorm.DB { return tdb.Distinct("name") },
		"Model":    func() *gorm.DB { return tdb.Model(&mockLegacyModel{}) },
		"Table":    func() *gorm.DB { return tdb.Table("mock_legacy_models") },
	}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			sql := build().Session(&gorm.Session{DryRun: true}).Find(&rows).Statement.SQL.String()
			if !strings.Contains(sql, "owner_id = ?") {
				t.Fatalf("%s dropped the tenant predicate: %s", name, sql)
			}
		})
	}
}

func TestTenantDB_ReadsAreTenantBounded(t *testing.T) {
	db := setupTestDB(t)
	own, _ := seedTenantRows(t, db)
	tdb := tenantDBFor(t, db, 1)

	var found []mockLegacyModel
	if err := tdb.Find(&found).Error; err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 || found[0].ID != own.ID {
		t.Fatalf("Find returned %d rows, want only the org-1 row", len(found))
	}

	var first mockLegacyModel
	if err := tdb.First(&first).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if first.ID != own.ID {
		t.Fatalf("First returned row %d, want %d", first.ID, own.ID)
	}

	var taken mockLegacyModel
	if err := tdb.Take(&taken).Error; err != nil {
		t.Fatalf("Take: %v", err)
	}
	if taken.ID != own.ID {
		t.Fatalf("Take returned row %d, want %d", taken.ID, own.ID)
	}

	var count int64
	if err := tdb.Model(&mockLegacyModel{}).Count(&count).Error; err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Count = %d, want 1", count)
	}

	var viaModel []mockLegacyModel
	if err := tdb.Model(&mockLegacyModel{}).Find(&viaModel).Error; err != nil {
		t.Fatalf("Model+Find: %v", err)
	}
	if len(viaModel) != 1 || viaModel[0].ID != own.ID {
		t.Fatalf("Model+Find returned %d rows, want only the org-1 row", len(viaModel))
	}

	var viaTable []mockLegacyModel
	if err := tdb.Table("mock_legacy_models").Find(&viaTable).Error; err != nil {
		t.Fatalf("Table+Find: %v", err)
	}
	if len(viaTable) != 1 || viaTable[0].ID != own.ID {
		t.Fatalf("Table+Find returned %d rows, want only the org-1 row", len(viaTable))
	}
}

func TestTenantDB_WritesCannotCrossTenants(t *testing.T) {
	db := setupTestDB(t)
	_, foreign := seedTenantRows(t, db)
	tdb := tenantDBFor(t, db, 1)

	res := tdb.Delete(&mockLegacyModel{}, foreign.ID)
	if res.Error != nil {
		t.Fatalf("Delete: %v", res.Error)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("Delete touched %d foreign rows, want 0", res.RowsAffected)
	}
	var stillThere mockLegacyModel
	if err := db.First(&stillThere, foreign.ID).Error; err != nil {
		t.Fatalf("foreign row was deleted by another tenant: %v", err)
	}

	mismatched := mockLegacyModel{ID: foreign.ID, OwnerID: foreign.OwnerID, Name: "hijacked"}
	if err := tdb.Save(&mismatched).Error; !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("Save across tenants: got %v, want ErrTenantMismatch", err)
	}

	own := mockLegacyModel{OwnerID: 1, Name: "before"}
	if err := db.Create(&own).Error; err != nil {
		t.Fatalf("seed own row: %v", err)
	}
	own.Name = "after"
	if err := tdb.Save(&own).Error; err != nil {
		t.Fatalf("Save own row: %v", err)
	}
	var reloaded mockLegacyModel
	if err := db.First(&reloaded, own.ID).Error; err != nil {
		t.Fatalf("reload own row: %v", err)
	}
	if reloaded.Name != "after" {
		t.Fatalf("Save did not persist the update: name = %q", reloaded.Name)
	}
}

func TestTenantDB_ResolveColumnBranches(t *testing.T) {
	db := setupTestDB(t)
	tdb := tenantDBFor(t, db, 1)

	cases := []struct {
		name string
		tdb  *TenantDB
		arg  any
		want string
	}{
		{"nil model uses the default column", tdb, nil, "owner_id"},
		{"nil pointer falls back to the default", tdb, (*mockLegacyModel)(nil), "owner_id"},
		{"owner_id tag wins", tdb, &mockLegacyModel{}, "owner_id"},
		{"org_id column wins", tdb, &mockOrgModel{}, "org_id"},
		{"slice of structs inspects the element", tdb, []mockLegacyModel{}, "owner_id"},
		{"slice of pointers inspects the element", tdb, []*mockOrgModel{}, "org_id"},
		{"TenantScoped overrides reflection", tdb, mockCustomScopedModel{}, "account_id"},
		{"unknown value uses the default", tdb, map[string]any{}, "owner_id"},
		{"custom default column is used for unknown values", tdb.WithColumn("org_id"), map[string]any{}, "org_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tdb.ResolveColumn(tc.arg); got != tc.want {
				t.Fatalf("ResolveColumn(%T) = %q, want %q", tc.arg, got, tc.want)
			}
		})
	}
}

func TestTenantDB_WithColumnAndWithContext(t *testing.T) {
	db := setupTestDB(t)
	tdb := tenantDBFor(t, db, 3)

	if got := tdb.WithColumn("   ").DefaultColumn(); got != DefaultTenantColumn {
		t.Fatalf("WithColumn(blank) = %q, want %q", got, DefaultTenantColumn)
	}
	if got := tdb.WithColumn(" org_id ").DefaultColumn(); got != "org_id" {
		t.Fatalf("WithColumn does not trim: %q", got)
	}
	if got := tdb.DefaultColumn(); got != DefaultTenantColumn {
		t.Fatalf("WithColumn mutated the receiver: %q", got)
	}

	ctx := context.WithValue(context.Background(), struct{}{}, "value")
	bound := tdb.WithContext(ctx)
	if bound.OrgID() != tdb.OrgID() {
		t.Fatalf("WithContext changed the org: %d", bound.OrgID())
	}
	if bound == tdb {
		t.Fatal("WithContext returned the same handle")
	}

	// A zero-value handle has no *gorm.DB; WithContext must still copy it
	// without panicking.
	empty := (&TenantDB{}).WithContext(ctx)
	if empty.OrgID() != 0 {
		t.Fatalf("zero-value WithContext produced org %d", empty.OrgID())
	}
}
