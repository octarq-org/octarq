package models_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBaseDomain(t *testing.T) {
	// 1. Nil DB fallback to env
	t.Setenv(models.BaseDomainEnv, "Tenants.Example.COM.")
	if got := models.BaseDomain(nil); got != "tenants.example.com" {
		t.Fatalf("expected tenants.example.com from env, got %q", got)
	}

	// 2. DB without setting fallback to env
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate setting: %v", err)
	}

	if got := models.BaseDomain(db); got != "tenants.example.com" {
		t.Fatalf("expected fallback to env when setting absent, got %q", got)
	}

	// 3. DB with setting overrides env
	s := models.Setting{Key: models.BaseDomainSetting, Value: " Custom.Domain.Org. "}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	if got := models.BaseDomain(db); got != "custom.domain.org" {
		t.Fatalf("expected custom.domain.org from db setting, got %q", got)
	}

	// 4. Empty setting clears base domain even if env is set
	db.Model(&s).Update("value", "")
	if got := models.BaseDomain(db); got != "" {
		t.Fatalf("expected empty string when db setting is empty, got %q", got)
	}
}
