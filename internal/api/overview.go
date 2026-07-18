package api

import (
	"context"
	"time"

	mailmodels "github.com/octarq-org/octarq/plugins/mail"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"github.com/octarq-org/octarq/plugins/links"
	"gorm.io/gorm"
)

type OverviewInput struct {
	Ctx        huma.Context `hidden:"true"`
	IncludeBot bool         `query:"includeBot"`
}

func (i *OverviewInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type OverviewOutput struct {
	Body map[string]any
}

// overview returns aggregate dashboard statistics for the home page.
// Query param: includeBot=true — when present, bot clicks are counted alongside
// human clicks so the caller can compare bot vs human traffic.
func (h *Handler) overview(ctx context.Context, input *OverviewInput) (*OverviewOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	go h.auth.TouchSession(r)
	now := time.Now()
	since30 := now.AddDate(0, 0, -30)
	since7 := now.AddDate(0, 0, -7)
	org := h.orgID(r)
	includeBot := input.IncludeBot

	// botFilter applies is_bot=false unless the caller explicitly wants bot traffic.
	botFilter := func(q *gorm.DB) *gorm.DB {
		if includeBot {
			return q
		}
		return q.Where("is_bot = ?", false)
	}

	count := func(model any, conds ...any) int64 {
		var n int64
		q := h.db.Model(model).Where("owner_id = ?", org)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	var totalClicks int64
	botFilter(h.db.Model(&links.LinkEvent{}).
		Joins("JOIN links ON links.id = link_events.link_id AND links.owner_id = ?", org)).
		Select("COUNT(*)").Scan(&totalClicks)

	orgLinks := h.db.Model(&links.Link{}).Select("id").Where("owner_id = ?", org)

	// Daily click series for the last 30 days.
	var series []models.StatKV
	botFilter(h.db.Model(&links.LinkEvent{}).
		Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
		Select("strftime('%Y-%m-%d', created_at) as key, count(*) as count").
		Group("key").Order("key ASC").Scan(&series)
	if len(series) == 0 && h.cfg.DBDriver == "postgres" {
		botFilter(h.db.Model(&links.LinkEvent{}).
			Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
			Select("to_char(created_at, 'YYYY-MM-DD') as key, count(*) as count").
			Group("key").Order("key ASC").Scan(&series)
	}

	top := func(col string) []models.StatKV {
		var rows []models.StatKV
		q := botFilter(h.db.Model(&links.LinkEvent{}).
			Where("link_id IN (?) AND created_at >= ? AND "+col+" <> ''", orgLinks, since30))
		if col == "device" {
			// Dedup by device fingerprint; fall back to ip+ua for rows recorded
			// before fingerprints were captured.
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
	h.db.Model(&links.Link{}).
		Select("id, slug, host, clicks").
		Where("owner_id = ? AND archived = ?", org, false).
		Order("clicks DESC").Limit(5).Scan(&topLinks)

	type recentEmail struct {
		ID         uint      `json:"id"`
		FromAddr   string    `json:"from"`
		Subject    string    `json:"subject"`
		Read       bool      `json:"read"`
		ReceivedAt time.Time `json:"receivedAt"`
	}
	orgMailboxes := h.db.Model(&mailmodels.Mailbox{}).Select("id").Where("owner_id = ?", org)
	var recent []recentEmail
	h.db.Model(&mailmodels.Email{}).
		Select("id, from_addr, subject, read, received_at").
		Where("mailbox_id IN (?)", orgMailboxes).
		Order("received_at DESC").Limit(6).Scan(&recent)

	emailCount := func(conds ...any) int64 {
		var n int64
		q := h.db.Model(&mailmodels.Email{}).Where("mailbox_id IN (?)", orgMailboxes)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	clickCount := func(conds ...any) int64 {
		var n int64
		q := botFilter(h.db.Model(&links.LinkEvent{}).Where("link_id IN (?)", orgLinks))
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	// Bot-only counts always returned so the frontend can show the split.
	botCount := func(conds ...any) int64 {
		var n int64
		q := h.db.Model(&links.LinkEvent{}).Where("link_id IN (?) AND is_bot = ?", orgLinks, true)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	out := &OverviewOutput{
		Body: map[string]any{
			"links":        count(&links.Link{}),
			"activeLinks":  count(&links.Link{}, "archived = ? AND enabled = ?", false, true),
			"domains":      count(&dns.Domain{}),
			"linkDomains":  count(&dns.Domain{}, "for_link = ?", true),
			"mailDomains":  count(&dns.Domain{}, "for_mail = ?", true),
			"mailboxes":    count(&mailmodels.Mailbox{}),
			"emails":       emailCount(),
			"unread":       emailCount("read = ?", false),
			"tokens":       count(&models.Token{}),
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
			"recentEmails": recent,
			"includeBot":   includeBot,
		},
	}
	return out, nil
}
