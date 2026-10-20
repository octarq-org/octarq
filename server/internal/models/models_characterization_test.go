package models

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestCharacterization_AllModelsRegistryLocks validates the complete closed set
// of 20 models registered in AllModels() and verifies that AutoMigrate succeeds
// cleanly on a fresh database.
func TestCharacterization_AllModelsRegistryLocks(t *testing.T) {
	t.Parallel()

	models := AllModels()
	if len(models) != 20 {
		t.Fatalf("AllModels() must return exactly 20 registered models, got %d", len(models))
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate failed across AllModels(): %v", err)
	}

	// Verify custom table names
	customTableNames := map[string]string{
		"user_sessions":            Session{}.TableName(),
		"notifications":            Notification{}.TableName(),
		"notification_preferences": NotificationPreference{}.TableName(),
		"files":                    File{}.TableName(),
		"agent_harness_specs":      AgentHarnessSpec{}.TableName(),
		"agent_harness_lessons":    AgentHarnessLesson{}.TableName(),
	}

	for expected, actual := range customTableNames {
		if expected != actual {
			t.Errorf("expected table name %q, got %q", expected, actual)
		}
		if !db.Migrator().HasTable(expected) {
			t.Errorf("database migrator does not have table %q", expected)
		}
	}
}

// TestCharacterization_OrgSlugContract locks the behavior of slug allocation,
// static word reservation, and slug lifecycle states.
func TestCharacterization_OrgSlugContract(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Org{}, &OrgSlugHistory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 1. Reserved slugs must strictly return SlugReserved
	reservedSamples := []string{"admin", "api", "auth", "login", "sso", "webhook", "billing", "settings", "docs"}
	for _, r := range reservedSamples {
		status, err := CheckOrgSlugAvailable(db, r, 0)
		if err != nil {
			t.Errorf("CheckOrgSlugAvailable(%q) err: %v", r, err)
		}
		if status != SlugReserved {
			t.Errorf("CheckOrgSlugAvailable(%q) status = %v, want SlugReserved", r, status)
		}
		if !IsReservedOrgSlug(r) {
			t.Errorf("IsReservedOrgSlug(%q) = false, want true", r)
		}
	}

	// 2. Active org slug must return SlugTaken
	org := Org{Name: "Acme Corp", Slug: "acmecorp"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	status, err := CheckOrgSlugAvailable(db, "acmecorp", 0)
	if err != nil || status != SlugTaken {
		t.Errorf("CheckOrgSlugAvailable(active) = (%v, %v), want (SlugTaken, nil)", status, err)
	}
	// Self-check for same org ID must be available
	status, err = CheckOrgSlugAvailable(db, "acmecorp", org.ID)
	if err != nil || status != SlugAvailable {
		t.Errorf("CheckOrgSlugAvailable(self) = (%v, %v), want (SlugAvailable, nil)", status, err)
	}

	// 3. AllocateOrgSlug generates a valid non-reserved slug
	slug, err := AllocateOrgSlug(db)
	if err != nil {
		t.Fatalf("AllocateOrgSlug failed: %v", err)
	}
	if len(slug) != orgSlugLen {
		t.Errorf("allocated slug len = %d, want %d", len(slug), orgSlugLen)
	}
	for _, ch := range slug {
		if !strings.ContainsRune(slugAlphabet, ch) {
			t.Errorf("allocated slug contains invalid rune %q", ch)
		}
	}
}

// TestCharacterization_BaseDomainAndSharedHostsPrecedence locks the priority:
// DB setting table takes precedence over environment variable, with fallback to empty.
func TestCharacterization_BaseDomainAndSharedHostsPrecedence(t *testing.T) {
	// Not parallel due to environment variable manipulation
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Step A: Env fallback
	t.Setenv(BaseDomainEnv, "Env.Example.COM.")
	t.Setenv(SharedHostsEnv, "app.example.com:8080, api.example.com.")

	if got := BaseDomain(db); got != "env.example.com" {
		t.Errorf("BaseDomain(env) = %q, want env.example.com", got)
	}

	shared := SharedHosts(db)
	if len(shared) != 2 || shared[0] != "app.example.com" || shared[1] != "api.example.com" {
		t.Errorf("SharedHosts(env) = %v, want [app.example.com, api.example.com]", shared)
	}

	// Step B: DB override takes precedence over Env
	db.Create(&Setting{Key: BaseDomainSetting, Value: "Db.Example.Org."})
	db.Create(&Setting{Key: SharedHostsSetting, Value: "hub.example.org:443"})

	if got := BaseDomain(db); got != "db.example.org" {
		t.Errorf("BaseDomain(db) = %q, want db.example.org", got)
	}

	sharedDB := SharedHosts(db)
	if len(sharedDB) != 1 || sharedDB[0] != "hub.example.org" {
		t.Errorf("SharedHosts(db) = %v, want [hub.example.org]", sharedDB)
	}
}

// TestCharacterization_TokenSecurityInvariants locks token hashing,
// prefixing, and expiry checks.
func TestCharacterization_TokenSecurityInvariants(t *testing.T) {
	t.Parallel()

	raw := "oct_secret_token_12345"
	hash1 := HashToken(raw)
	hash2 := HashToken(raw)

	if hash1 != hash2 {
		t.Errorf("HashToken must be deterministic: %q != %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("HashToken must be 64 hex characters (SHA-256), got len %d", len(hash1))
	}

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	tokNoExpiry := Token{ExpiresAt: nil}
	if tokNoExpiry.Expired() {
		t.Errorf("nil ExpiresAt must not report expired")
	}

	tokPast := Token{ExpiresAt: &past}
	if !tokPast.Expired() {
		t.Errorf("past ExpiresAt must report expired")
	}

	tokFuture := Token{ExpiresAt: &future}
	if tokFuture.Expired() {
		t.Errorf("future ExpiresAt must not report expired")
	}
}
