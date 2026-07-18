package links

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/safehttp"
	qrcode "github.com/skip2/go-qrcode"
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
	if input.Tag != "" {
		q = q.Where("tags LIKE ?", "%"+input.Tag+"%")
	}
	if input.Host != "" {
		q = q.Where("host = ?", input.Host)
	}
	limit := 50
	if input.Limit > 0 && input.Limit <= 500 {
		limit = input.Limit
	}
	offset := 0
	if input.Offset > 0 {
		offset = input.Offset
	}
	q = q.Limit(limit).Offset(offset)
	q.Find(&links)
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
	title, desc := safehttp.FetchPageMeta(r.Context(), raw)
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
	if err := validateRedirectTargets(&l); err != nil {
		return nil, err
	}
	if err := p.db.Create(&l).Error; err != nil {
		return nil, huma.NewError(http.StatusConflict, "slug already exists on this host")
	}
	if p.audit != nil {
		p.audit(r, "link.create", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target})
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
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	p.db.Delete(&l)
	if p.deleteCache != nil {
		_ = p.deleteCache(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
	}

	p.db.Where("link_id = ?", input.ID).Delete(&LinkEvent{})
	if p.audit != nil {
		p.audit(r, "link.delete", "link", input.ID, nil)
	}
	return &DeleteLinkOutput{Body: map[string]bool{"ok": true}}, nil
}

type LinkStatsInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Days int          `query:"days"`
}

func (i *LinkStatsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type LinkStatsOutput struct {
	Body map[string]any
}

// linkStats returns basic analytics: totals, a daily time series, and the
// top referers / countries / devices / browsers over the requested window.
func (p *Plugin) linkStats(ctx context.Context, input *LinkStatsInput) (*LinkStatsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	// Ensure the link belongs to the caller's org before exposing its analytics.
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	days := 30
	if input.Days > 0 && input.Days <= 365 {
		days = input.Days
	}
	since := time.Now().AddDate(0, 0, -days)

	top := func(col string) []models.StatKV {
		rows := make([]models.StatKV, 0)
		q := p.db.Model(&LinkEvent{}).
			Where("link_id = ? AND created_at >= ? AND "+col+" <> ''", input.ID, since)
		if col == "device" {
			q = q.Select(col + " as key, count(distinct(ip || ' ' || ua)) as count")
		} else {
			q = q.Select(col + " as key, count(*) as count")
		}
		q.Group(col).Order("count DESC").Limit(10).Scan(&rows)
		return rows
	}

	var total int64
	p.db.Model(&LinkEvent{}).Where("link_id = ?", input.ID).Count(&total)

	series := make([]models.StatKV, 0)
	p.db.Model(&LinkEvent{}).
		Select("strftime('%Y-%m-%d', created_at) as key, count(*) as count").
		Where("link_id = ? AND created_at >= ?", input.ID, since).
		Group("key").Order("key ASC").Scan(&series)
	// Postgres uses to_char; fall back when sqlite strftime yields nothing.
	if len(series) == 0 && p.db.Dialector.Name() == "postgres" {
		p.db.Model(&LinkEvent{}).
			Select("to_char(created_at, 'YYYY-MM-DD') as key, count(*) as count").
			Where("link_id = ? AND created_at >= ?", input.ID, since).
			Group("key").Order("key ASC").Scan(&series)
	}

	return &LinkStatsOutput{
		Body: map[string]any{
			"total":     total,
			"windowed":  models.SumStatKV(series),
			"days":      days,
			"series":    series,
			"referers":  top("referer"),
			"countries": top("country"),
			"regions":   top("region"),
			"devices":   top("device"),
			"browsers":  top("browser"),
		},
	}, nil
}

// models.StatKV is a key/count pair used across link analytics responses.

type LinkQRInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *LinkQRInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) linkQR(ctx context.Context, input *LinkQRInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var l Link
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&l).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	target := shortURL(r, l)
	png, err := qrcode.Encode(target, qrcode.Medium, 320)
	if err != nil {
		return nil, huma.Error500InternalServerError("qr failed")
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
	return nil, nil
}

// shortURL builds the public URL for a link. When the link has its own host it
// is used; otherwise the URL is derived from the incoming request so no extra
// configuration is needed.
func shortURL(r *http.Request, l Link) string {
	host := l.Host
	scheme := "https"
	if host == "" {
		host = r.Host
		if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			scheme = "http"
		}
	}
	return scheme + "://" + host + "/" + l.Slug
}
