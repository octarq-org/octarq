package models

import (
	"os"
	"strings"

	"gorm.io/gorm"
)

// BaseDomainSetting is the settings-table key holding the shared base domain
// under which every newly created org gets an automatic tenant subdomain:
// "<slug>.<base>". An empty value disables the feature entirely; so does an
// absent row — the shared base is a runtime decision the operator makes from
// Settings → Instance, not a boot-time constant.
const BaseDomainSetting = "base_domain"

// BaseDomainEnv is the bootstrap fallback for the base domain. It exists so a
// fresh deployment can start issuing tenant subdomains before anyone has saved
// the setting through the dashboard. The moment a settings row exists it
// governs, so an operator can turn the feature off by clearing the setting even
// while this variable is still set.
const BaseDomainEnv = "OCTARQ_BASE_DOMAIN"

// BaseDomain returns the configured tenant-subdomain base, normalized
// (lowercased, trimmed, trailing dot removed), or "" when the feature is off —
// either no settings row and no env value, or an explicit empty setting.
//
// It is the single source of truth for the base domain, read by every consumer
// (origin's host→org resolution, the dns plugin's reserved-zone checks, and the
// tenancy provisioning helper), so none of them can drift apart about what the
// base is.
func BaseDomain(db *gorm.DB) string {
	if db != nil {
		var s Setting
		if err := db.First(&s, "key = ?", BaseDomainSetting).Error; err == nil {
			return normalizeBaseDomain(s.Value)
		}
	}
	return normalizeBaseDomain(os.Getenv(BaseDomainEnv))
}

func normalizeBaseDomain(v string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
}
