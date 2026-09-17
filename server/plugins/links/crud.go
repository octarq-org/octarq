package links

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
)

type QuickCreateLinkBody struct {
	URL  string `json:"url"`
	Host string `json:"host,omitempty"`
}

type QuickCreateLinkInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body QuickCreateLinkBody
}

func (i *QuickCreateLinkInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type QuickCreateLinkOutput struct {
	Body linkView
}

func (p *Plugin) quickCreateLink(ctx context.Context, input *QuickCreateLinkInput) (*QuickCreateLinkOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	target := strings.TrimSpace(input.Body.URL)
	if target == "" {
		return nil, huma.Error400BadRequest("url is required")
	}
	normalized, ok := normalizeTarget(target)
	if !ok {
		return nil, huma.Error400BadRequest("url must be an http(s) URL")
	}
	host := strings.TrimSpace(input.Body.Host)
	if host != "" && !p.ownsHost(p.orgID(r), host) {
		return nil, huma.Error403Forbidden("host is not a link host of this workspace")
	}
	if host == "" && p.linkHostRequired(p.orgID(r)) {
		return nil, huma.Error400BadRequest("host is required: this instance is multi-tenant, short links must pick a link host")
	}
	slug := models.RandomSlug(6)
	if p.isReservedSlug(slug) {
		return nil, huma.NewError(http.StatusConflict, "slug is reserved")
	}
	l := Link{
		OrgID:   p.orgID(r),
		Host:    host,
		Slug:    slug,
		Target:  normalized,
		Enabled: true,
	}
	if err := p.checkQuota(ctx, l.OrgID, "links", 1); err != nil {
		return nil, err
	}
	tdb := p.tenantDB(l.OrgID)
	if tdb == nil {
		return nil, huma.Error500InternalServerError("database not available")
	}
	if err := tdb.Create(&l).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "slug already exists on this host")
	}
	if p.audit != nil {
		p.audit(r, "link.create", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target})
	}
	if p.publishEvent != nil {
		p.publishEvent(l.OrgID, "link.create", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
	}

	if p.enqueue != nil {
		payload, _ := json.Marshal(map[string]any{
			"id":     l.ID,
			"target": l.Target,
		})
		_ = p.enqueue(r.Context(), "link.crawl", payload)
	}

	if p.deleteCache != nil {
		_ = p.deleteCache(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
	}
	return &QuickCreateLinkOutput{Body: view(l)}, nil
}

type UpdateLinkInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body linkDTO
}

func (i *UpdateLinkInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateLinkOutput struct {
	Body linkView
}

func (p *Plugin) updateLink(ctx context.Context, input *UpdateLinkInput) (*UpdateLinkOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var l Link
	if tdb.Where("id = ?", input.ID).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	// Capture BEFORE mutation so cache invalidation targets the original key.
	oldHost := l.Host
	oldSlug := l.Slug

	if input.Body.Slug != "" {
		slug := strings.TrimSpace(input.Body.Slug)
		if slug != l.Slug && p.isReservedSlug(slug) {
			return nil, huma.NewError(http.StatusConflict, "slug is reserved")
		}
		l.Slug = slug
	}
	if input.Body.Target != "" {
		normalized, ok := normalizeTarget(strings.TrimSpace(input.Body.Target))
		if !ok {
			return nil, huma.Error400BadRequest("target must be an http(s) URL")
		}
		l.Target = normalized
	}

	l.Host = strings.TrimSpace(input.Body.Host)
	if l.Host != "" && !p.ownsHost(p.orgID(r), l.Host) {
		return nil, huma.Error403Forbidden("host is not a link host of this workspace")
	}
	if l.Host == "" && p.linkHostRequired(p.orgID(r)) {
		return nil, huma.Error400BadRequest("host is required: this instance is multi-tenant, short links must pick a link host")
	}
	l.Note = input.Body.Note
	l.Title = input.Body.Title
	l.Tags = input.Body.Tags
	if input.Body.Password != nil {
		l.Password = *input.Body.Password
	}
	l.ExpiresAt = input.Body.ExpiresAt
	l.ExpiredURL = input.Body.ExpiredURL
	l.ClickLimit = input.Body.ClickLimit
	if input.Body.RoutingRules != nil {
		l.RoutingRules = input.Body.RoutingRules
	}
	if input.Body.Archived != nil {
		l.Archived = *input.Body.Archived
	}
	if input.Body.Enabled != nil {
		l.Enabled = *input.Body.Enabled
	}
	if err := validateRedirectTargets(&l); err != nil {
		return nil, err
	}
	if err := tdb.Save(&l).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "slug already exists on this host")
	}

	if p.deleteCache != nil {
		_ = p.deleteCache(r.Context(), "link:redirect:"+oldHost+":"+oldSlug)
		if oldHost != l.Host || oldSlug != l.Slug {
			_ = p.deleteCache(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
		}
	}

	if p.audit != nil {
		p.audit(r, "link.update", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target})
	}
	return &UpdateLinkOutput{Body: view(l)}, nil
}

type DeleteLinkInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteLinkInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteLinkOutput struct {
	Body map[string]bool
}

func (p *Plugin) deleteLink(ctx context.Context, input *DeleteLinkInput) (*DeleteLinkOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to delete link")
	}
	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var l Link
	if tdb.Where("id = ?", input.ID).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	linkIDs := tdb.Model(&Link{}).Select("id").Where("id = ?", input.ID)
	p.db.Where("link_id IN (?)", linkIDs).Delete(&LinkEvent{})
	tdb.Delete(&l)
	if p.deleteCache != nil {
		_ = p.deleteCache(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
	}

	if p.audit != nil {
		p.audit(r, "link.delete", "link", input.ID, nil)
	}
	if p.publishEvent != nil {
		p.publishEvent(l.OrgID, "link.delete", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
	}
	return &DeleteLinkOutput{Body: map[string]bool{"ok": true}}, nil
}
