package links

import (
	"context"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func (p *Plugin) Start(ctx context.Context) {
	<-ctx.Done()
	if p.engine != nil {
		p.engine.Close()
	}
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.ctx = ctx
	if ctx.DB != nil {
		p.db = ctx.DB
	}
	p.host = plugin.EnsureHost(ctx)
	if p.host != nil {
		if p.host.Session() != nil {
			p.auth.UserID = p.host.Session().UserID
			p.auth.OrgID = p.host.Session().OrgID
			p.requireRole = p.host.Session().RequireRole
			p.isInstanceAdmin = p.host.Session().IsInstanceAdmin
		}
		if p.host.Settings() != nil {
			p.getGlobalSetting = p.host.Settings().GetGlobalSetting
			p.getWorkspaceSetting = p.host.Settings().GetWorkspaceSetting
		}
		if p.host.Events() != nil {
			p.publishEvent = p.host.Events().PublishEvent
			p.host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "link.create", Group: "Links", Title: "Link Created", Description: "A short link was created"})
			p.host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "link.click", Group: "Links", Title: "Link Clicked", Description: "A tracked short link was visited"})
			p.host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{Key: "link.delete", Group: "Links", Title: "Link Deleted", Description: "A short link was deleted"})
		}
	}
	if ctx.Audit != nil {
		p.audit = ctx.Audit
	}
	if ctx.Enqueue != nil {
		p.enqueue = ctx.Enqueue
	}
	if ctx.DeleteCache != nil {
		p.deleteCache = ctx.DeleteCache
	}

	RegisterViews(ctx)

	p.registerRoutes(ctx)

	if ctx.Provide != nil {
		ctx.Provide(plugin.OverviewServiceName("links"), plugin.OverviewFunc(p.overview))
		ctx.Provide(plugin.PurgeServiceName("links"), plugin.PurgeFunc(p.purge))
		ctx.Provide(plugin.ExportServiceName("links"), plugin.ExportFunc(p.exportData))
		ctx.Provide(plugin.ServiceLinkResolve, plugin.LinkResolver(p.resolveSlug))
		ctx.Provide("links.create", plugin.LinkCreator(p))
		ctx.Provide(plugin.CleanupServiceName("links"), plugin.CleanupFunc(p.cleanupEvents))
		ctx.Provide(plugin.MCPExportServiceName("links"), plugin.MCPExporter(p.mcpExportLinks))
		// Trust-proxy seam: the app assembly layer resolves this service
		// after mounting all plugins and calls it with cfg.TrustProxy.
		ctx.Provide("links.trust_proxy", SetTrustProxy)
	}
	if ctx.RegisterTask != nil {
		ctx.RegisterTask("link.crawl", p.handleLinkCrawl)
	}

	p.engine = NewEngine(ctx.DB, ctx)
}

func (p *Plugin) purge(orgID uint) error {
	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return nil
	}
	linkIDs := tdb.Model(&Link{}).Select("id")
	p.db.Where("link_id IN (?)", linkIDs).Delete(&LinkEvent{})
	tdb.Delete(&Link{})
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	var l []Link
	if tdb := p.tenantDB(orgID); tdb != nil {
		tdb.Find(&l)
	}
	return map[string]any{
		"links": l,
	}
}

// resolveSlug attributes a reported slug to the link actually served at
// (host, slug). It backs the public abuse-report form, which files a report
// against the owning workspace.
//
// Host is not decoration. Two workspaces may hold the same slug on different
// hostnames, and matching by slug alone files the report against whichever row
// the database happened to return first — so a report about victim.com/x could
// land on another tenant's queue while the victim never learns their link was
// reported. Scoping is delegated to the engine so attribution and the redirect
// itself can never disagree about whose link a hostname serves.
//
// Unlike the redirect, a disabled or archived link still attributes: a report
// about a link that was just turned off is exactly the report a moderator
// wants to see.
func (p *Plugin) resolveSlug(host, slug string) (target string, orgID uint, ok bool) {
	if p.engine == nil {
		return "", 0, false
	}
	query, servable := p.engine.scopeForHost(slug, host)
	if !servable {
		return "", 0, false
	}
	var l Link
	if query.Order("host DESC").First(&l).Error == nil {
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

type statKV struct {
	Key   string `json:"key" gorm:"column:key"`
	Count int64  `json:"count" gorm:"column:count"`
}

type topLink struct {
	ID     uint   `json:"id"`
	Slug   string `json:"slug"`
	Host   string `json:"host"`
	Title  string `json:"title"`
	Tags   string `json:"tags"`
	Clicks int64  `json:"clicks"`
}

func (p *Plugin) overview(orgID uint, includeBot bool) map[string]any {
	tdb := p.tenantDB(orgID)
	if tdb == nil {
		return map[string]any{}
	}
	botFilter := func(q *gorm.DB) *gorm.DB {
		if includeBot {
			return q
		}
		return q.Where("is_bot = ?", false)
	}
	count := func(model any, conds ...any) int64 {
		var n int64
		q := tdb.Model(model)
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

	orgLinks := tdb.Model(&Link{}).Select("id")
	now := time.Now()
	since30 := now.AddDate(0, 0, -30)
	since7 := now.AddDate(0, 0, -7)

	var series []statKV
	botFilter(p.db.Model(&LinkEvent{}).
		Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
		Select(dialectDayBucket(p.db) + " as key, count(*) as count").
		Group("key").Order("key ASC").Scan(&series)

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

	var topLinks []topLink
	tdb.Model(&Link{}).
		Select("id, slug, host, title, tags, clicks").
		Where("archived = ?", false).
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
