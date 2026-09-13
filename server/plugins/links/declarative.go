package links

import (
	"context"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

type DeclarativeLinkInput struct {
	Slug        string   `json:"slug,omitempty" doc:"Custom short link slug (alphanumeric and dashes)" maxLength:"50" example:"launch-day"`
	Destination string   `json:"destination" doc:"Target destination URL" format:"uri" example:"https://example.com"`
	Tags        []string `json:"tags,omitempty" doc:"Optional list of grouping tags" example:"[\"marketing\"]"`
}

type DeclarativeLinkOutput struct {
	ID          uint      `json:"id" doc:"Unique link ID"`
	Slug        string    `json:"slug" doc:"Assigned short link slug"`
	Destination string    `json:"destination" doc:"Original target URL"`
	CreatedAt   time.Time `json:"created_at" doc:"Creation timestamp"`
}

func (p *Plugin) createDeclarativeLink(ctx context.Context, in DeclarativeLinkInput) (*DeclarativeLinkOutput, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, plugin.NewAgentError(401, "UNAUTHORIZED", "unauthorized: missing workspace", "Ensure an authenticated session or API token is provided.", false)
	}
	if p.linkHostRequired(orgID) {
		return nil, plugin.NewAgentError(400, "HOST_REQUIRED", "host is required in multi-tenant mode", "Please configure a custom link host first.", false)
	}
	if err := p.checkQuota(ctx, orgID, "links", 1); err != nil {
		return nil, err
	}
	dest := strings.TrimSpace(in.Destination)
	if dest == "" {
		return nil, plugin.NewAgentError(400, "MISSING_DESTINATION", "destination is required", "Please provide a valid destination URL.", false)
	}
	normalized, ok := normalizeTarget(dest)
	if !ok {
		return nil, plugin.NewAgentError(400, "INVALID_DESTINATION", "destination must be an http(s) URL", "Please provide a valid URL starting with http:// or https://.", false)
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = models.RandomSlug(6)
	}
	if p.isReservedSlug(slug) {
		return nil, plugin.NewAgentError(409, "SLUG_RESERVED", "slug is reserved", "The specified slug is a system reserved path. Please pick a different slug.", false)
	}
	tagStr := strings.Join(in.Tags, ",")
	l := Link{
		OrgID:     orgID,
		Slug:      slug,
		Target:    normalized,
		Tags:      tagStr,
		Enabled:   true,
		CreatedAt: time.Now(),
	}
	if err := p.db.WithContext(ctx).Create(&l).Error; err != nil {
		return nil, plugin.NewAgentError(409, "SLUG_ALREADY_EXISTS", "slug already exists on this host", "The slug is already taken. Please choose another slug or leave it blank.", false)
	}
	if p.audit != nil {
		p.audit(nil, "link.create", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target, "source": "declarative"})
	}
	if p.publishEvent != nil {
		p.publishEvent(l.OrgID, "link.create", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
	}
	if p.deleteCache != nil {
		_ = p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug)
	}
	return &DeclarativeLinkOutput{
		ID:          l.ID,
		Slug:        l.Slug,
		Destination: l.Target,
		CreatedAt:   l.CreatedAt,
	}, nil
}
