package dns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/dnsprovider"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
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
// the caller's org. Exactly one registry answers: the Integrations Hub when
// mounted, the local provider_accounts table on standalone Core. The second
// return reports a resolution failure (500-class), which is never "not owned".
func (p *Plugin) ownsProviderAccount(r *http.Request, id uint) (bool, error) {
	if id == 0 {
		return false, nil
	}
	orgID := p.orgID(r)
	cred, err := p.credentialHost().LookupCredential(r.Context(), orgID, plugin.CredentialQuery{
		ID:           id,
		ProviderType: "dns",
	})
	switch {
	case err == nil:
		return cred != nil && cred.OrgID == orgID, nil
	case errors.Is(err, plugin.ErrCredentialNotFound):
		return false, nil
	case !errors.Is(err, plugin.ErrCredentialResolverUnavailable):
		return false, fmt.Errorf("%w: %v", errProviderCredentials, err)
	}

	var acc ProviderAccount
	if err := p.db.Where("id = ? AND owner_id = ?", id, orgID).First(&acc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", errProviderCredentials, err)
	}
	return true, nil
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
