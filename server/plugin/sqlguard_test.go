package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestSQLGuardAndMetaHelpers(t *testing.T) {
	t.Parallel()

	ctx := WithOrgID(context.Background(), 42)
	if orgID := OrgIDFromContext(ctx); orgID != 42 {
		t.Errorf("OrgIDFromContext expected 42, got %d", orgID)
	}
	if orgID := OrgIDFromContext(context.Background()); orgID != 0 {
		t.Errorf("OrgIDFromContext expected 0, got %d", orgID)
	}

	if _, err := ValidateReadOnlyQuery(""); err == nil {
		t.Error("expected error for empty query")
	}
	if _, err := ValidateReadOnlyQuery("SELECT 1; SELECT 2"); err == nil {
		t.Error("expected error for multiple statements")
	}
	if _, err := ValidateReadOnlyQuery("UPDATE links SET clicks = 0"); err == nil {
		t.Error("expected error for UPDATE query")
	}
	if _, err := ValidateReadOnlyQuery("SELECT * FROM links DROP TABLE users"); err == nil {
		t.Error("expected error for DROP keyword")
	}
	if _, err := ValidateReadOnlyQuery("SELECT * FROM users"); err == nil {
		t.Error("expected error for banned identifier users")
	}

	validQ, err := ValidateReadOnlyQuery("SELECT * FROM links")
	if err != nil || !strings.Contains(validQ, "LIMIT 200") {
		t.Errorf("ValidateReadOnlyQuery failed to add LIMIT: %v, %s", err, validQ)
	}

	validQ2, err := ValidateReadOnlyQuery("SELECT * FROM links LIMIT 10")
	if err != nil || validQ2 != "SELECT * FROM links LIMIT 10" {
		t.Errorf("ValidateReadOnlyQuery altered explicit LIMIT: %v, %s", err, validQ2)
	}

	row := map[string]any{
		"id":       1,
		"password": "secretpass",
		"title":    "My Title",
	}
	RedactRow([]string{"id", "password", "title"}, row)
	if row["password"] != RedactedValue {
		t.Errorf("RedactRow failed to redact password, got %v", row["password"])
	}

	pCore := dummyPlugin{info: Info{Core: true, Group: "mygrp"}}
	pNonCore := dummyPlugin{info: Info{Core: false, Group: "other"}}
	if !FeatureIsCore([]Plugin{pCore, pNonCore}, "mygrp") {
		t.Error("FeatureIsCore expected true for mygrp")
	}
	if FeatureIsCore([]Plugin{pCore, pNonCore}, "other") {
		t.Error("FeatureIsCore expected false for other")
	}
}

type dummyPlugin struct {
	info Info
}

func (d dummyPlugin) Name() string        { return "dummy" }
func (d dummyPlugin) Describe() Info      { return d.info }
func (d dummyPlugin) Models() []any       { return nil }
func (d dummyPlugin) Menus() []MenuItem   { return nil }
func (d dummyPlugin) Actions() []Action   { return nil }
func (d dummyPlugin) Mount(Mux, *Context) {}

func TestValidateReadOnlyQueryAccepts(t *testing.T) {
	t.Parallel()

	cases := []string{
		"SELECT * FROM links",
		"select count(*) from emails",
		"WITH t AS (SELECT id FROM links) SELECT * FROM t",
		"SELECT * FROM links;",
	}
	for _, q := range cases {
		if _, err := ValidateReadOnlyQuery(q); err != nil {
			t.Errorf("expected accept for %q, got %v", q, err)
		}
	}
}

func TestValidateReadOnlyQueryRejects(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"DELETE FROM links",
		"UPDATE links SET clicks = 0",
		"INSERT INTO links (slug) VALUES ('x')",
		"DROP TABLE links",
		"PRAGMA table_info(links)",
		"ATTACH DATABASE 'x.db' AS y",
		"SELECT * FROM links; DROP TABLE links",
		"SELECT 1; SELECT 2",
		"VACUUM",
		"SELECT * FROM users",
		"SELECT * FROM tokens",
		"SELECT config FROM provider_accounts",
		"SELECT password_hash AS x FROM users",
		"SELECT * FROM emails JOIN users ON 1=1",
	}
	for _, q := range cases {
		if _, err := ValidateReadOnlyQuery(q); err == nil {
			t.Errorf("expected reject for %q, got nil error", q)
		}
	}
}

func TestContainsWordBoundary(t *testing.T) {
	t.Parallel()

	if ContainsWord("select created_at from links", "create") {
		t.Error("containsWord matched 'create' inside 'created_at'")
	}
	if !ContainsWord("drop table x", "drop") {
		t.Error("containsWord missed standalone 'drop'")
	}
}

func TestValidateReadOnlyQueryBlocksMailAndOTPIdentifiers(t *testing.T) {
	t.Parallel()

	for _, q := range []string{
		"SELECT otp, subject FROM email_ais",
		"SELECT * FROM email_ais",
		"SELECT otp FROM links",
	} {
		if _, err := ValidateReadOnlyQuery(q); err == nil {
			t.Errorf("expected reject for %q, got nil error", q)
		}
	}
}

func TestRedactRowRedactsMessageContentAndOTP(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "email", "password_hash", "raw", "subject", "text", "otp"}
	row := map[string]any{"id": 1, "email": "a@b.c", "password_hash": "deadbeef", "raw": "rfc822...", "subject": "Your code", "text": "code is 123456", "otp": "123456"}
	RedactRow(cols, row)
	for _, c := range []string{"password_hash", "raw", "subject", "text", "otp"} {
		if row[c] != RedactedValue {
			t.Errorf("column %q not redacted: %v", c, row[c])
		}
	}
	if row["id"] != 1 {
		t.Errorf("non-sensitive column altered: %v", row["id"])
	}
	if row["email"] != "a@b.c" {
		t.Errorf("non-sensitive column altered: %v", row["email"])
	}
}
