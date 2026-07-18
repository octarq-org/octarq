package dns

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

type listDomainsInput struct{}

type domainOut struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	ForMail bool   `json:"forMail"`
	ForLink bool   `json:"forLink"`
	ZoneID  string `json:"zoneId"`
}

func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_domains",
		Description: "List managed domains and what each is used for (mail / links).",
	}, p.mcpListDomains)
}

func (p *Plugin) mcpListDomains(ctx context.Context, _ *mcp.CallToolRequest, _ listDomainsInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		orgID = 1
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
