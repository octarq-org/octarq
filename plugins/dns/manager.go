package dns

import (
	"context"
	"errors"

	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/plugin"
)

// dnsManager adapts the plugin's per-domain DNS provider to the stable
// plugin.DNSManager seam, so Pro plugins (e.g. the AI MCP tools) can change real
// DNS records without importing internal/dnsprovider. It resolves each domain's
// zone and credentials via the same providerFor() the dashboard uses.
type dnsManager struct{ p *Plugin }

// DNSManager returns the plugin-facing DNS manager backed by this plugin. The
// app provides it as the "dns.manager" service and wires ctx.DNS to it.
func (p *Plugin) DNSManager() plugin.DNSManager { return dnsManager{p} }

// resolve loads a domain owned by orgID and builds its DNS provider. The org
// scope is the seam's own tenant boundary: a caller that forgets to check
// ownership gets "domain not found" here rather than a write into someone
// else's zone. An unset orgID is a caller bug, not a wildcard.
func (m dnsManager) resolve(orgID, domainID uint) (Domain, dnsprovider.Provider, error) {
	var dom Domain
	if orgID == 0 {
		return dom, nil, errOrgRequired
	}
	if err := m.p.db.Where("id = ? AND owner_id = ?", domainID, orgID).First(&dom).Error; err != nil {
		return dom, nil, err
	}
	prov, err := m.p.providerFor(dom)
	return dom, prov, err
}

// errOrgRequired is returned when a caller passes no org, which would otherwise
// silently widen the lookup to every tenant's domains.
var errOrgRequired = errors.New("dns: an org id is required to resolve a domain")

func (m dnsManager) List(ctx context.Context, orgID, domainID uint) ([]plugin.DNSRecord, error) {
	dom, prov, err := m.resolve(orgID, domainID)
	if err != nil {
		return nil, err
	}
	recs, err := prov.ListRecords(ctx, dom.ZoneID)
	if err != nil {
		return nil, err
	}
	out := make([]plugin.DNSRecord, len(recs))
	for i, r := range recs {
		out[i] = toPluginRecord(r)
	}
	return out, nil
}

func (m dnsManager) Set(ctx context.Context, orgID, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error) {
	dom, prov, err := m.resolve(orgID, domainID)
	if err != nil {
		return plugin.DNSRecord{}, err
	}
	rec := fromPluginRecord(r)
	var res dnsprovider.Record
	if r.ID == "" {
		res, err = prov.CreateRecord(ctx, dom.ZoneID, rec)
	} else {
		res, err = prov.UpdateRecord(ctx, dom.ZoneID, rec)
	}
	if err != nil {
		return plugin.DNSRecord{}, err
	}
	return toPluginRecord(res), nil
}

func (m dnsManager) Delete(ctx context.Context, orgID, domainID uint, recordID string) error {
	dom, prov, err := m.resolve(orgID, domainID)
	if err != nil {
		return err
	}
	return prov.DeleteRecord(ctx, dom.ZoneID, recordID)
}

func toPluginRecord(r dnsprovider.Record) plugin.DNSRecord {
	return plugin.DNSRecord{
		ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
		TTL: r.TTL, Proxied: r.Proxied, Comment: r.Comment, Priority: r.Priority,
	}
}

func fromPluginRecord(r plugin.DNSRecord) dnsprovider.Record {
	return dnsprovider.Record{
		ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
		TTL: r.TTL, Proxied: r.Proxied, Comment: r.Comment, Priority: r.Priority,
	}
}
