package links

import (
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// normalizeHost delegates to the dns package, which owns the Domain rows every
// comparison here is made against — see dns.NormalizeHost.
func normalizeHost(host string) string { return dns.NormalizeHost(host) }

// ownsHost reports whether orgID may serve short links on host.
func (p *Plugin) ownsHost(orgID uint, host string) bool {
	normHost := normalizeHost(host)
	if normHost == "" {
		return true
	}
	if orgID == 0 || p.db == nil || !p.db.Migrator().HasTable("domains") {
		return false
	}
	var doms []dns.Domain
	if err := p.db.Where("owner_id = ? AND for_link = ?", orgID, true).Find(&doms).Error; err != nil {
		return false
	}
	for _, d := range doms {
		hosts := d.EffectiveLinkHosts()
		if len(hosts) == 0 {
			if normalizeHost(d.Name) == normHost {
				return true
			}
		} else {
			for _, h := range hosts {
				if normalizeHost(h) == normHost {
					return true
				}
			}
		}
	}
	return false
}

// resolveHostOrg returns the owner_id of the tenant that owns host via a for_link domain,
// or 0 if host belongs to no tenant (shared host, IP, or build without dns plugin).
//
// Note: Per internal/api/host_org.go, the Host header is client-controlled and must
// never be used for caller authorization. Here, Host is used solely as a public
// routing key to determine which tenant's brand namespace owns the hostname,
// ensuring links served on a tenant's custom domain belong to that same tenant.
func resolveHostOrg(db *gorm.DB, host string) uint {
	normHost := normalizeHost(host)
	if normHost == "" || db == nil || !db.Migrator().HasTable("domains") {
		return 0
	}
	var doms []dns.Domain
	if err := db.Where("for_link = ?", true).Find(&doms).Error; err != nil {
		return 0
	}
	for _, d := range doms {
		hosts := d.EffectiveLinkHosts()
		if len(hosts) == 0 {
			if normalizeHost(d.Name) == normHost {
				return d.OrgID
			}
		} else {
			for _, h := range hosts {
				if normalizeHost(h) == normHost {
					return d.OrgID
				}
			}
		}
	}
	return 0
}
