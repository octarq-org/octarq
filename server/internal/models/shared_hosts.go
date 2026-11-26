package models

import (
	"os"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const SharedHostsSetting = "shared_hosts"

const SharedHostsEnv = "OCTARQ_SHARED_HOSTS"

func SharedHosts(db *gorm.DB) []string {
	var raw string
	if db != nil && db.Migrator().HasTable("settings") {
		var s Setting
		if err := db.Where("key = ?", SharedHostsSetting).Limit(1).Find(&s).Error; err == nil && s.Key != "" {
			raw = s.Value
		}
		if strings.TrimSpace(raw) == "" {
			raw = os.Getenv(SharedHostsEnv)
			if strings.TrimSpace(raw) != "" {
				db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Key: SharedHostsSetting, Value: raw})
			}
		}
	} else {
		raw = os.Getenv(SharedHostsEnv)
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return ParseSharedHosts(raw)
}

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
