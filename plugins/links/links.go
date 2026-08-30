package links

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

// linkDTO is the create/update payload.
type linkDTO struct {
	Host         string       `json:"host,omitempty"`
	Slug         string       `json:"slug,omitempty"`
	Target       string       `json:"target"`
	Password     string       `json:"password,omitempty"`
	Note         string       `json:"note,omitempty"`
	Title        string       `json:"title,omitempty"`
	Tags         string       `json:"tags,omitempty"`
	ExpiresAt    *time.Time   `json:"expiresAt,omitempty"`
	ExpiredURL   string       `json:"expiredUrl,omitempty"`
	ClickLimit   int64        `json:"clickLimit,omitempty"`
	Archived     *bool        `json:"archived,omitempty"`
	Enabled      *bool        `json:"enabled,omitempty"`
	RoutingRules RoutingRules `json:"routingRules,omitempty"`
}

type linkView struct {
	Link
	HasPassword bool `json:"hasPassword"`
}

func view(l Link) linkView {
	return linkView{Link: l, HasPassword: l.Password != ""}
}

type ListLinksInput struct {
	Ctx      huma.Context `hidden:"true"`
	Archived string       `query:"archived"`
	Q        string       `query:"q"`
	Tag      string       `query:"tag"`
	Host     string       `query:"host"`
	Limit    int          `query:"limit"`
	Offset   int          `query:"offset"`
}

func (i *ListLinksInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListLinksOutput struct {
	Body []linkView
}

func (p *Plugin) listLinks(ctx context.Context, input *ListLinksInput) (*ListLinksOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var links []Link
	q := p.orgDB(r).Order("created_at DESC")
	// Archived links are hidden unless explicitly requested (?archived=1).
	if input.Archived == "1" {
		q = q.Where("archived = ?", true)
	} else {
		q = q.Where("archived = ?", false)
	}
	if input.Q != "" {
		like := "%" + input.Q + "%"
		q = q.Where("slug LIKE ? OR target LIKE ? OR note LIKE ? OR title LIKE ? OR tags LIKE ?", like, like, like, like, like)
	}
	if tag := strings.TrimSpace(input.Tag); tag != "" {
		q = filterByTag(q, tag)
	}
	if input.Host != "" {
		q = q.Where("host = ?", input.Host)
	}
	limit := plugin.PageLimit(input.Limit, 50, 500)
	offset := plugin.PageOffset(input.Offset)
	q = q.Limit(limit).Offset(offset)
	if err := q.Find(&links).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to list links")
	}
	out := make([]linkView, len(links))
	for i, l := range links {
		out[i] = view(l)
	}
	return &ListLinksOutput{Body: out}, nil
}

type GetLinkInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *GetLinkInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetLinkOutput struct {
	Body linkView
}

func (p *Plugin) getLink(ctx context.Context, input *GetLinkInput) (*GetLinkOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	return &GetLinkOutput{Body: view(l)}, nil
}

type CreateLinkInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body linkDTO
}

func (i *CreateLinkInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateLinkOutput struct {
	Body linkView
}

func (p *Plugin) createLink(ctx context.Context, input *CreateLinkInput) (*CreateLinkOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	target := strings.TrimSpace(input.Body.Target)
	if target == "" {
		return nil, huma.Error400BadRequest("target is required")
	}
	normalized, ok := normalizeTarget(target)
	if !ok {
		return nil, huma.Error400BadRequest("target must be an http(s) URL")
	}
	slug := strings.TrimSpace(input.Body.Slug)
	if slug == "" {
		slug = models.RandomSlug(6)
	}
	if p.isReservedSlug(slug) {
		return nil, huma.NewError(http.StatusConflict, "slug is reserved")
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	l := Link{
		OrgID: p.orgID(r),
		Host:  strings.TrimSpace(input.Body.Host), Slug: slug, Target: normalized,
		Password: input.Body.Password, Note: input.Body.Note, Title: input.Body.Title, Tags: input.Body.Tags,
		ExpiresAt: input.Body.ExpiresAt, ExpiredURL: input.Body.ExpiredURL, ClickLimit: input.Body.ClickLimit,
		RoutingRules: input.Body.RoutingRules,
		Enabled:      enabled,
	}
	if l.Host != "" && !p.ownsHost(l.OrgID, l.Host) {
		return nil, huma.Error403Forbidden("host is not a link host of this workspace")
	}
	if l.Host == "" && p.linkHostRequired(l.OrgID) {
		return nil, huma.Error400BadRequest("host is required: this instance is multi-tenant, short links must pick a link host")
	}
	if err := validateRedirectTargets(&l); err != nil {
		return nil, err
	}
	// Metered consumption: ask the (hosted-only) quota checker whether this
	// org may create another link. A self-hosted install has no checker, so
	// this always passes there — unlimited links are the self-hosted selling
	// point, and the check must run before the row exists (no partial record
	// when it refuses).
	if err := p.checkQuota(ctx, l.OrgID, "links", 1); err != nil {
		return nil, err
	}
	if err := p.db.Create(&l).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "slug already exists on this host")
	}
	if p.audit != nil {
		p.audit(r, "link.create", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target})
	}
	if p.publishEvent != nil {
		p.publishEvent(l.OrgID, "link.create", map[string]any{"id": l.ID, "slug": l.Slug, "host": l.Host, "target": l.Target})
	}

	if l.Title == "" && p.enqueue != nil {
		payload, _ := json.Marshal(map[string]any{
			"id":     l.ID,
			"target": l.Target,
		})
		_ = p.enqueue(r.Context(), "link.crawl", payload)
	}

	if p.deleteCache != nil {
		_ = p.deleteCache(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
	}
	return &CreateLinkOutput{Body: view(l)}, nil
}
