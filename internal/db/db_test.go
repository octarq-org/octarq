package db

import (
	"testing"

	"github.com/octarq-org/octarq/config"
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
