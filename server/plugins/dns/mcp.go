package dns

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/dnsprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

// errNoOrgInContext refuses a tool call that arrives without a tenant scope.
// Every transport supplies one — the networked ones from the caller's API token,
// the stdio CLI from its entry point — so an absent org means something is wrong,
// and defaulting it to a tenant would hand that tenant's data to whoever asked.
var errNoOrgInContext = errors.New("no workspace in this request")

type listDomainsInput struct{}

type domainOut struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	ForMail bool   `json:"forMail"`
	ForLink bool   `json:"forLink"`
	ZoneID  string `json:"zoneId"`
}

func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	plugin.AddMCPTool(srv, "dns", &mcp.Tool{
		Name:        "octarq_network__list_domains",
		Description: "List managed domains and what each is used for (mail / links).",
	}, p.mcpListDomains)

	plugin.AddMCPTool(srv, "dns", &mcp.Tool{
		Name:        "octarq_network__update_domain_config",
		Description: "Update master config toggles for a managed domain (forMail, forLink, note) by its ID.",
	}, p.mcpUpdateDomainConfig)

	plugin.AddMCPTool(srv, "dns", &mcp.Tool{
		Name:        "octarq_network__list_dns_records",
		Description: "List the DNS records of a managed domain (by domain ID) from its live DNS provider.",
	}, p.mcpListDNSRecords)

	plugin.AddMCPTool(srv, "dns", &mcp.Tool{
		Name: "octarq_network__set_dns_record",
		Description: "Create or update a real DNS record on a managed domain's provider (e.g. point an A record at a new IP). " +
			"Omit recordId to create; pass it to update. type is A/AAAA/CNAME/TXT/MX/…, name is the record name, content is the value.",
	}, p.mcpSetDNSRecord)

	plugin.AddMCPTool(srv, "dns", &mcp.Tool{
		Name:        "octarq_network__delete_dns_record",
		Description: "Delete a DNS record from a managed domain's provider by domain ID and provider record ID.",
	}, p.mcpDeleteDNSRecord)
}

type updateDomainConfigInput struct {
	ID      uint   `json:"id" jsonschema:"Domain ID (required)"`
	ForMail *bool  `json:"forMail,omitempty" jsonschema:"Enable email for domain"`
	ForLink *bool  `json:"forLink,omitempty" jsonschema:"Enable short links for domain"`
	Note    string `json:"note,omitempty" jsonschema:"Administrative note"`
}

func (p *Plugin) mcpUpdateDomainConfig(ctx context.Context, _ *mcp.CallToolRequest, in updateDomainConfigInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.ID == 0 {
		return nil, nil, errors.New("id is required")
	}
	var dom Domain
	if err := p.db.WithContext(ctx).Where("id = ? AND owner_id = ?", in.ID, orgID).First(&dom).Error; err != nil {
		return nil, nil, errors.New("domain not found")
	}
	if in.ForMail != nil {
		dom.ForMail = *in.ForMail
	}
	if in.ForLink != nil {
		dom.ForLink = *in.ForLink
	}
	if in.Note != "" {
		dom.Note = in.Note
	}
	if err := p.db.WithContext(ctx).Save(&dom).Error; err != nil {
		return nil, nil, err
	}
	return jsonResult(domainOut{ID: dom.ID, Name: dom.Name, ForMail: dom.ForMail, ForLink: dom.ForLink, ZoneID: dom.ZoneID})
}

type listDNSRecordsInput struct {
	DomainID uint `json:"domainId" jsonschema:"Domain ID to list DNS records from (required)"`
}

