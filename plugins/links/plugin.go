package links

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type Plugin struct {
	db   *gorm.DB
	auth struct {
		UserID func(r *http.Request) uint
		OrgID  func(r *http.Request) uint
	}
	audit               func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	getGlobalSetting    func(key string) string
	getWorkspaceSetting func(orgID uint, key string) string
	enqueue             func(ctx context.Context, taskType string, payload []byte) error
	deleteCache         func(ctx context.Context, key string) error
}

var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "links" }
func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{Title: "Short Links", Description: "Short link creation, custom domain routing, and click analytics.", EnabledByDefault: true, Requires: []string{"dns"}}
}
func (p *Plugin) Models() []any {
	return []any{&Link{}, &LinkEvent{}}
}

func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.auth.OrgID(r))
}

func (p *Plugin) orgID(r *http.Request) uint {
	return p.auth.OrgID(r)
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx.DB != nil {
		p.db = ctx.DB
	}
	if ctx.UserID != nil {
		p.auth.UserID = ctx.UserID
	}
	if ctx.OrgID != nil {
		p.auth.OrgID = ctx.OrgID
	}
	if ctx.Audit != nil {
		p.audit = ctx.Audit
	}
	if ctx.GetGlobalSetting != nil {
		p.getGlobalSetting = ctx.GetGlobalSetting
	}
	if ctx.GetWorkspaceSetting != nil {
		p.getWorkspaceSetting = ctx.GetWorkspaceSetting
	}
	if ctx.Enqueue != nil {
		p.enqueue = ctx.Enqueue
	}
	if ctx.DeleteCache != nil {
		p.deleteCache = ctx.DeleteCache
	}
	api := ctx.Huma
	if api != nil {
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links", Summary: "List Links", Tags: []string{"Links"}}, p.listLinks)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/links", Summary: "Create Link", Tags: []string{"Links"}, DefaultStatus: 201}, p.createLink)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/metadata", Summary: "Link Metadata", Tags: []string{"Links"}}, p.linkMetadata)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}", Summary: "Get Link", Tags: []string{"Links"}}, p.getLink)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/links/{id}", Summary: "Update Link", Tags: []string{"Links"}}, p.updateLink)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/links/{id}", Summary: "Delete Link", Tags: []string{"Links"}}, p.deleteLink)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/stats", Summary: "Link Stats", Tags: []string{"Links"}}, p.linkStats)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/{id}/qr", Summary: "Link QR", Tags: []string{"Links"}}, p.linkQR)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/links/export.csv", Summary: "Export Links", Tags: []string{"Links"}}, p.exportLinksCSV)
	}
	if ctx.Provide != nil {
		ctx.Provide("links.overview", p.overview)
		ctx.Provide("links.purge", p.purge)
		ctx.Provide("links.export", p.exportData)
		ctx.Provide("links.resolve", p.resolveSlug)
		ctx.Provide("links.cleanup", p.cleanupEvents)
		ctx.Provide("links.mcp_export", p.mcpExportLinks)
		ctx.Provide("links.trust_proxy", SetTrustProxy)
	}
	if ctx.RegisterTask != nil {
		ctx.RegisterTask("link.crawl", p.handleLinkCrawl)
	}

	engine := NewEngine(ctx.DB, ctx)
	if ctx.HandleRoot != nil {
		ctx.HandleRoot(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := strings.TrimPrefix(r.URL.Path, "/")
			if slug == "" {
				http.NotFound(w, r)
				return
			}
			link, ok := engine.Lookup(r.Host, slug)
			if !ok {
				http.NotFound(w, r)
				return
			}
			engine.Handle(w, r, link)
		}))
	}
}

func (p *Plugin) purge(orgID uint) error {
	linkIDs := p.db.Model(&Link{}).Select("id").Where("owner_id = ?", orgID)
	p.db.Where("link_id IN (?)", linkIDs).Delete(&LinkEvent{})
	p.db.Where("owner_id = ?", orgID).Delete(&Link{})
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	var l []Link
	p.db.Where("owner_id = ?", orgID).Find(&l)
	return map[string]any{
		"links": l,
	}
}

func (p *Plugin) resolveSlug(slug string) (target string, orgID uint, ok bool) {
	var l Link
	if p.db.Where("slug = ?", slug).First(&l).Error == nil {
		return l.Target, l.OrgID, true
	}
	return "", 0, false
}

