package links

import (
	"context"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// CreateLink implements plugin.LinkCreator: it creates an enabled short link for
// targetURL in orgID's workspace on the default host with a random slug and
// returns the slug. It is the programmatic entry behind the "links.create"
// service (Provide/LookupAs), mirroring POST /api/links without a request.
func (p *Plugin) CreateLink(ctx context.Context, orgID uint, targetURL string) (string, error) {
	if orgID == 0 {
		return "", fmt.Errorf("orgID is required")
	}
	normalized, ok := normalizeTarget(strings.TrimSpace(targetURL))
	if !ok {
		return "", fmt.Errorf("target must be an http(s) URL")
	}
	l := Link{OrgID: orgID, Slug: models.RandomSlug(6), Target: normalized, Enabled: true}
	if err := validateRedirectTargets(&l); err != nil {
		return "", err
	}
	if err := p.db.WithContext(ctx).Create(&l).Error; err != nil {
		return "", fmt.Errorf("create link: %w", err)
	}
	if p.publishEvent != nil {
		p.publishEvent(l.OrgID, "link.create", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
	}
	if p.deleteCache != nil {
		_ = p.deleteCache(ctx, "link:redirect:"+l.Host+":"+l.Slug)
	}
	return l.Slug, nil
}

// Compile-time assertion that the links plugin satisfies the public seam.
var _ plugin.LinkCreator = (*Plugin)(nil)
