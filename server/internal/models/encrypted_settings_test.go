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

	// A plain setting is not ciphertext.
	if err := db.Create(&Setting{Key: "app_name", Value: "Octarq"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if found, err = HasEncryptedValues(db); err != nil || found {
		t.Fatalf("plain setting: found=%v err=%v", found, err)
	}

	// An encrypted setting is.
	if err := db.Create(&Setting{Key: SettingGoogleClientSecret, Value: "ciphertext"}).Error; err != nil {
		t.Fatalf("create encrypted setting: %v", err)
	}
	if found, err = HasEncryptedValues(db); err != nil || !found {
		t.Fatalf("encrypted setting: found=%v err=%v", found, err)
	}
}

func TestHasEncryptedValuesChecksTheDataTables(t *testing.T) {
	cases := []struct {
		name  string
		seed  func(t *testing.T, db *gorm.DB)
		found bool
	}{
		{
			name: "notification channel config",
			seed: func(t *testing.T, db *gorm.DB) {
				if err := db.Create(&NotificationChannel{Name: "Ops", Type: "webhook", Config: "ciphertext"}).Error; err != nil {
					t.Fatalf("create channel: %v", err)
				}
			},
			found: true,
		},
		{
			name: "webhook secret",
			seed: func(t *testing.T, db *gorm.DB) {
				if err := db.Create(&Webhook{Name: "Ops", URL: "https://example.com/hook", Secret: "ciphertext"}).Error; err != nil {
					t.Fatalf("create webhook: %v", err)
				}
			},
			found: true,
		},
		{
			name: "totp secret",
			seed: func(t *testing.T, db *gorm.DB) {
				if err := db.Create(&User{Email: "ops@example.com", PasswordHash: "hash", TOTPSecret: "ciphertext"}).Error; err != nil {
					t.Fatalf("create user: %v", err)
				}
			},
			found: true,
		},
		{
			name: "channel without a config",
			seed: func(t *testing.T, db *gorm.DB) {
				if err := db.Create(&NotificationChannel{Name: "Ops", Type: "webhook"}).Error; err != nil {
					t.Fatalf("create channel: %v", err)
				}
			},
			found: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := encryptedValueDB(t)
			tc.seed(t, db)
			found, err := HasEncryptedValues(db)
			if err != nil {
				t.Fatalf("HasEncryptedValues: %v", err)
			}
			if found != tc.found {
				t.Errorf("HasEncryptedValues = %v, want %v", found, tc.found)
			}
		})
	}
}
