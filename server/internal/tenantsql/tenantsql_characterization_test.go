package tenantsql_test

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type charTestModel struct {
	ID      uint `gorm:"primaryKey"`
	OwnerID uint `gorm:"index"`
	Name    string
}

func setupCharTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(&charTestModel{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

func TestCharacterization_TenantDB_FailClosedGuards(t *testing.T) {
	t.Parallel()

	db := setupCharTestDB(t)

	if _, err := tenantsql.NewTenantDB(nil, 42); err != tenantsql.ErrNilDB {
		t.Errorf("NewTenantDB(nil, 42) = %v, want ErrNilDB", err)
	}

	if _, err := tenantsql.NewTenantDB(db, 0); err != tenantsql.ErrZeroOrgID {
		t.Errorf("NewTenantDB(db, 0) = %v, want ErrZeroOrgID", err)
	}

	if _, err := tenantsql.TenantDBFromContext(nil, db); err != tenantsql.ErrMissingTenantContext { //nolint:staticcheck // SA1012: nil is the exact input under test (nil-context fail-closed guard)
		t.Errorf("TenantDBFromContext(nil, db) = %v, want ErrMissingTenantContext", err)
	}

	emptyCtx := context.Background()
	if _, err := tenantsql.TenantDBFromContext(emptyCtx, db); err != tenantsql.ErrMissingTenantContext {
		t.Errorf("TenantDBFromContext(emptyCtx, db) = %v, want ErrMissingTenantContext", err)
	}

	validCtx := tenantsql.WithOrgID(emptyCtx, 100)
	tdb, err := tenantsql.TenantDBFromContext(validCtx, db)
	if err != nil || tdb == nil {
		t.Fatalf("TenantDBFromContext(validCtx, db) failed: %v", err)
	}
	if tdb.OrgID() != 100 {
		t.Errorf("tdb.OrgID() = %d, want 100", tdb.OrgID())
	}
}

func TestCharacterization_TenantDB_WriteMismatchGuards(t *testing.T) {
	t.Parallel()

	db := setupCharTestDB(t)
	tdb, err := tenantsql.NewTenantDB(db, 10)
	if err != nil {
		t.Fatalf("NewTenantDB: %v", err)
	}

	conflicting := &charTestModel{
		OwnerID: 999,
		Name:    "evil_cross_tenant_insert",
	}
	err = tdb.Create(conflicting).Error
	if err == nil || !strings.Contains(err.Error(), "entity tenant ID does not match bound tenant scope") {
		t.Errorf("expected ErrTenantMismatch on cross-tenant Create, got %v", err)
	}

	err = tdb.Updates(map[string]any{
		"owner_id": uint(999),
	}).Error
	if err == nil || !strings.Contains(err.Error(), "entity tenant ID does not match bound tenant scope") {
		t.Errorf("expected ErrTenantMismatch on cross-tenant Updates map, got %v", err)
	}

	valid := &charTestModel{
		OwnerID: 0,
		Name:    "legit_item",
	}
	if err := tdb.Create(valid).Error; err != nil {
		t.Fatalf("legit create failed: %v", err)
	}
	if valid.OwnerID != 10 {
		t.Errorf("expected valid.OwnerID auto-populated to 10, got %d", valid.OwnerID)
	}
}

func TestCharacterization_Execute_SecurityGuards(t *testing.T) {
	t.Parallel()

	db := setupCharTestDB(t)

	_, _, err := tenantsql.Execute(context.Background(), db, nil, "SELECT * FROM tenant_links")
	if err == nil || !strings.Contains(err.Error(), "missing workspace context") {
		t.Errorf("expected missing workspace context error, got %v", err)
	}

	ctx := tenantsql.WithOrgID(context.Background(), 1)
	_, _, err = tenantsql.Execute(ctx, nil, nil, "SELECT * FROM tenant_links")
	if err == nil || !strings.Contains(err.Error(), "database handle is nil") {
		t.Errorf("expected nil database handle error, got %v", err)
	}
}

func TestCharacterization_SensitiveColumns(t *testing.T) {
	t.Parallel()

	expectedSensitive := []string{
		"password", "password_hash", "hash", "secret", "token",
		"access_token", "refresh_token", "private_key", "otp", "ciphertext",
	}

	for _, col := range expectedSensitive {
		if !tenantsql.SensitiveColumns[col] {
			t.Errorf("SensitiveColumns[%q] must be true", col)
		}
	}

	if tenantsql.RedactedValue != "[redacted]" {
		t.Errorf("RedactedValue = %q, want [redacted]", tenantsql.RedactedValue)
	}

	row := map[string]any{
		"id":       1,
		"name":     "alice",
		"password": "supersecretpassword",
		"secret":   "secretkey",
	}
	tenantsql.RedactRow([]string{"id", "name", "password", "secret"}, row)

	if row["password"] != tenantsql.RedactedValue {
		t.Errorf("expected password redacted, got %v", row["password"])
	}
	if row["secret"] != tenantsql.RedactedValue {
		t.Errorf("expected secret redacted, got %v", row["secret"])
	}
	if row["name"] != "alice" {
		t.Errorf("expected name unchanged, got %v", row["name"])
	}
}
