package db

import (
	"testing"

	"github.com/octarq-org/led/config"
	"github.com/octarq-org/led/internal/models"
)

func TestDB(t *testing.T) {
	// 1. Open SQLite (valid)
	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    "file::memory:?cache=shared",
	}
	gdb, err := Open(cfg)
	if err != nil {
		t.Fatalf("expected no error opening DB, got %v", err)
	}

	// 2. Open unsupported driver
	badCfg := &config.Config{
		DBDriver: "invalid",
	}
	_, err = Open(badCfg)
	if err == nil {
		t.Error("expected error for unsupported driver, got nil")
	}

	// 3. Migrate
	type ExtraModel struct {
		ID   uint   `gorm:"primaryKey"`
		Name string `gorm:"size:255"`
	}
	err = Migrate(gdb, &ExtraModel{})
	if err != nil {
		t.Errorf("expected no error migrating DB, got %v", err)
	}
}

func TestDBMigrationLegacy(t *testing.T) {
	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    "file:memdb_legacy?mode=memory&cache=shared",
	}
	gdb, err := Open(cfg)
	if err != nil {
		t.Fatalf("expected no error opening DB, got %v", err)
	}

	// 1. Manually create the legacy domains table
	err = gdb.Exec(`CREATE TABLE domains (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		provider TEXT,
		config TEXT,
		provider_account_id INTEGER
	)`).Error
	if err != nil {
		t.Fatalf("failed to create legacy domains table: %v", err)
	}

	// 2. Insert some legacy domain rows
	err = gdb.Exec(`INSERT INTO domains (name, provider, config) VALUES 
		('legacy1.com', 'cloudflare', 'enc-token-1'),
		('legacy2.com', 'cloudflare', 'enc-token-1'),
		('legacy3.com', '', '')`).Error
	if err != nil {
		t.Fatalf("failed to insert legacy data: %v", err)
	}

	// 3. Run Migrate - it should detect the columns, migrate them, and create provider accounts
	err = Migrate(gdb)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// 4. Verify that ProviderAccounts were created and domains were updated
	var count int64
	gdb.Model(&models.ProviderAccount{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 migrated provider account, got %d", count)
	}

	var d1 models.Domain
	gdb.Where("name = ?", "legacy1.com").First(&d1)
	if d1.ProviderAccountID == 0 {
		t.Errorf("legacy1.com provider_account_id was not updated")
	}

	var d2 models.Domain
	gdb.Where("name = ?", "legacy2.com").First(&d2)
	if d2.ProviderAccountID != d1.ProviderAccountID {
		t.Errorf("legacy2.com should share the same provider account, got different ID")
	}
}
