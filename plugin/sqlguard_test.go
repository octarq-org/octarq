package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestSQLGuardAndMetaHelpers(t *testing.T) {
	t.Parallel()

	// 1. WithOrgID & OrgIDFromContext
	ctx := WithOrgID(context.Background(), 42)
	if orgID := OrgIDFromContext(ctx); orgID != 42 {
		t.Errorf("OrgIDFromContext expected 42, got %d", orgID)
	}
	if orgID := OrgIDFromContext(context.Background()); orgID != 0 {
		t.Errorf("OrgIDFromContext expected 0, got %d", orgID)
	}

	// 2. ValidateReadOnlyQuery
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

	// 3. RedactRow
	row := map[string]any{
		"id":       1,
		"password": "secretpass",
		"title":    "My Title",
	}
	RedactRow([]string{"id", "password", "title"}, row)
	if row["password"] != RedactedValue {
		t.Errorf("RedactRow failed to redact password, got %v", row["password"])
	}

	// 4. FeatureIsCore
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

// The identifiers below were reachable before this guard existed: `email_ais`
// (a Pro table) and its `otp` column let a raw SELECT dump other tenants' one-
// time codes, and `subject`/`text` returned other tenants' plaintext mail.
// ValidateReadOnlyQuery is only a content filter, but out-of-tree plugins call
// it, so these must stay closed.
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

// Redaction is the second line: a column that slips through the identifier check
// (a view, a computed alias) must still not return message contents.
func TestRedactRowRedactsMessageContentAndOTP(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "subject", "text", "otp"}
	row := map[string]any{"id": 1, "subject": "Your code", "text": "code is 123456", "otp": "123456"}
	RedactRow(cols, row)
	for _, c := range []string{"subject", "text", "otp"} {
		if row[c] != RedactedValue {
			t.Errorf("column %q not redacted: %v", c, row[c])
		}
	}
	if row["id"] != 1 {
		t.Errorf("non-sensitive column altered: %v", row["id"])
	}
}
