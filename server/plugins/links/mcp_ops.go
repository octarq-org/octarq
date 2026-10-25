package links

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

type toggleShortlinkInput struct {
	ID      uint  `json:"id" jsonschema:"ID of the short link to toggle (required)"`
	Enabled *bool `json:"enabled,omitempty" jsonschema:"New enabled state; if omitted, toggles current state"`
}

func (p *Plugin) mcpToggleShortlink(ctx context.Context, _ *mcp.CallToolRequest, in toggleShortlinkInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.ID == 0 {
		return nil, nil, errors.New("id is required")
	}

	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil, nil, errors.New("database not available")
	}

	var l Link
	if err := tdb.WithContext(ctx).Where("id = ? AND owner_id = ?", in.ID, orgID).First(&l).Error; err != nil {
		return nil, nil, errors.New("link not found")
	}

	if in.Enabled != nil {
		l.Enabled = *in.Enabled
	} else {
		l.Enabled = !l.Enabled
	}

	if err := tdb.WithContext(ctx).Save(&l).Error; err != nil {
		return nil, nil, fmt.Errorf("toggle link: %w", err)
	}

	if p.publishEvent != nil {
		p.publishEvent(orgID, "link.update", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "enabled": l.Enabled})
	}
	if p.deleteCache != nil {
		if err := p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug); err != nil {
			log.Printf("links: failed to delete cache for %s:%s: %v", l.Host, l.Slug, err)
		}
	}

	out := linkOut{
		ID:        l.ID,
		Host:      l.Host,
		Slug:      l.Slug,
		Target:    l.Target,
		Title:     l.Title,
		Tags:      l.Tags,
		Clicks:    l.Clicks,
		Enabled:   l.Enabled,
		Archived:  l.Archived,
		CreatedAt: l.CreatedAt,
	}
	return jsonResult(out)
}

type getLinkAnalyticsInput struct {
	ID     uint   `json:"id" jsonschema:"ID of the short link (required)"`
	Days   int    `json:"days,omitempty" jsonschema:"Time window in days (default 30, max 365)"`
	Metric string `json:"metric,omitempty" jsonschema:"Metric type: 'uv' (unique visitors) or 'pv' (page views)"`
}

func (p *Plugin) mcpGetLinkAnalytics(ctx context.Context, _ *mcp.CallToolRequest, in getLinkAnalyticsInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.ID == 0 {
		return nil, nil, errors.New("id is required")
	}

	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil, nil, errors.New("database not available")
	}

	var l Link
	if err := tdb.WithContext(ctx).Where("id = ? AND owner_id = ?", in.ID, orgID).First(&l).Error; err != nil {
		return nil, nil, errors.New("link not found")
	}

	stats := p.queryLinkAnalytics(in.ID, l.RoutingRules, in.Days, in.Metric)
	return jsonResult(stats)
}

type deleteLinkInput struct {
	ID uint `json:"id" jsonschema:"ID of the short link to delete (required)"`
}

func (p *Plugin) mcpDeleteLink(ctx context.Context, _ *mcp.CallToolRequest, in deleteLinkInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.ID == 0 {
		return nil, nil, errors.New("id is required")
	}

	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil, nil, errors.New("database not available")
	}

	var l Link
	if err := tdb.WithContext(ctx).Where("id = ? AND owner_id = ?", in.ID, orgID).First(&l).Error; err != nil {
		return nil, nil, errors.New("link not found")
	}

	if err := tdb.WithContext(ctx).Delete(&l).Error; err != nil {
		return nil, nil, fmt.Errorf("delete link: %w", err)
	}

	if p.publishEvent != nil {
		p.publishEvent(orgID, "link.delete", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host})
	}
	if p.deleteCache != nil {
		if err := p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug); err != nil {
			log.Printf("links: failed to delete cache for %s:%s: %v", l.Host, l.Slug, err)
		}
	}

	return jsonResult(map[string]bool{"ok": true})
}

type batchCreateLinksInput struct {
	Links []createLinkInput `json:"links" jsonschema:"Array of short links to create"`
}

func (p *Plugin) mcpBatchCreateLinks(ctx context.Context, req *mcp.CallToolRequest, in batchCreateLinksInput) (*mcp.CallToolResult, any, error) {
	if len(in.Links) == 0 {
		return nil, nil, errors.New("links array is empty")
	}
	out := make([]linkOut, 0, len(in.Links))
	for _, item := range in.Links {
		_, raw, err := p.mcpCreateLink(ctx, req, item)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, raw.(linkOut))
	}
	return jsonResult(out)
}
