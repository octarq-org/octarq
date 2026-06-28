// SQL guardrails for the query_db_readonly MCP tool.
//
// This is the single place in led that hands raw database content to an LLM, so
// it is defense-in-depth, exactly as the AI roadmap mandates (§2 强制护栏):
//
//  1. Read-only: only a single SELECT / WITH…SELECT statement is accepted; any
//     write / DDL / PRAGMA / ATTACH keyword is rejected, and the query runs
//     inside a read-only transaction.
//  2. Sensitive-column redaction: values in known-secret columns
//     (users.password_hash, tokens.hash, provider_accounts.config, emails.raw /
//     emails.html, links.password, and obvious token/secret names) are replaced
//     with "[redacted]" in results, even on SELECT *.
//  3. Row / byte caps: a LIMIT is injected when absent and results are bounded,
//     so the model can never pull the whole database into its context.
//
// Cross-tenant scoping for arbitrary SQL cannot be enforced by rewriting the
// query, so the operator-facing contract is: `led mcp` runs scoped to one
// operator's data (LED_MCP_ORG_ID); on a multi-tenant deployment, run a
// separate MCP process per tenant. The convenience tools (list_*) DO inject an
// owner_id filter; this raw-SQL escape hatch trades that for flexibility and
// relies on read-only + redaction + caps.
package mcp

import (
	"fmt"
	"strings"
)

// maxRows caps how many rows query_db_readonly returns. The model gets enough
// to analyze without dumping a table into context.
const maxRows = 200

// sensitiveColumns are result column names whose values are always redacted,
// regardless of the table they came from. Matched case-insensitively against
// the result column name (so `password_hash`, `u.password_hash AS password_hash`
// etc. are all caught).
var sensitiveColumns = map[string]bool{
	"password":      true, // links.password, users.password
	"password_hash": true, // users.password_hash
	"hash":          true, // tokens.hash
	"config":        true, // provider_accounts.config (AES-encrypted creds)
	"raw":           true, // emails.raw (full RFC822)
	"html":          true, // emails.html
	"secret":        true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"private_key":   true,
}

// redactedValue is what a sensitive column's value is replaced with.
const redactedValue = "[redacted]"

// bannedKeywords are statement types that must never run through the read-only
// tool. Matched as whole words, case-insensitively.
var bannedKeywords = []string{
	"insert", "update", "delete", "drop", "alter", "create", "replace",
	"truncate", "attach", "detach", "pragma", "vacuum", "reindex",
	"grant", "revoke", "merge", "upsert",
}

// validateReadOnlyQuery checks a user-supplied SQL string against the guardrails
// and returns a normalized, LIMIT-bounded query ready to execute, or an error
// explaining the rejection.
func validateReadOnlyQuery(query string) (string, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";")
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("query is empty")
	}

	// Reject anything with an embedded statement separator — a single statement
	// only (the trailing one was already stripped above).
	if strings.Contains(q, ";") {
		return "", fmt.Errorf("only a single statement is allowed (no ';')")
	}

	lower := strings.ToLower(q)

	// Must be a read query.
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
		return "", fmt.Errorf("only SELECT (or WITH…SELECT) queries are allowed")
	}

	// Reject write / DDL / PRAGMA / ATTACH keywords as whole words.
	for _, kw := range bannedKeywords {
		if containsWord(lower, kw) {
			return "", fmt.Errorf("disallowed keyword %q in a read-only query", kw)
		}
	}

	// Inject a LIMIT when the query doesn't already constrain itself, so a bare
	// `SELECT * FROM link_events` can't stream millions of rows.
	if !containsWord(lower, "limit") {
		q = fmt.Sprintf("%s LIMIT %d", q, maxRows)
	}

	return q, nil
}

// containsWord reports whether s contains word as a whole word (bounded by
// non-alphanumeric characters or the string ends). s and word are both expected
// lowercased.
func containsWord(s, word string) bool {
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

// redactRow replaces the values of sensitive columns in a result row with
// redactedValue. columns and the row are positionally aligned.
func redactRow(columns []string, row map[string]any) {
	for _, c := range columns {
		if sensitiveColumns[strings.ToLower(c)] {
			if _, ok := row[c]; ok {
				row[c] = redactedValue
			}
		}
	}
}
