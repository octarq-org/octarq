package tenantsql

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockLegacyModel struct {
	ID        uint   `gorm:"primaryKey"`
	OwnerID   uint   `gorm:"column:owner_id;index"`
	Name      string `gorm:"size:255"`
	SecretVal string `gorm:"size:255"`
}

type mockOrgModel struct {
	ID      uint   `gorm:"primaryKey"`
	OrgID   uint   `gorm:"column:org_id;index"`
	Details string `gorm:"size:255"`
}

type mockCustomScopedModel struct {
	ID        uint   `gorm:"primaryKey"`
	AccountID uint   `gorm:"column:account_id;index"`
	Data      string `gorm:"size:255"`
}

func (m mockCustomScopedModel) TenantColumn() string {
	return "account_id"
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&mockLegacyModel{}, &mockOrgModel{}, &mockCustomScopedModel{}); err != nil {
		t.Fatalf("failed to auto-migrate test models: %v", err)
	}
	return db
}

func TestNewTenantDB_Validation(t *testing.T) {
	db := setupTestDB(t)

	// Nil DB
	if _, err := NewTenantDB(nil, 1); !errors.Is(err, ErrNilDB) {
		t.Errorf("expected ErrNilDB, got %v", err)
	}

	// Zero OrgID
	if _, err := NewTenantDB(db, 0); !errors.Is(err, ErrZeroOrgID) {
		t.Errorf("expected ErrZeroOrgID, got %v", err)
	}

	// Valid creation
	tdb, err := NewTenantDB(db, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tdb.OrgID() != 42 {
		t.Errorf("expected OrgID 42, got %d", tdb.OrgID())
	}
	if tdb.DefaultColumn() != "owner_id" {
		t.Errorf("expected default column owner_id, got %s", tdb.DefaultColumn())
	}

	// Custom column option
	tdbCustom, err := NewTenantDB(db, 42, WithTenantColumn("org_id"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tdbCustom.DefaultColumn() != "org_id" {
		t.Errorf("expected default column org_id, got %s", tdbCustom.DefaultColumn())
	}
}

func TestTenantDBFromContext(t *testing.T) {
	db := setupTestDB(t)

	// Nil context
	var nilCtx context.Context
	if _, err := TenantDBFromContext(nilCtx, db); !errors.Is(err, ErrMissingTenantContext) {
		t.Errorf("expected ErrMissingTenantContext for nil ctx, got %v", err)
	}

	// Unauthenticated context (OrgID 0)
	ctxBg := context.Background()
	if _, err := TenantDBFromContext(ctxBg, db); !errors.Is(err, ErrMissingTenantContext) {
		t.Errorf("expected ErrMissingTenantContext for empty ctx, got %v", err)
	}

	// Authenticated context
	ctxAuth := WithOrgID(ctxBg, 100)
	tdb, err := TenantDBFromContext(ctxAuth, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tdb.OrgID() != 100 {
		t.Errorf("expected OrgID 100, got %d", tdb.OrgID())
	}
}

func TestTenantDB_ResolveColumn(t *testing.T) {
	db := setupTestDB(t)
	tdb, err := NewTenantDB(db, 10)
	if err != nil {
		t.Fatal(err)
	}

	// nil model fallback
	if col := tdb.ResolveColumn(nil); col != "owner_id" {
		t.Errorf("expected owner_id fallback for nil, got %s", col)
	}

	// mockLegacyModel (has column:owner_id)
	if col := tdb.ResolveColumn(&mockLegacyModel{}); col != "owner_id" {
		t.Errorf("expected owner_id for mockLegacyModel, got %s", col)
	}

	// mockOrgModel (has column:org_id)
	if col := tdb.ResolveColumn(&mockOrgModel{}); col != "org_id" {
		t.Errorf("expected org_id for mockOrgModel, got %s", col)
	}

	// mockCustomScopedModel (implements TenantScoped)
	if col := tdb.ResolveColumn(&mockCustomScopedModel{}); col != "account_id" {
		t.Errorf("expected account_id for mockCustomScopedModel, got %s", col)
	}

	// Slice of models
	if col := tdb.ResolveColumn(&[]mockOrgModel{}); col != "org_id" {
		t.Errorf("expected org_id for slice of mockOrgModel, got %s", col)
	}
}

func TestTenantDB_CrossTenantIsolation_Queries(t *testing.T) {
	db := setupTestDB(t)

	// Seed data for Tenant 1 and Tenant 2
	t1Model := mockLegacyModel{OwnerID: 1, Name: "tenant1_item", SecretVal: "secret1"}
	t2Model := mockLegacyModel{OwnerID: 2, Name: "tenant2_item", SecretVal: "secret2"}
	if err := db.Create(&t1Model).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&t2Model).Error; err != nil {
		t.Fatal(err)
	}

	tdb1, _ := NewTenantDB(db, 1)
	tdb2, _ := NewTenantDB(db, 2)

	// 1. Find all: Tenant 1 should only see its own records
	var t1Results []mockLegacyModel
	if err := tdb1.Find(&t1Results).Error; err != nil {
		t.Fatalf("tdb1.Find failed: %v", err)
	}
	if len(t1Results) != 1 || t1Results[0].Name != "tenant1_item" {
		t.Errorf("tdb1.Find leaked records: got %+v", t1Results)
	}

	// 2. IDOR Attempt: Tenant 2 queries Tenant 1's ID directly via First
	var idorTarget mockLegacyModel
	err := tdb2.First(&idorTarget, "id = ?", t1Model.ID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("expected ErrRecordNotFound on cross-tenant IDOR read, got %v", err)
	}

	// 3. Count: Tenant 1 count should be exactly 1
	var cnt int64
	if err := tdb1.Model(&mockLegacyModel{}).Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Errorf("expected count 1, got %d", cnt)
	}
}

func TestTenantDB_AutomaticTenantInjectionOnCreate(t *testing.T) {
	db := setupTestDB(t)
	tdb1, _ := NewTenantDB(db, 1)

	// Case 1: Zero OwnerID is automatically populated with tdb1.OrgID
	newItem := mockLegacyModel{Name: "auto_scoped_item"}
	if err := tdb1.Create(&newItem).Error; err != nil {
		t.Fatalf("tdb1.Create failed: %v", err)
	}
	if newItem.OwnerID != 1 {
		t.Errorf("expected OwnerID 1, got %d", newItem.OwnerID)
	}

	// Verify in DB
	var fetched mockLegacyModel
	if err := db.First(&fetched, "id = ?", newItem.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fetched.OwnerID != 1 {
		t.Errorf("persisted OwnerID expected 1, got %d", fetched.OwnerID)
	}

	// Case 2: Explicit conflicting tenant ID is blocked
	tamperedItem := mockLegacyModel{OwnerID: 999, Name: "spoofed_item"}
	err := tdb1.Create(&tamperedItem).Error
	if !errors.Is(err, ErrTenantMismatch) {
		t.Errorf("expected ErrTenantMismatch on spoofed tenant create, got %v", err)
	}

	// Verify tamperedItem was NOT inserted
	var count int64
	db.Model(&mockLegacyModel{}).Where("name = ?", "spoofed_item").Count(&count)
	if count != 0 {
		t.Errorf("spoofed item was illegally persisted to database!")
	}
}

func TestTenantDB_CrossTenantDeletePrevention(t *testing.T) {
	db := setupTestDB(t)

	// Seed Tenant 1 and Tenant 2 data
	t1Model := mockLegacyModel{OwnerID: 1, Name: "t1_delete_target"}
	t2Model := mockLegacyModel{OwnerID: 2, Name: "t2_protected"}
	db.Create(&t1Model)
	db.Create(&t2Model)

	tdb1, _ := NewTenantDB(db, 1)

	// Tenant 1 attempts to delete Tenant 2's item by ID
	res := tdb1.Delete(&mockLegacyModel{}, "id = ?", t2Model.ID)
	if res.Error != nil {
		t.Fatalf("delete error: %v", res.Error)
	}
	if res.RowsAffected != 0 {
		t.Errorf("cross-tenant delete succeeded! RowsAffected: %d", res.RowsAffected)
	}

	// Verify Tenant 2's item is still in DB
	var checkT2 mockLegacyModel
	if err := db.First(&checkT2, "id = ?", t2Model.ID).Error; err != nil {
		t.Errorf("tenant 2 record was unexpectedly deleted: %v", err)
	}

	// Tenant 1 deletes its own item
	resT1 := tdb1.Delete(&mockLegacyModel{}, "id = ?", t1Model.ID)
	if resT1.RowsAffected != 1 {
		t.Errorf("expected 1 row deleted for own tenant, got %d", resT1.RowsAffected)
	}
}

func TestTenantDB_TransactionScoping(t *testing.T) {
	db := setupTestDB(t)
	tdb1, _ := NewTenantDB(db, 1)

	err := tdb1.Transaction(func(tx *TenantDB) error {
		if tx.OrgID() != 1 {
			t.Errorf("expected transaction tx to inherit OrgID 1, got %d", tx.OrgID())
		}
		item := mockLegacyModel{Name: "tx_item"}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		if item.OwnerID != 1 {
			t.Errorf("expected tx created item to have OwnerID 1, got %d", item.OwnerID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify rollback on error
	_ = tdb1.Transaction(func(tx *TenantDB) error {
		_ = tx.Create(&mockLegacyModel{Name: "rollback_item"}).Error
		return errors.New("simulated error")
	})

	var rbCount int64
	db.Model(&mockLegacyModel{}).Where("name = ?", "rollback_item").Count(&rbCount)
	if rbCount != 0 {
		t.Errorf("expected rollback item count 0, got %d", rbCount)
	}
}

func TestTenantDB_AutoDetectOrgIDColumn(t *testing.T) {
	db := setupTestDB(t)
	tdb, _ := NewTenantDB(db, 5)

	// mockOrgModel uses `org_id` column
	orgItem := mockOrgModel{Details: "org_model_test"}
	if err := tdb.Create(&orgItem).Error; err != nil {
		t.Fatalf("failed to create mockOrgModel: %v", err)
	}
	if orgItem.OrgID != 5 {
		t.Errorf("expected auto-injected OrgID 5, got %d", orgItem.OrgID)
	}

	// Query using Scoped - should use `org_id = 5`
	var found []mockOrgModel
	if err := tdb.Find(&found).Error; err != nil {
		t.Fatalf("failed to query mockOrgModel: %v", err)
	}
	if len(found) != 1 || found[0].Details != "org_model_test" {
		t.Errorf("unexpected query result: %+v", found)
	}
}

func TestTenantDB_ExecuteSQL(t *testing.T) {
	db := setupTestDB(t)
	reg := NewRegistry()
	_ = reg.Register(TenantView{
		Name: "tenant_sample_items",
		Columns: []TenantColumn{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "text"},
		},
		Definition: func(orgID uint) string {
			return "SELECT id, name FROM mock_legacy_models WHERE owner_id = " + quoteVal(orgID)
		},
	})

	db.Create(&mockLegacyModel{OwnerID: 7, Name: "tenant7_sample"})
	db.Create(&mockLegacyModel{OwnerID: 8, Name: "tenant8_sample"})

	tdb7, err := NewTenantDB(db, 7, WithTenantRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}

	rows, meta, err := tdb7.ExecuteSQL("SELECT name FROM tenant_sample_items")
	if err != nil {
		t.Fatalf("tdb7.ExecuteSQL failed: %v", err)
	}
	if meta.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", meta.RowCount)
	}
	if len(rows) != 1 || rows[0]["name"] != "tenant7_sample" {
		t.Errorf("unexpected rows: %+v", rows)
	}
}

func TestTenantDB_UnscopedDangerous(t *testing.T) {
	db := setupTestDB(t)
	tdb, _ := NewTenantDB(db, 10)

	raw := tdb.UnscopedDangerous()
	if raw == nil {
		t.Fatal("expected non-nil raw *gorm.DB")
	}
}

func quoteVal(v uint) string {
	return time.Now().Format("") + string([]byte{byte('0' + v%10)})
}
