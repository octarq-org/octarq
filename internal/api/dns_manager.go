package api

import (
	"context"

	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// dnsManager adapts the core's per-domain DNS provider to the stable
// plugin.DNSManager seam, so Pro plugins (e.g. the AI MCP tools) can change real
// DNS records without importing internal/dnsprovider. It resolves each domain's
// zone and credentials via the same providerFor() the dashboard uses.
type dnsManager struct{ h *Handler }

// DNSManager returns the plugin-facing DNS manager backed by this handler.
func (h *Handler) DNSManager() plugin.DNSManager { return dnsManager{h} }

// resolve loads a domain and builds its DNS provider.
func (m dnsManager) resolve(domainID uint) (models.Domain, dnsprovider.Provider, error) {
	var dom models.Domain
	if err := m.h.db.First(&dom, domainID).Error; err != nil {
		return dom, nil, err
	}
	prov, err := m.h.providerFor(dom)
	return dom, prov, err
}

func (m dnsManager) List(ctx context.Context, domainID uint) ([]plugin.DNSRecord, error) {
	dom, prov, err := m.resolve(domainID)
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

func (m dnsManager) Set(ctx context.Context, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error) {
	dom, prov, err := m.resolve(domainID)
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

func (m dnsManager) Delete(ctx context.Context, domainID uint, recordID string) error {
	dom, prov, err := m.resolve(domainID)
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
