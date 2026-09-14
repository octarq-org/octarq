package models

import (
	"os"
	"strings"

	"gorm.io/gorm"
)

// SharedHostsSetting is the settings-table key holding the instance-wide
// shared hostnames (comma or newline separated) trusted for public origin derivation.
const SharedHostsSetting = "shared_hosts"

// SharedHostsEnv is the bootstrap fallback environment variable.
const SharedHostsEnv = "OCTARQ_SHARED_HOSTS"

// SharedHosts returns the configured instance-wide shared hostnames, normalized
// (lowercased, trimmed, port removed, trailing dot removed, de-duplicated).
// It reads from the settings table first; if not found or empty, it falls back
// to the OCTARQ_SHARED_HOSTS environment variable.
func SharedHosts(db *gorm.DB) []string {
	var raw string
	if db != nil && db.Migrator().HasTable("settings") {
		var s Setting
		if err := db.First(&s, "key = ?", SharedHostsSetting).Error; err == nil {
			raw = s.Value
		}
	}
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv(SharedHostsEnv)
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return ParseSharedHosts(raw)
}

// ParseSharedHosts parses and normalizes a comma/newline/space-separated list of hostnames.
func ParseSharedHosts(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	seen := make(map[string]bool)
	var out []string
	for _, f := range fields {
		norm := normalizeHost(f)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	return out
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}
