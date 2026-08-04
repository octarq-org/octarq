// Package shortlink resolves slugs to targets, records click events
// asynchronously, and renders the password gate when a link is protected.
package links

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"html"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/internal/safego"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
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
	createdAt   time.Time
}

// Service handles redirect resolution and analytics.
type Engine struct {
	db          *gorm.DB
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
	events := make([]LinkEvent, len(batch))
	clicksByLink := make(map[uint]int64)
	clicksByOrg := make(map[uint]int64)

	for i, item := range batch {
		events[i] = LinkEvent{
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
		}
		if !item.bot {
			clicksByLink[item.linkID]++
			clicksByOrg[item.orgID]++
		}
	}

	err := e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&events).Error; err != nil {
			return err
		}
		for linkID, count := range clicksByLink {
			if err := tx.Model(&Link{}).Where("id = ?", linkID).
				UpdateColumn("clicks", gorm.Expr("clicks + ?", count)).Error; err != nil {
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
			for orgID, count := range clicksByOrg {
				e.ctx.RecordUsage(orgID, "links", count)
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

// Lookup finds an enabled, non-archived link for (host, slug), preferring an
// exact host match and falling back to a host-agnostic link. Expiry and click
// limits are evaluated in Handle so an expired link can still honor ExpiredURL.
func (e *Engine) Lookup(host, slug string) (*Link, bool) {
	host = stripPort(host)
	ctx := context.Background()

	var link Link
	cacheKey := "link:redirect:" + host + ":" + slug

	// Try reading from cache first
	if e.ctx != nil && e.ctx.CacheGet != nil && e.ctx.CacheGet(ctx, cacheKey, &link) {
		if link.ID == 0 {
			// Cached negative result
			return nil, false
		}
		return &link, true
	}

	query := e.db.Where("slug = ? AND (host = ? OR host = '')", slug, host)
	if ownerOrg := resolveHostOrg(e.db, host); ownerOrg != 0 {
		query = query.Where("owner_id = ?", ownerOrg)
	}

	err := query.
		Order("host DESC"). // non-empty host sorts first, so exact match wins
		First(&link).Error
	if err != nil {
		// Cache negative result (1 minute TTL) to prevent DB hammering for invalid links
		var empty Link
		if e.ctx != nil && e.ctx.CacheSet != nil {
			_ = e.ctx.CacheSet(ctx, cacheKey, &empty, time.Minute)
		}
		return nil, false
	}
	if !link.Enabled || link.Archived {
		var empty Link
		if e.ctx != nil && e.ctx.CacheSet != nil {
			_ = e.ctx.CacheSet(ctx, cacheKey, &empty, time.Minute)
		}
		return nil, false
	}
	// A host-scoped link does not resolve if its host is a temporarily disabled
	// link host. Unmanaged hosts (not listed on any domain) are unaffected.
	if link.Host != "" && e.linkHostDisabled(link.OrgID, host) {
		return nil, false
	}

	// Cache successful result (1 hour TTL)
	if e.ctx != nil && e.ctx.CacheSet != nil {
		_ = e.ctx.CacheSet(ctx, cacheKey, &link, time.Hour)
	}
	return &link, true
}

// linkHostDisabled reports whether host is listed as a link host by orgID but
// every such listing is disabled. Unmanaged hosts (not listed on that org's
// domains) are unaffected.
//
// orgID is passed explicitly because host strings are not unique across tenants;
// matching on host name alone would leak tenant isolation.
func (e *Engine) linkHostDisabled(orgID uint, host string) bool {
	if orgID == 0 {
		return false
	}
	normHost := normalizeHost(host)
	var doms []dns.Domain
	e.db.Where("owner_id = ? AND for_link = ?", orgID, true).Find(&doms)
	listed := false
	for _, d := range doms {
		for _, h := range d.LinkHosts {
			if normalizeHost(h.Host) == normHost {
				listed = true
				if h.Enabled {
					return false
				}
			}
		}
	}
	return listed
}

// expired reports whether a link is past its expiry or over its click limit.
func expired(link *Link) bool {
	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		return true
	}
	if link.ClickLimit > 0 && link.Clicks >= link.ClickLimit {
		return true
	}
	return false
}

// Handle serves a redirect (or the password gate) and records the click. An
// expired/over-limit link redirects to its ExpiredURL when set, else 404s.
func (e *Engine) Handle(w http.ResponseWriter, r *http.Request, link *Link) {
	if expired(link) {
		if link.ExpiredURL != "" {
			http.Redirect(w, r, link.ExpiredURL, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	if link.Password != "" {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("pw")), []byte(link.Password)) != 1 {
			renderPasswordGate(w, r.URL.Path)
			return
		}
	}

	ip := clientIP(r)
	ua := r.UserAgent()
	var country, region, city string
	if e.ctx != nil && e.ctx.GeoLookup != nil {
		country, region, city = e.ctx.GeoLookup(ip)
	}
	var device, browser, osStr string
	if e.ctx != nil && e.ctx.ParseUA != nil {
		device, browser, osStr = e.ctx.ParseUA(ua)
	}
	bot := isBot(ua)

	target := link.Target

	if len(link.RoutingRules) > 0 {
		lang := r.Header.Get("Accept-Language")
		for _, rule := range link.RoutingRules {
			if matchRule(rule, country, device, osStr, lang) {
				target = rule.Target
				break
			}
		}
	}

	if e.rateLimiter != nil && !e.rateLimiter.Allow(ip) {
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	e.record(r, link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, device, browser, osStr, bot)
	http.Redirect(w, r, target, http.StatusFound)
}

var botSignatures = []string{
	"bot", "spider", "crawl", "slurp",
	"googlebot", "bingbot", "yandexbot", "duckduckbot", "baiduspider",
	"facebookexternalhit", "twitterbot", "linkedinbot", "whatsapp", "slackbot", "telegrambot",
	"discordbot", "skypeuripreview",
}

func isBot(ua string) bool {
	uaLower := strings.ToLower(ua)
	for _, sig := range botSignatures {
		if strings.Contains(uaLower, sig) {
			return true
		}
	}
	return false
}

func matchRule(rule RoutingRule, country, device, os, lang string) bool {
	matchLower := strings.ToLower(rule.Match)
	switch rule.Type {
	case "geo":
		return strings.ToLower(country) == matchLower
	case "device":
		return strings.ToLower(device) == matchLower
	case "os":
		return strings.ToLower(os) == matchLower
	case "language":
		return strings.Contains(strings.ToLower(lang), matchLower)
	}
	return false
}

// anonymizeIP truncates the last octet of IPv4 (1.2.3.4 → 1.2.3.0) and the
// last 80 bits of IPv6 to avoid storing personally identifiable data.
func anonymizeIP(raw string) string {
	ip := net.ParseIP(raw)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		v4[3] = 0
		return v4.String()
	}
	// IPv6: zero the last 10 bytes (80 bits), keep the first 6 bytes (48 bits).
	v6 := ip.To16()
	for i := 6; i < 16; i++ {
		v6[i] = 0
	}
	return v6.String()
}

// deviceFingerprint derives a stable, privacy-preserving per-device hash from
// the already-anonymized IP, the user-agent, and the Accept-Language header.
// It carries no more PII than the fields already stored, but collapses repeat
// visits from one device into a single value so analytics can dedup them.
func deviceFingerprint(anonIP, ua, acceptLang string) string {
	sum := sha256.Sum256([]byte(anonIP + "\n" + ua + "\n" + acceptLang))
	return hex.EncodeToString(sum[:16]) // 128-bit hex, fits size:64
}

// record writes a click event and increments the counter in the background.
func (e *Engine) record(r *http.Request, orgID uint, slug string, linkID uint, ip, country, region, city, ua string, device, browser, osStr string, bot bool) {
	referer := r.Referer()
	anonIP := anonymizeIP(ip)
	fingerprint := deviceFingerprint(anonIP, ua, r.Header.Get("Accept-Language"))
	item := clickItem{
		orgID:       orgID,
		slug:        slug,
		linkID:      linkID,
		ip:          anonIP,
		country:     country,
		region:      region,
		city:        city,
		ua:          ua,
		device:      device,
		browser:     browser,
		osStr:       osStr,
		bot:         bot,
		referer:     referer,
		fingerprint: fingerprint,
		createdAt:   time.Now(),
	}

	select {
	case e.queue <- item:
	default:
		cnt := e.dropCount.Add(1)
		slog.Warn("link click event dropped due to full queue", "dropped_total", cnt, "link_id", linkID)
	}
}

// trustProxy gates whether proxy-supplied client-IP headers are honoured when
// attributing analytics. Set once from config via SetTrustProxy; when false,
// X-Forwarded-For / X-Real-IP are ignored so a client can't spoof its IP (and
// thus its geo/attribution) by sending those headers directly.
var trustProxy bool

// SetTrustProxy configures whether proxy-supplied client-IP headers are trusted.
// Call once at startup from config, mirroring auth.New / server construction.
func SetTrustProxy(v bool) { trustProxy = v }

func clientIP(r *http.Request) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
		if rip := strings.TrimSpace(r.Header.Get("X-Real-IP")); rip != "" {
			return rip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func renderPasswordGate(w http.ResponseWriter, path string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Protected link</title>
<style>body{font-family:system-ui;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#0b0b0f;color:#fff}
form{background:#16161d;padding:2rem;border-radius:12px;width:300px}
input{width:100%;padding:.6rem;margin:.5rem 0;border-radius:8px;border:1px solid #333;background:#0b0b0f;color:#fff;box-sizing:border-box}
button{width:100%;padding:.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;font-weight:600;cursor:pointer}</style></head>
<body><form method="get" action="` + html.EscapeString(path) + `">
<h3>🔒 This link is protected</h3>
<input type="password" name="pw" placeholder="Password" autofocus>
<button type="submit">Continue</button></form></body></html>`))
}
