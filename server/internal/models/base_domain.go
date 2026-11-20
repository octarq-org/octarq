package models

import (
	"os"
	"strings"

	"gorm.io/gorm"
)

const BaseDomainSetting = "base_domain"

const BaseDomainEnv = "OCTARQ_BASE_DOMAIN"

func BaseDomain(db *gorm.DB) string {
	if db != nil && db.Migrator().HasTable("settings") {
		var s Setting
		if err := db.Where("key = ?", BaseDomainSetting).Limit(1).Find(&s).Error; err == nil && s.Key != "" {
			return normalizeBaseDomain(s.Value)
		}
	}
	return normalizeBaseDomain(os.Getenv(BaseDomainEnv))
}

func normalizeBaseDomain(v string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
}
