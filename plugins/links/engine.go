package links

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/internal/safego"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/origin"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type ipRateLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	resets map[string]time.Time
	limit  int
	window time.Duration
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		counts: make(map[string]int),
		resets: make(map[string]time.Time),
		limit:  limit,
		window: window,
	}
}

func (l *ipRateLimiter) Allow(ip string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	reset, ok := l.resets[ip]
	if !ok || now.After(reset) {
		l.counts[ip] = 1
		l.resets[ip] = now.Add(l.window)
		return true
	}

	if l.counts[ip] >= l.limit {
		return false
	}
	l.counts[ip]++
	return true
}

type clickItem struct {
	orgID       uint
	slug        string
	linkID      uint
	ip          string
	country     string
	region      string
	city        string
	ua          string
	device      string
	browser     string
	osStr       string
	bot         bool
	referer     string
	fingerprint string
	variant     string
	utmSource   string
	utmMedium   string
	utmCampaign string
	createdAt   time.Time
}

// Engine handles redirect resolution and analytics.
type Engine struct {
	db          *gorm.DB
	resolver    *origin.Resolver
	ctx         *plugin.Context
	queue       chan clickItem
	wg          sync.WaitGroup
	closeOnce   sync.Once
	dropCount   atomic.Uint64
	txCount     atomic.Uint64
	rateLimiter *ipRateLimiter
}

func NewEngine(db *gorm.DB, ctx *plugin.Context) *Engine {
	e := &Engine{
		db:          db,
		resolver:    origin.NewResolver(db),
		ctx:         ctx,
		queue:       make(chan clickItem, 5000),
		rateLimiter: newIPRateLimiter(300, time.Minute),
	}
	e.wg.Add(1)
	go e.worker()
	return e
}

func (e *Engine) SetRateLimit(limit int, window time.Duration) {
	e.rateLimiter = newIPRateLimiter(limit, window)
}

func (e *Engine) Close() {
	e.closeOnce.Do(func() {
		close(e.queue)
		e.wg.Wait()
	})
}

func (e *Engine) DropCount() uint64 {
	return e.dropCount.Load()
}

func (e *Engine) TxCount() uint64 {
	return e.txCount.Load()
}

