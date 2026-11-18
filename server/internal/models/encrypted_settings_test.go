package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// encryptedValueDB opens an isolated in-memory database for the probe tests.
func encryptedValueDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&Setting{}, &NotificationChannel{}, &Webhook{}, &User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestHasEncryptedValues(t *testing.T) {
	db := encryptedValueDB(t)

	found, err := HasEncryptedValues(db)
	if err != nil || found {
		t.Fatalf("empty database: found=%v err=%v", found, err)
	}

	// Plain settings and vault-backed metadata rows do not depend on the instance DEK.
	if err := db.Create(&Setting{Key: "app_name", Value: "Octarq"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&NotificationChannel{Name: "Ops", Type: "webhook", Config: `{"url":"https://example.com"}`}).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&Webhook{Name: "Ops", URL: "https://example.com/hook", VaultItemID: 1}).Error; err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if found, err = HasEncryptedValues(db); err != nil || found {
		t.Fatalf("non-DEK rows: found=%v err=%v", found, err)
	}

	// User TOTP secret is still sealed under the instance envelope DEK.
	if err := db.Create(&User{Email: "ops@example.com", PasswordHash: "hash", TOTPSecret: "ciphertext"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if found, err = HasEncryptedValues(db); err != nil || !found {
		t.Fatalf("totp secret: found=%v err=%v", found, err)
	}
}
