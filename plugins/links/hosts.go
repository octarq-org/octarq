package links

import (
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
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

// linkHostRequired reports whether hostless (host = "") links must be refused
// for orgID: a shared base domain is configured AND the org has at least one
// link host to pick from. There the empty host means the cross-tenant shared
// namespace — tenant A's slug silently blocks tenant B, and the 409 acts as an
// existence probe across orgs. A self-hosted single-tenant install (no base
// domain) keeps its neutral-host links, which scopeForHost still serves.
func (p *Plugin) linkHostRequired(orgID uint) bool {
	if orgID == 0 || p.db == nil || !p.db.Migrator().HasTable("domains") {
		return false
	}
	if models.BaseDomain(p.db) == "" {
		return false
	}
	var n int64
	p.db.Model(&dns.Domain{}).Where("owner_id = ? AND for_link = ?", orgID, true).Count(&n)
	return n > 0
}
