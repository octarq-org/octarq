package links

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type createLinkInput struct {
	Target  string `json:"target" jsonschema:"Destination URL (required)"`
	Host    string `json:"host,omitempty" jsonschema:"Custom domain host (e.g. go.example.com)"`
	Slug    string `json:"slug,omitempty" jsonschema:"Custom slug. If omitted, a random 6-character slug is generated"`
	Title   string `json:"title,omitempty" jsonschema:"Link title"`
	Tags    string `json:"tags,omitempty" jsonschema:"Comma-separated tags"`
	Note    string `json:"note,omitempty" jsonschema:"Administrative note"`
	Enabled *bool  `json:"enabled,omitempty" jsonschema:"Whether the link is enabled (default true)"`
}

func (p *Plugin) mcpCreateLink(ctx context.Context, _ *mcp.CallToolRequest, in createLinkInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}

	target := strings.TrimSpace(in.Target)
	if target == "" {
		return nil, nil, errors.New("target is required")
	}
	normalized, ok := normalizeTarget(target)
	if !ok {
		return nil, nil, errors.New("target must be an http(s) URL")
	}

	host := strings.TrimSpace(in.Host)
	if host != "" && !p.ownsHost(orgID, host) {
		return nil, nil, errors.New("host is not a link host of this workspace")
	}
	if host == "" && p.linkHostRequired(orgID) {
		return nil, nil, errors.New("host is required in multi-tenant mode")
	}

	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = models.RandomSlug(6)
	}
	if p.isReservedSlug(slug) {
		return nil, nil, errors.New("slug is reserved")
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	if err := p.checkQuota(ctx, orgID, "links", 1); err != nil {
		return nil, nil, err
	}

	l := Link{
		OrgID:    orgID,
		Host:     host,
		Slug:     slug,
		Target:   normalized,
		Title:    in.Title,
		Tags:     in.Tags,
		Note:     in.Note,
		Enabled:  enabled,
		Archived: false,
	}

	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil, nil, errors.New("database not available")
	}
	if err := tdb.WithContext(ctx).Create(&l).Error; err != nil {
		return nil, nil, fmt.Errorf("create link: %w", err)
	}

	if p.publishEvent != nil {
		p.publishEvent(orgID, "link.create", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
	}
	if p.deleteCache != nil {
		_ = p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug)
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

type updateLinkInput struct {
	ID       uint    `json:"id" jsonschema:"ID of the short link to update (required)"`
	Target   *string `json:"target,omitempty" jsonschema:"New destination URL"`
	Title    *string `json:"title,omitempty" jsonschema:"New link title"`
	Tags     *string `json:"tags,omitempty" jsonschema:"New comma-separated tags"`
	Note     *string `json:"note,omitempty" jsonschema:"New administrative note"`
	Enabled  *bool   `json:"enabled,omitempty" jsonschema:"Whether link is enabled"`
	Archived *bool   `json:"archived,omitempty" jsonschema:"Whether link is archived"`
}

func (p *Plugin) mcpUpdateLink(ctx context.Context, _ *mcp.CallToolRequest, in updateLinkInput) (*mcp.CallToolResult, any, error) {
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

	if in.Target != nil {
		target := strings.TrimSpace(*in.Target)
		if target != "" {
			normalized, ok := normalizeTarget(target)
			if !ok {
				return nil, nil, errors.New("target must be an http(s) URL")
			}
			l.Target = normalized
		}
	}
	if in.Title != nil {
		l.Title = *in.Title
	}
	if in.Tags != nil {
		l.Tags = *in.Tags
	}
	if in.Note != nil {
		l.Note = *in.Note
	}
	if in.Enabled != nil {
		l.Enabled = *in.Enabled
	}
	if in.Archived != nil {
		l.Archived = *in.Archived
	}

	if err := tdb.WithContext(ctx).Save(&l).Error; err != nil {
		return nil, nil, fmt.Errorf("update link: %w", err)
	}

	if p.publishEvent != nil {
		p.publishEvent(orgID, "link.update", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host})
	}
	if p.deleteCache != nil {
		_ = p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug)
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
		_ = p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug)
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