func (p *Plugin) cleanupEvents(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	totalPurged := int64(0)
	for {
		var ids []uint
		if err := p.db.Model(&LinkEvent{}).Where("created_at < ?", cutoff).Limit(2000).Pluck("id", &ids).Error; err != nil {
			return
		}
		if len(ids) == 0 {
			break
		}
		res := p.db.Delete(&LinkEvent{}, ids)
		if res.Error != nil {
			return
		}
		totalPurged += res.RowsAffected
		time.Sleep(50 * time.Millisecond)
	}
}

func (p *Plugin) overview(orgID uint, includeBot bool) map[string]any {
	botFilter := func(q *gorm.DB) *gorm.DB {
		if includeBot {
			return q
		}
		return q.Where("is_bot = ?", false)
	}
	count := func(model any, conds ...any) int64 {
		var n int64
		q := p.db.Model(model).Where("owner_id = ?", orgID)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	var totalClicks int64
	botFilter(p.db.Model(&LinkEvent{}).
		Joins("JOIN links ON links.id = link_events.link_id AND links.owner_id = ?", orgID)).
		Select("COUNT(*)").Scan(&totalClicks)

	orgLinks := p.db.Model(&Link{}).Select("id").Where("owner_id = ?", orgID)
	now := time.Now()
	since30 := now.AddDate(0, 0, -30)
	since7 := now.AddDate(0, 0, -7)

	type statKV struct {
		Key   string `json:"key" gorm:"column:key"`
		Count int64  `json:"count" gorm:"column:count"`
	}
	var series []statKV
	botFilter(p.db.Model(&LinkEvent{}).
		Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
		Select("strftime('%Y-%m-%d', created_at) as key, count(*) as count").
		Group("key").Order("key ASC").Scan(&series)
	// Postgres uses to_char; fall back when sqlite strftime yields nothing.
	if len(series) == 0 && p.db.Dialector.Name() == "postgres" {
		botFilter(p.db.Model(&LinkEvent{}).
			Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
			Select("to_char(created_at, 'YYYY-MM-DD') as key, count(*) as count").
			Group("key").Order("key ASC").Scan(&series)
	}

	top := func(col string) []statKV {
		var rows []statKV
		q := botFilter(p.db.Model(&LinkEvent{}).
			Where("link_id IN (?) AND created_at >= ? AND "+col+" <> ''", orgLinks, since30))
		if col == "device" {
			q = q.Select(col + " as key, count(distinct COALESCE(NULLIF(fingerprint, ''), ip || ' ' || ua)) as count")
		} else {
			q = q.Select(col + " as key, count(*) as count")
		}
		q.Group(col).Order("count DESC").Limit(8).Scan(&rows)
		return rows
	}

	type topLink struct {
		ID     uint   `json:"id"`
		Slug   string `json:"slug"`
		Host   string `json:"host"`
		Clicks int64  `json:"clicks"`
	}
	var topLinks []topLink
	p.db.Model(&Link{}).
		Select("id, slug, host, clicks").
		Where("owner_id = ? AND archived = ?", orgID, false).
		Order("clicks DESC").Limit(5).Scan(&topLinks)

	clickCount := func(conds ...any) int64 {
		var n int64
		q := botFilter(p.db.Model(&LinkEvent{}).Where("link_id IN (?)", orgLinks))
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	botCount := func(conds ...any) int64 {
		var n int64
		q := p.db.Model(&LinkEvent{}).Where("link_id IN (?) AND is_bot = ?", orgLinks, true)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	return map[string]any{
		"links":        count(&Link{}),
		"activeLinks":  count(&Link{}, "archived = ? AND enabled = ?", false, true),
		"totalClicks":  totalClicks,
		"clicks7d":     clickCount("created_at >= ?", since7),
		"clicks30d":    clickCount("created_at >= ?", since30),
		"botClicks7d":  botCount("created_at >= ?", since7),
		"botClicks30d": botCount("created_at >= ?", since30),
		"series":       series,
		"topLinks":     topLinks,
		"devices":      top("device"),
		"countries":    top("country"),
		"cities":       top("city"),
	}
}

var builtinReservedSlugs = map[string]bool{
	"admin":  true,
	"api":    true,
	"assets": true,
	"portal": true,
}

func (p *Plugin) isReservedSlug(slug string) bool {
	slug = strings.ToLower(slug)
	if builtinReservedSlugs[slug] {
		return true
	}
	if p.getGlobalSetting != nil {
		for _, res := range splitList(p.getGlobalSetting("reserved_slugs")) {
			if res == slug {
				return true
			}
		}
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