func (p *Plugin) mcpListDNSRecords(ctx context.Context, _ *mcp.CallToolRequest, in listDNSRecordsInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.DomainID == 0 {
		return nil, nil, errors.New("domainId is required")
	}
	prov, dom, err := p.recordsProviderForOrg(orgID, in.DomainID)
	if err != nil {
		return nil, nil, err
	}
	if dom.ZoneID == "" {
		return nil, nil, errors.New("domain has no Zone ID configured")
	}
	recs, err := prov.ListRecords(ctx, dom.ZoneID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(recs)
}

type setDNSRecordInput struct {
	DomainID uint   `json:"domainId" jsonschema:"Domain ID (required)"`
	RecordID string `json:"recordId,omitempty" jsonschema:"Optional record ID to update (omit to create)"`
	Type     string `json:"type" jsonschema:"Record type (A, AAAA, CNAME, TXT, MX, etc.)"`
	Name     string `json:"name" jsonschema:"Record name or subdomain"`
	Content  string `json:"content" jsonschema:"Record content or target IP/value"`
	TTL      int    `json:"ttl,omitempty" jsonschema:"TTL in seconds"`
	Proxied  bool   `json:"proxied,omitempty" jsonschema:"Whether Cloudflare proxy is enabled"`
	Comment  string `json:"comment,omitempty" jsonschema:"Optional comment"`
	Priority *int   `json:"priority,omitempty" jsonschema:"Priority for MX/SRV records"`
}

func (p *Plugin) mcpSetDNSRecord(ctx context.Context, _ *mcp.CallToolRequest, in setDNSRecordInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.DomainID == 0 {
		return nil, nil, errors.New("domainId is required")
	}
	prov, dom, err := p.recordsProviderForOrg(orgID, in.DomainID)
	if err != nil {
		return nil, nil, err
	}
	if dom.ZoneID == "" {
		return nil, nil, errors.New("domain has no Zone ID configured")
	}
	rec := dnsprovider.Record{
		ID:       in.RecordID,
		Type:     strings.ToUpper(strings.TrimSpace(in.Type)),
		Name:     strings.TrimSpace(in.Name),
		Content:  strings.TrimSpace(in.Content),
		TTL:      in.TTL,
		Proxied:  in.Proxied,
		Comment:  in.Comment,
		Priority: in.Priority,
	}
	if msg := validateRecord(rec); msg != "" {
		return nil, nil, errors.New(msg)
	}
	var out dnsprovider.Record
	if in.RecordID == "" {
		created, err := prov.CreateRecord(ctx, dom.ZoneID, rec)
		if err != nil {
			return nil, nil, err
		}
		out = created
	} else {
		rec.ID = in.RecordID
		updated, err := prov.UpdateRecord(ctx, dom.ZoneID, rec)
		if err != nil {
			return nil, nil, err
		}
		out = updated
	}
	return jsonResult(out)
}

type deleteDNSRecordInput struct {
	DomainID uint   `json:"domainId" jsonschema:"Domain ID (required)"`
	RecordID string `json:"recordId" jsonschema:"DNS provider record ID to delete (required)"`
}

func (p *Plugin) mcpDeleteDNSRecord(ctx context.Context, _ *mcp.CallToolRequest, in deleteDNSRecordInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.DomainID == 0 {
		return nil, nil, errors.New("domainId is required")
	}
	if strings.TrimSpace(in.RecordID) == "" {
		return nil, nil, errors.New("recordId is required")
	}
	prov, dom, err := p.recordsProviderForOrg(orgID, in.DomainID)
	if err != nil {
		return nil, nil, err
	}
	if dom.ZoneID == "" {
		return nil, nil, errors.New("domain has no Zone ID configured")
	}
	if err := prov.DeleteRecord(ctx, dom.ZoneID, in.RecordID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]bool{"ok": true})
}

func (p *Plugin) mcpListDomains(ctx context.Context, _ *mcp.CallToolRequest, _ listDomainsInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	var doms []Domain
	if err := p.db.WithContext(ctx).
		Where("owner_id = ?", orgID).
		Order("name ASC").Find(&doms).Error; err != nil {
		return nil, nil, err
	}
	out := make([]domainOut, 0, len(doms))
	for _, d := range doms {
		out = append(out, domainOut{ID: d.ID, Name: d.Name, ForMail: d.ForMail, ForLink: d.ForLink, ZoneID: d.ZoneID})
	}
	return jsonResult(out)
}

func (p *Plugin) mcpExportDomains(ctx context.Context, orgID uint) (any, error) {
	var v []Domain
	if err := p.db.WithContext(ctx).Where("owner_id = ?", orgID).Find(&v).Error; err != nil {
		return nil, err
	}
	return v, nil
}

func jsonResult[T any](v T) (*mcp.CallToolResult, any, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
	}, v, nil
}
