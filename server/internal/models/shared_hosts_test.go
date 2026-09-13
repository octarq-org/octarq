package models

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSharedHosts(t *testing.T) {
	t.Run("empty fallback", func(t *testing.T) {
		t.Setenv(SharedHostsEnv, "")
		if got := SharedHosts(nil); len(got) != 0 {
			t.Errorf("SharedHosts(nil) = %v, want empty", got)
		}
	})

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv(SharedHostsEnv, "app.example.com, OCTARQ.EXAMPLE.COM:8080 , s.example.com.")
		got := SharedHosts(nil)
		want := []string{"app.example.com", "octarq.example.com", "s.example.com"}
		if len(got) != len(want) {
			t.Fatalf("SharedHosts(nil) len = %d, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("db override", func(t *testing.T) {
		dir := t.TempDir()
		db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "test.db")), &gorm.Config{})
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		if err := db.AutoMigrate(&Setting{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}

		t.Setenv(SharedHostsEnv, "env.example.com")

		// No row in settings yet -> returns env
		if got := SharedHosts(db); len(got) != 1 || got[0] != "env.example.com" {
			t.Errorf("SharedHosts(db) before insert = %v, want [env.example.com]", got)
		}

		// Insert setting in DB
		db.Create(&Setting{Key: SharedHostsSetting, Value: "db1.example.com\ndb2.example.com"})
		got := SharedHosts(db)
		if len(got) != 2 || got[0] != "db1.example.com" || got[1] != "db2.example.com" {
			t.Errorf("SharedHosts(db) after insert = %v, want [db1.example.com, db2.example.com]", got)
		}
	})
}
