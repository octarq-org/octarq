package links

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

// errNoOrgInContext refuses a tool call that arrives without a tenant scope.
// Every transport supplies one — the networked ones from the caller's API token,
// the stdio CLI from its entry point — so an absent org means something is wrong,
// and defaulting it to a tenant would hand that tenant's data to whoever asked.
var errNoOrgInContext = plugin.NewAgentError(
	401,
	"UNAUTHORIZED_NO_WORKSPACE",
	"no workspace in this request",
	"Every tool execution requires a valid workspace/tenant context. Ensure a valid API token or session is provided.",
	false,
)

type listLinksInput struct {
	Host  string `json:"host,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type linkOut struct {
	ID        uint      `json:"id"`
	Host      string    `json:"host"`
	Slug      string    `json:"slug"`
	Target    string    `json:"target"`
	Title     string    `json:"title"`
	Tags      string    `json:"tags"`
	Clicks    int64     `json:"clicks"`
	Enabled   bool      `json:"enabled"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"createdAt"`
}

func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_links",
		Description: "List short links with their click counts. Optionally filter by host or tag, and limit the count.",
	}, p.mcpListLinks)
}

func (p *Plugin) mcpListLinks(ctx context.Context, _ *mcp.CallToolRequest, in listLinksInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}
	q := p.db.WithContext(ctx).Model(&Link{}).
		Where("owner_id = ?", orgID)
	if in.Host != "" {
		q = q.Where("host = ?", in.Host)
	}
	if tag := strings.TrimSpace(in.Tag); tag != "" {
		q = filterByTag(q, tag)
	}
	q = q.Order("clicks DESC").Limit(limit)
	var links []Link
	if err := q.Find(&links).Error; err != nil {
		return nil, nil, err
	}

	out := make([]linkOut, len(links))
	for i, l := range links {
		out[i] = linkOut{
			ID: l.ID, Host: l.Host, Slug: l.Slug, Target: l.Target,
			Title: l.Title, Tags: l.Tags, Clicks: l.Clicks,
			Enabled: l.Enabled, Archived: l.Archived, CreatedAt: l.CreatedAt,
		}
	}
	return jsonResult(out)
}

func (p *Plugin) mcpExportLinks(ctx context.Context, orgID uint) (any, error) {
	var v []Link
	if err := p.db.WithContext(ctx).Where("owner_id = ?", orgID).Find(&v).Error; err != nil {
		return nil, err
	}
	return v, nil
}

func tagsContain(field, tag string) bool {
	if tag == "" {
		return true
	}
	for _, t := range strings.Split(field, ",") {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
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
