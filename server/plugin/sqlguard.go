package plugin

import (
	"fmt"
	"strings"
	"sync"
)

const MaxRows = 200

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
	"otp":           true,
	"subject":       true,
	"text":          true,
	"storage_key":   true,
	"fingerprint":   true,
}

const RedactedValue = "[redacted]"

var BannedKeywords = []string{
	"insert", "update", "delete", "drop", "alter", "create", "replace",
	"truncate", "attach", "detach", "pragma", "vacuum", "reindex",
	"grant", "revoke", "merge", "upsert",
}

var BannedIdentifiers = []string{
	"users", "user_sessions", "tokens", "provider_accounts", "settings",
	"workspace_settings", "org_members", "orgs", "webhooks",
	"password", "password_hash", "passwordhash", "hash", "secret",
	"private_key", "privatekey", "private_key_enc", "token",
	"access_token", "refresh_token", "totp_secret", "recovery_codes",
	"inbound_token", "invite_token", "otp",
}

var bannedRegistry struct {
	sync.RWMutex
	identifiers []string
}

func RegisterBannedIdentifiers(ids ...string) {
	bannedRegistry.Lock()
	defer bannedRegistry.Unlock()
	for _, id := range ids {
		bannedRegistry.identifiers = append(bannedRegistry.identifiers, strings.ToLower(id))
	}
}

func bannedIdentifiers() []string {
	bannedRegistry.RLock()
	defer bannedRegistry.RUnlock()
	out := make([]string, 0, len(BannedIdentifiers)+len(bannedRegistry.identifiers))
	out = append(out, BannedIdentifiers...)
	out = append(out, bannedRegistry.identifiers...)
	return out
}

func ValidateReadOnlyQuery(query string) (string, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";")
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("query is empty")
	}

	if strings.Contains(q, ";") {
		return "", fmt.Errorf("only a single statement is allowed (no ';')")
	}

	lower := strings.ToLower(q)

	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
		return "", fmt.Errorf("only SELECT (or WITH…SELECT) queries are allowed")
	}

	for _, kw := range BannedKeywords {
		if ContainsWord(lower, kw) {
			return "", fmt.Errorf("disallowed keyword %q in a read-only query", kw)
		}
	}

	for _, id := range bannedIdentifiers() {
		if ContainsWord(lower, id) {
			return "", fmt.Errorf("disallowed table/column %q in a read-only query", id)
		}
	}

	if !ContainsWord(lower, "limit") {
		q = fmt.Sprintf("%s LIMIT %d", q, MaxRows)
	}

	return q, nil
}

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

func RedactRow(columns []string, row map[string]any) {
	for _, c := range columns {
		if SensitiveColumns[strings.ToLower(c)] {
			if _, ok := row[c]; ok {
				row[c] = RedactedValue
			}
		}
	}
}
