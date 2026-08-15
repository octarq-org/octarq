package links

import (
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
