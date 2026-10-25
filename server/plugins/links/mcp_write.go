package links

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

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

type updateLinkInput struct {
	ID           uint          `json:"id" jsonschema:"ID of the short link to update (required)"`
	Target       *string       `json:"target,omitempty" jsonschema:"New destination URL"`
	Slug         *string       `json:"slug,omitempty" jsonschema:"New slug"`
	Host         *string       `json:"host,omitempty" jsonschema:"New host domain"`
	Title        *string       `json:"title,omitempty" jsonschema:"New link title"`
	Tags         *string       `json:"tags,omitempty" jsonschema:"New comma-separated tags"`
	Note         *string       `json:"note,omitempty" jsonschema:"New administrative note"`
	Password     *string       `json:"password,omitempty" jsonschema:"Access protection password"`
	ClickLimit   *int64        `json:"clickLimit,omitempty" jsonschema:"Total click limit (0 = unlimited)"`
	ExpiresAt    *string       `json:"expiresAt,omitempty" jsonschema:"Expiration timestamp in RFC3339 format (empty string clears expiry)"`
	ExpiredURL   *string       `json:"expiredUrl,omitempty" jsonschema:"Redirect URL after expiration or reaching click limit"`
	RoutingRules *RoutingRules `json:"routingRules,omitempty" jsonschema:"Conditional routing rules array"`
	Enabled      *bool         `json:"enabled,omitempty" jsonschema:"Whether link is enabled"`
	Archived     *bool         `json:"archived,omitempty" jsonschema:"Whether link is archived"`
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

	oldHost := l.Host
	oldSlug := l.Slug

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
	if in.Slug != nil {
		slug := strings.TrimSpace(*in.Slug)
		if slug != "" {
			if slug != l.Slug && p.isReservedSlug(slug) {
				return nil, nil, errors.New("slug is reserved")
			}
			l.Slug = slug
		}
	}
	if in.Host != nil {
		host := strings.TrimSpace(*in.Host)
		if host != "" && !p.ownsHost(orgID, host) {
			return nil, nil, errors.New("host is not a link host of this workspace")
		}
		if host == "" && p.linkHostRequired(orgID) {
			return nil, nil, errors.New("host is required in multi-tenant mode")
		}
		l.Host = host
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
	if in.Password != nil {
		l.Password = *in.Password
	}
	if in.ClickLimit != nil {
		l.ClickLimit = *in.ClickLimit
	}
	if in.ExpiresAt != nil {
		if *in.ExpiresAt == "" {
			l.ExpiresAt = nil
		} else {
			expTime, err := time.Parse(time.RFC3339, *in.ExpiresAt)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid expiresAt format (expected RFC3339): %w", err)
			}
			l.ExpiresAt = &expTime
		}
	}
	if in.ExpiredURL != nil {
		expURL := strings.TrimSpace(*in.ExpiredURL)
		if expURL != "" {
			normExp, ok := normalizeTarget(expURL)
			if !ok {
				return nil, nil, errors.New("expiredUrl must be an http(s) URL")
			}
			l.ExpiredURL = normExp
		} else {
			l.ExpiredURL = ""
		}
	}
	if in.RoutingRules != nil {
		l.RoutingRules = *in.RoutingRules
	}
	if in.Enabled != nil {
		l.Enabled = *in.Enabled
	}
	if in.Archived != nil {
		l.Archived = *in.Archived
	}

	if err := validateRedirectTargets(&l); err != nil {
		return nil, nil, err
	}

	if err := tdb.WithContext(ctx).Save(&l).Error; err != nil {
		return nil, nil, fmt.Errorf("update link: %w", err)
	}

	if p.publishEvent != nil {
		p.publishEvent(orgID, "link.update", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host})
	}
	if p.deleteCache != nil {
		if err := p.deleteCache(ctx, "link:redirect:"+oldHost+":"+oldSlug); err != nil {
			log.Printf("links: failed to delete old cache for %s:%s: %v", oldHost, oldSlug, err)
		}
		if oldHost != l.Host || oldSlug != l.Slug {
			if err := p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug); err != nil {
				log.Printf("links: failed to delete cache for %s:%s: %v", l.Host, l.Slug, err)
			}
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
