// Package plugin SQL guardrails for query_db_readonly.
package plugin

import (
	"fmt"
	"strings"
)

// MaxRows caps how many rows query_db_readonly returns.
const MaxRows = 200

// SensitiveColumns are result column names whose values are always redacted.
var SensitiveColumns = map[string]bool{
	"password":      true,
	"password_hash": true,
	"hash":          true,
	"config":        true,
	"raw":           true,
	"html":          true,
	"secret":        true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"private_key":   true,
}

// RedactedValue is what a sensitive column's value is replaced with.
const RedactedValue = "[redacted]"

// BannedKeywords are statement types that must never run through the read-only tool.
var BannedKeywords = []string{
	"insert", "update", "delete", "drop", "alter", "create", "replace",
	"truncate", "attach", "detach", "pragma", "vacuum", "reindex",
	"grant", "revoke", "merge", "upsert",
}

// BannedIdentifiers are table and column names a read-only query may never
// reference. Output-column redaction alone is bypassable (SELECT password_hash
// AS x hides the name), so we reject the query outright if it so much as
// mentions a secret-bearing table or column. Tables holding credentials,
// password/token hashes, or another tenant's account data are off-limits; the
// analytics tables (links, link_events, emails, domains, …) stay queryable.
var BannedIdentifiers = []string{
	// secret-bearing / cross-tenant tables
	"users", "user_sessions", "tokens", "provider_accounts", "settings",
	"workspace_settings", "org_members", "orgs", "product_keys",
	"issued_licenses", "license_devices", "webhooks",
	// secret-bearing columns (in case a JOIN or view exposes them)
	"password", "password_hash", "passwordhash", "hash", "secret",
	"private_key", "privatekey", "private_key_enc", "token",
	"access_token", "refresh_token", "totp_secret", "recovery_codes",
	"inbound_token", "invite_token",
}

// ValidateReadOnlyQuery checks a user-supplied SQL string against the guardrails
// and returns a normalized, LIMIT-bounded query ready to execute, or an error.
func ValidateReadOnlyQuery(query string) (string, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";")
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("query is empty")
	}

	// Reject anything with an embedded statement separator.
	if strings.Contains(q, ";") {
		return "", fmt.Errorf("only a single statement is allowed (no ';')")
	}

	lower := strings.ToLower(q)

	// Must be a read query.
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
		return "", fmt.Errorf("only SELECT (or WITH…SELECT) queries are allowed")
	}

	// Reject write / DDL / PRAGMA / ATTACH keywords.
	for _, kw := range BannedKeywords {
		if ContainsWord(lower, kw) {
			return "", fmt.Errorf("disallowed keyword %q in a read-only query", kw)
		}
	}

	// Reject any reference to a secret-bearing or cross-tenant table/column. This
	// closes the output-alias bypass of column-name redaction.
	for _, id := range BannedIdentifiers {
		if ContainsWord(lower, id) {
			return "", fmt.Errorf("disallowed table/column %q in a read-only query", id)
		}
	}

	// Inject a LIMIT when the query doesn't already constrain itself.
	if !ContainsWord(lower, "limit") {
		q = fmt.Sprintf("%s LIMIT %d", q, MaxRows)
	}

	return q, nil
}

// ContainsWord reports whether s contains word as a whole word.
func ContainsWord(s, word string) bool {
	idx := 0
	for {
		i := strings.Index(s[idx:], word)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(word)
		beforeOK := start == 0 || !isIdentChar(s[start-1])
		afterOK := end == len(s) || !isIdentChar(s[end])
		if beforeOK && afterOK {
			return true
		}
		idx = end
	}
}

func isIdentChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// RedactRow replaces the values of sensitive columns in a result row with RedactedValue.
func RedactRow(columns []string, row map[string]any) {
	for _, c := range columns {
		if SensitiveColumns[strings.ToLower(c)] {
			if _, ok := row[c]; ok {
				row[c] = RedactedValue
			}
		}
	}
}
