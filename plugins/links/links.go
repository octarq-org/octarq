package links

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// linkDTO is the create/update payload.
type linkDTO struct {
	Host       string     `json:"host,omitempty"`
	Slug       string     `json:"slug,omitempty"`
	Target     string     `json:"target"`
	Password   string     `json:"password,omitempty"`
	Note       string     `json:"note,omitempty"`
	Title      string     `json:"title,omitempty"`
	Tags       string     `json:"tags,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	ExpiredURL string     `json:"expiredUrl,omitempty"`
	ClickLimit int64      `json:"clickLimit,omitempty"`
	Archived   *bool      `json:"archived,omitempty"`
	Enabled    *bool      `json:"enabled,omitempty"`
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

type LinkMetadataInput struct {
	Ctx huma.Context `hidden:"true"`
	URL string       `query:"url"`
}

func (i *LinkMetadataInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LinkMetadataOutput struct {
	Body map[string]any
}

// linkMetadata fetches the target page's <title>, description, and favicon so
// the dashboard can prefill a link's title (dub-style). Best-effort.
func (p *Plugin) linkMetadata(ctx context.Context, input *LinkMetadataInput) (*LinkMetadataOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	raw := strings.TrimSpace(input.URL)
	if raw == "" {
		return nil, huma.Error400BadRequest("url required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, huma.Error400BadRequest("invalid url")
	}
	title, desc := fetchPageMeta(r.Context(), raw)
	favicon := u.Scheme + "://" + u.Host + "/favicon.ico"
	return &LinkMetadataOutput{
		Body: map[string]any{
			"title": title, "description": desc, "favicon": favicon,
		},
	}, nil
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

// normalizeTarget trims a user-supplied redirect target, defaults a bare host
// to https, and rejects anything that isn't a well-formed http(s) URL. This
// keeps javascript:, data:, and other dangerous schemes out of a stored link
// (which is later emitted verbatim in a 302 Location header). Returns the
// normalized URL and true on success, or ("", false) when it must be refused.
func normalizeTarget(raw string) (string, bool) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return raw, true
}

// validateRedirectTargets checks every user-supplied redirect target on a link
// that is later emitted verbatim in a 302 Location header — the ExpiredURL and
// each RoutingRule.Target — against normalizeTarget's http(s) scheme allowlist.
// Unlike the primary Target these are not normalized in place (they may be
// stored raw), so this rejects javascript:, data:, etc. at write time. An empty
// ExpiredURL is allowed (it just means "404 when expired"). It normalizes the
// accepted values in place so a bare host defaults to https like Target does.
func validateRedirectTargets(l *Link) error {
	if l.ExpiredURL != "" {
		n, ok := normalizeTarget(strings.TrimSpace(l.ExpiredURL))
		if !ok {
			return huma.Error400BadRequest("expiredUrl must be an http(s) URL")
		}
		l.ExpiredURL = n
	}
	for i := range l.RoutingRules {
		n, ok := normalizeTarget(strings.TrimSpace(l.RoutingRules[i].Target))
		if !ok {
			return huma.Error400BadRequest("routing rule target must be an http(s) URL")
		}
		l.RoutingRules[i].Target = n
	}
	return nil
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
		Enabled: enabled,
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
	// Same quota gate as createLink — quick-create is a second door into the
	// same table, and leaving it ungated would be a trivial bypass.
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

type ExportLinksCSVInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ExportLinksCSVInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) exportLinksCSV(ctx context.Context, input *ExportLinksCSVInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var links []Link
	p.orgDB(r).Order("created_at DESC").Find(&links)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"links.csv\"")

	cw := csv.NewWriter(w)
	cw.Write([]string{"ID", "Host", "Slug", "Target", "Title", "Clicks", "CreatedAt"})
	for _, l := range links {
		cw.Write([]string{
			fmt.Sprintf("%d", l.ID),
			l.Host,
			l.Slug,
			l.Target,
			l.Title,
			fmt.Sprintf("%d", l.Clicks),
			l.CreatedAt.Format(time.RFC3339),
		})
	}
	cw.Flush()
	return nil, nil
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
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
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
	l.Password = input.Body.Password
	l.ExpiresAt = input.Body.ExpiresAt
	l.ExpiredURL = input.Body.ExpiredURL
	l.ClickLimit = input.Body.ClickLimit
	if input.Body.Archived != nil {
		l.Archived = *input.Body.Archived
	}
	if input.Body.Enabled != nil {
		l.Enabled = *input.Body.Enabled
	}
	if err := validateRedirectTargets(&l); err != nil {
		return nil, err
	}
	if err := p.db.Save(&l).Error; err != nil {
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
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	p.db.Where("link_id IN (SELECT id FROM links WHERE id = ? AND owner_id = ?)", input.ID, p.orgID(r)).Delete(&LinkEvent{})
	p.db.Delete(&l)
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

// checkQuota asks the (hosted-only) quota checker whether the org may consume
// n more of a metered resource, and maps a refusal to the HTTP error a client
// should see. An exhausted allowance is a 429; a capability the plan simply
// does not include is a 402 upgrade prompt — the two must stay distinct
// because the dashboard renders them differently. With no checker registered
// (self-hosted) it always passes: unlimited links are the self-hosted selling
// point.
func (p *Plugin) checkQuota(ctx context.Context, orgID uint, metric string, n int64) error {
	if err := plugin.CheckQuota(p.ctx, ctx, orgID, metric, n); err != nil {
		if errors.Is(err, plugin.ErrQuotaUnavailable) {
			return huma.Error402PaymentRequired(metric + " is not included in this plan")
		}
		return huma.Error429TooManyRequests(metric + " quota exceeded for this workspace")
	}
	return nil
}

// escapeLike escapes LIKE metacharacters so a user tag of `_` or `100%`
// cannot match extra rows. '!' is the ESCAPE character (portable, unlike '\').
func escapeLike(s string) string {
	r := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return r.Replace(s)
}

// filterByTag is the SQL counterpart of tagsContain: case-insensitive, comma
// token match, surrounding spaces ignored. Spaces are stripped rather than
// enumerated as LIKE arms so Postgres (case-sensitive LIKE) and ", b ,"
// whitespace stay in lockstep with the Go helper.
func filterByTag(q *gorm.DB, tag string) *gorm.DB {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return q
	}
	folded := strings.ToLower(strings.ReplaceAll(tag, " ", ""))
	esc := escapeLike(folded)
	norm := "LOWER(REPLACE(tags, ' ', ''))"
	return q.Where(
		norm+" = ? OR "+norm+" LIKE ? ESCAPE '!' OR "+norm+" LIKE ? ESCAPE '!' OR "+norm+" LIKE ? ESCAPE '!'",
		folded,
		esc+",%",
		"%,"+esc,
		"%,"+esc+",%",
	)
}
