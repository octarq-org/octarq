package dns

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// normalizeHost cleans a user-supplied host into a bare lowercase hostname
// (no scheme, no path, no trailing dot).
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	host = strings.TrimSuffix(host, ".")
	if i := strings.IndexAny(host, "/:"); i >= 0 {
		host = host[:i]
	}
	return host
}

// underBaseZone reports whether name is the reserved tenant-subdomain base
// itself or a subdomain of it, when one is configured.
func underBaseZone(db *gorm.DB, name string) bool {
	base := models.BaseDomain(db)
	if base == "" {
		return false
	}
	name = normalizeHost(name)
	return name == base || strings.HasSuffix(name, "."+base)
}

// reservedInHostLists returns the first hostname across linkHosts and mailHosts
// that lies inside the reserved tenant-subdomain base zone, or "" when none
// does.
func reservedInHostLists(db *gorm.DB, linkHosts, mailHosts models.HostList) string {
	base := models.BaseDomain(db)
	if base == "" {
		return ""
	}
	for _, list := range []models.HostList{linkHosts, mailHosts} {
		for _, h := range list {
			host := normalizeHost(h.Host)
			if host == base || strings.HasSuffix(host, "."+base) {
				return host
			}
		}
	}
	return ""
}

// hostsOutsideZone returns the first hostname across linkHosts and mailHosts
// that is neither the domain's own name nor a subdomain of it, or "" when
// every entry is inside the zone.
func hostsOutsideZone(name string, linkHosts, mailHosts models.HostList) string {
	name = NormalizeHost(name)
	if name == "" {
		return ""
	}
	for _, list := range []models.HostList{linkHosts, mailHosts} {
		for _, h := range list {
			host := NormalizeHost(h.Host)
			if host != name && !strings.HasSuffix(host, "."+name) {
				return host
			}
		}
	}
	return ""
}

// hostEntry is a host with its enable flag in create/update payloads.
type hostEntry struct {
	Host    string `json:"host"`
	Enabled *bool  `json:"enabled"`
}

// normalizeHosts cleans and de-duplicates a host list, preserving each host's
// enabled flag (defaulting to enabled).
func normalizeHosts(hosts []hostEntry) models.HostList {
	seen := map[string]bool{}
	var out models.HostList
	for _, h := range hosts {
		name := normalizeHost(h.Host)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		enabled := true
		if h.Enabled != nil {
			enabled = *h.Enabled
		}
		out = append(out, models.Host{Host: name, Enabled: enabled})
	}
	return out
}

// ownsProviderAccount reports whether the given provider account id belongs to
// the caller's org. Guards against binding another tenant's DNS credentials.
func (p *Plugin) ownsProviderAccount(r *http.Request, id uint) bool {
	if id == 0 {
		return false
	}
	var acc ProviderAccount
	return p.db.Where("id = ? AND owner_id = ?", id, p.orgID(r)).First(&acc).Error == nil
}

type DNSProvidersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *DNSProvidersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DNSProvidersOutput struct {
	Body []string
}

func (p *Plugin) dnsProviders(ctx context.Context, input *DNSProvidersInput) (*DNSProvidersOutput, error) {
	return &DNSProvidersOutput{Body: dnsprovider.Names()}, nil
}
