package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/led/internal/models"
	"github.com/octarq-org/led/plugin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestValidateReadOnlyQueryAccepts(t *testing.T) {
	cases := []string{
		"SELECT * FROM links",
		"select count(*) from emails",
		"WITH t AS (SELECT id FROM links) SELECT * FROM t",
		"SELECT * FROM links;", // trailing semicolon stripped
	}
	for _, q := range cases {
		if _, err := validateReadOnlyQuery(q); err != nil {
			t.Errorf("expected accept for %q, got %v", q, err)
		}
	}
}

func TestValidateReadOnlyQueryRejects(t *testing.T) {
	cases := []string{
		"",
		"DELETE FROM links",
		"UPDATE links SET clicks = 0",
		"INSERT INTO links (slug) VALUES ('x')",
		"DROP TABLE links",
		"PRAGMA table_info(links)",
		"ATTACH DATABASE 'x.db' AS y",
		"SELECT * FROM links; DROP TABLE links", // multi-statement
		"SELECT 1; SELECT 2",                    // multi-statement
		"VACUUM",
	}
	for _, q := range cases {
		if _, err := validateReadOnlyQuery(q); err == nil {
			t.Errorf("expected reject for %q, got nil error", q)
		}
	}
}

func TestValidateInjectsLimit(t *testing.T) {
	got, err := validateReadOnlyQuery("SELECT * FROM links")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToUpper(got), "LIMIT") {
		t.Errorf("expected LIMIT injected, got %q", got)
	}
	// Existing LIMIT preserved, not doubled.
	got2, _ := validateReadOnlyQuery("SELECT * FROM links LIMIT 5")
	if strings.Count(strings.ToUpper(got2), "LIMIT") != 1 {
		t.Errorf("LIMIT should not be doubled: %q", got2)
	}
}

func TestContainsWordBoundary(t *testing.T) {
	// "created" contains "create" as a substring but not as a word.
	if plugin.ContainsWord("select created_at from links", "create") {
		t.Error("containsWord matched 'create' inside 'created_at'")
	}
	if !plugin.ContainsWord("drop table x", "drop") {
		t.Error("containsWord missed standalone 'drop'")
	}
}

func TestRedactRow(t *testing.T) {
	cols := []string{"id", "email", "password_hash", "raw"}
	row := map[string]any{"id": 1, "email": "a@b.c", "password_hash": "deadbeef", "raw": "rfc822..."}
	redactRow(cols, row)
	if row["password_hash"] != plugin.RedactedValue || row["raw"] != plugin.RedactedValue {
		t.Errorf("sensitive columns not redacted: %+v", row)
	}
	if row["email"] != "a@b.c" {
		t.Errorf("non-sensitive column altered: %v", row["email"])
	}
}

// TestRunReadOnlyQueryRedactsSecrets is an end-to-end check: a SELECT * over a
// seeded users table must come back with the password hash redacted.
func TestRunReadOnlyQueryRedactsSecrets(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.User{}, &models.Link{}); err != nil {
		t.Fatal(err)
	}
	gdb.Create(&models.User{Email: "boss@co", PasswordHash: "TOPSECRET"})

	s := &server{gdb: gdb, orgID: 1}
	cols, rows, err := s.runReadOnlyQuery(context.Background(), "SELECT * FROM users")
	if err != nil {
		t.Fatalf("runReadOnlyQuery: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	_ = cols
	if rows[0]["password_hash"] != plugin.RedactedValue {
		t.Errorf("password_hash leaked: %v", rows[0]["password_hash"])
	}
	if rows[0]["email"] != "boss@co" {
		t.Errorf("email = %v", rows[0]["email"])
	}
}

// TestRunReadOnlyQueryRejectsWrite confirms the write is blocked before it
// reaches the DB.
func TestRunReadOnlyQueryRejectsWrite(t *testing.T) {
	gdb, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	gdb.AutoMigrate(&models.Link{})
	s := &server{gdb: gdb, orgID: 1}
	if _, _, err := s.runReadOnlyQuery(context.Background(), "DELETE FROM links"); err == nil {
		t.Error("expected write to be rejected")
	}
}
