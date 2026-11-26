package models_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBaseDomain(t *testing.T) {
	t.Setenv(models.BaseDomainEnv, "Tenants.Example.COM.")
	if got := models.BaseDomain(nil); got != "tenants.example.com" {
		t.Fatalf("expected tenants.example.com from env, got %q", got)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate setting: %v", err)
	}

	if got := models.BaseDomain(db); got != "tenants.example.com" {
		t.Fatalf("expected the env base domain, got %q", got)
	}

	var seeded models.Setting
	if err := db.Where("key = ?", models.BaseDomainSetting).First(&seeded).Error; err != nil {
		t.Fatalf("expected the env base domain to be persisted as the setting: %v", err)
	}

	if err := db.Model(&models.Setting{}).Where("key = ?", models.BaseDomainSetting).Update("value", " Custom.Domain.Org. ").Error; err != nil {
		t.Fatalf("update setting: %v", err)
	}

	if got := models.BaseDomain(db); got != "custom.domain.org" {
		t.Fatalf("expected custom.domain.org from db setting, got %q", got)
	}

	db.Model(&models.Setting{}).Where("key = ?", models.BaseDomainSetting).Update("value", "")
	if got := models.BaseDomain(db); got != "" {
		t.Fatalf("expected empty string when db setting is empty, got %q", got)
	}
}
