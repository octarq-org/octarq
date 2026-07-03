package api

import (
	"net/http"
	"time"

	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

// overview returns aggregate dashboard statistics for the home page.
// Query param: includeBot=true — when present, bot clicks are counted alongside
// human clicks so the caller can compare bot vs human traffic.
func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	go h.auth.TouchSession(r)
	now := time.Now()
	since30 := now.AddDate(0, 0, -30)
	since7 := now.AddDate(0, 0, -7)
	org := h.orgID(r)
	includeBot := r.URL.Query().Get("includeBot") == "true"

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
	botFilter(h.db.Model(&models.LinkEvent{}).
		Joins("JOIN links ON links.id = link_events.link_id AND links.owner_id = ?", org)).
		Select("COUNT(*)").Scan(&totalClicks)

	orgLinks := h.db.Model(&models.Link{}).Select("id").Where("owner_id = ?", org)

	// Daily click series for the last 30 days.
	var series []statKV
	botFilter(h.db.Model(&models.LinkEvent{}).
		Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
		Select("strftime('%Y-%m-%d', created_at) as key, count(*) as count").
		Group("key").Order("key ASC").Scan(&series)
	if len(series) == 0 && h.cfg.DBDriver == "postgres" {
		botFilter(h.db.Model(&models.LinkEvent{}).
			Where("link_id IN (?) AND created_at >= ?", orgLinks, since30)).
			Select("to_char(created_at, 'YYYY-MM-DD') as key, count(*) as count").
			Group("key").Order("key ASC").Scan(&series)
	}

	top := func(col string) []statKV {
		var rows []statKV
		botFilter(h.db.Model(&models.LinkEvent{}).
			Where("link_id IN (?) AND created_at >= ? AND "+col+" <> ''", orgLinks, since30)).
			Select(col+" as key, count(*) as count").
			Group(col).Order("count DESC").Limit(8).Scan(&rows)
		return rows
	}

	type topLink struct {
		ID     uint   `json:"id"`
		Slug   string `json:"slug"`
		Host   string `json:"host"`
		Clicks int64  `json:"clicks"`
	}
	var topLinks []topLink
	h.db.Model(&models.Link{}).
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
	orgMailboxes := h.db.Model(&models.Mailbox{}).Select("id").Where("owner_id = ?", org)
	var recent []recentEmail
	h.db.Model(&models.Email{}).
		Select("id, from_addr, subject, read, received_at").
		Where("mailbox_id IN (?)", orgMailboxes).
		Order("received_at DESC").Limit(6).Scan(&recent)

	emailCount := func(conds ...any) int64 {
		var n int64
		q := h.db.Model(&models.Email{}).Where("mailbox_id IN (?)", orgMailboxes)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	clickCount := func(conds ...any) int64 {
		var n int64
		q := botFilter(h.db.Model(&models.LinkEvent{}).Where("link_id IN (?)", orgLinks))
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	// Bot-only counts always returned so the frontend can show the split.
	botCount := func(conds ...any) int64 {
		var n int64
		q := h.db.Model(&models.LinkEvent{}).Where("link_id IN (?) AND is_bot = ?", orgLinks, true)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"links":        count(&models.Link{}),
		"activeLinks":  count(&models.Link{}, "archived = ? AND enabled = ?", false, true),
		"domains":      count(&models.Domain{}),
		"linkDomains":  count(&models.Domain{}, "for_link = ?", true),
		"mailDomains":  count(&models.Domain{}, "for_mail = ?", true),
		"mailboxes":    count(&models.Mailbox{}),
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
		"recentEmails": recent,
		"includeBot":   includeBot,
	})
}