func (e *Engine) worker() {
	defer e.wg.Done()
	batch := make([]clickItem, 0, 100)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		e.flushBatch(batch)
		batch = batch[:0]
	}

	// consume runs one iteration of the event loop. It returns true when the
	// channel is closed (normal shutdown) — the caller must stop looping.
	// A panic inside flushBatch / PublishEvent is caught by safego.Recover
	// and consume returns false so the outer loop restarts it.
	consume := func() (closed bool) {
		defer safego.Recover("links.click-worker")
		for {
			select {
			case item, ok := <-e.queue:
				if !ok {
					flush()
					return true
				}
				batch = append(batch, item)
				if len(batch) >= 100 {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}

	for !consume() {
		// After a panic-recovery, re-enter the loop. The channel and
		// batch slice survive because they live in the outer scope.
		// Reset the batch — whatever was in it may be half-processed.
		batch = batch[:0]
	}
}

func (e *Engine) flushBatch(batch []clickItem) {
	if len(batch) == 0 {
		return
	}

	// Count non-bot clicks per org up front: the per-org totals are what the
	// quota check below judges, and an over-quota click must not land anywhere.
	clicksByOrg := make(map[uint]int64)
	for _, item := range batch {
		if !item.bot {
			clicksByOrg[item.orgID]++
		}
	}

	// Ask the quota checker once per org before writing anything. Redirects are
	// never refused — short links get printed on QR codes and campaign material,
	// and stopping the 302 would break the link owner's customers irrecoverably,
	// while the write-and-store cost is what actually bills. So an org that has
	// used up its monthly click allowance simply stops being counted. Only an
	// explicit ErrQuotaExceeded suppresses; any other error (a broken checker,
	// an unknown one) reads as "allowed" so a metering outage never becomes
	// silent data loss. With no checker registered (self-hosted) CheckQuota
	// returns nil and nothing is suppressed.
	suppressed := make(map[uint]bool)
	for orgID, count := range clicksByOrg {
		if errors.Is(plugin.CheckQuota(e.ctx, context.Background(), orgID, "clicksPerMonth", count), plugin.ErrQuotaExceeded) {
			suppressed[orgID] = true
		}
	}

	events := make([]LinkEvent, 0, len(batch))
	clicksByLink := make(map[uint]int64)
	for _, item := range batch {
		// Suppression is all-or-nothing per org: drop both the event row and the
		// Link.clicks increment. Skipping the event while still bumping the
		// counter would make the link detail page's total disagree with the event
		// table — worse than not counting at all.
		if suppressed[item.orgID] {
			continue
		}
		events = append(events, LinkEvent{
			LinkID:      item.linkID,
			CreatedAt:   item.createdAt,
			IP:          item.ip,
			Country:     item.country,
			Region:      item.region,
			City:        item.city,
			Device:      item.device,
			Browser:     item.browser,
			OS:          item.osStr,
			Referer:     item.referer,
			UA:          item.ua,
			Fingerprint: item.fingerprint,
			IsBot:       item.bot,
			Variant:     item.variant,
			UTMSource:   item.utmSource,
			UTMMedium:   item.utmMedium,
			UTMCampaign: item.utmCampaign,
		})
		if !item.bot {
			clicksByLink[item.linkID]++
		}
	}

	err := e.db.Transaction(func(tx *gorm.DB) error {
		// A batch can be entirely suppressed (every item over quota). GORM
		// treats Create on an empty slice as an error, so only write when
		// there is actually an event row to persist.
		if len(events) > 0 {
			if err := tx.Create(&events).Error; err != nil {
				return err
			}
		}
		if len(clicksByLink) > 0 {
			var queryBuilder strings.Builder
			queryBuilder.WriteString("UPDATE links SET clicks = clicks + CASE id")

			args := make([]interface{}, 0, len(clicksByLink)*2+1)
			ids := make([]uint, 0, len(clicksByLink))

			for linkID, count := range clicksByLink {
				queryBuilder.WriteString(" WHEN ? THEN ?")
				args = append(args, linkID, count)
				ids = append(ids, linkID)
			}

			queryBuilder.WriteString(" ELSE 0 END WHERE id IN ?")
			args = append(args, ids)

			if err := tx.Exec(queryBuilder.String(), args...).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		e.txCount.Add(1)
	} else {
		slog.Error("failed to flush click events batch", "count", len(batch), "err", err)
	}

	if e.ctx != nil {
		if e.ctx.RecordUsage != nil {
			// Meter only the clicks that were actually written. A suppressed org
			// consumed nothing on disk, so metering it would bill for clicks the
			// product never counted — the cap exists to stop the bill, not to
			// keep it growing. The metric is "clicks", not "links": "links" is
			// the stock-quota key for how many short links an org may hold,
			// a different thing that would collide if clicks were reported
			// under the same name.
			for orgID, count := range clicksByOrg {
				if suppressed[orgID] {
					continue
				}
				e.ctx.RecordUsage(orgID, usagemetric.Clicks, count)
			}
		}
		if e.ctx.PublishEvent != nil {
			for _, item := range batch {
				e.ctx.PublishEvent(item.orgID, "link.click", map[string]any{
					"linkId":    item.linkID,
					"slug":      item.slug,
					"ip":        item.ip,
					"country":   item.country,
					"region":    item.region,
					"city":      item.city,
					"device":    item.device,
					"browser":   item.browser,
					"os":        item.osStr,
					"referer":   item.referer,
					"isBot":     item.bot,
					"timestamp": item.createdAt,
				})
			}
		}
	}
}
