// Package shortlink resolves slugs to targets, records click events
// asynchronously, and renders the password gate when a link is protected.
package links

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/internal/safego"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/origin"
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
	variant     string
	utmSource   string
	utmMedium   string
	utmCampaign string
	createdAt   time.Time
}

// Service handles redirect resolution and analytics.
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

// Lookup finds an enabled, non-archived link for (host, slug), preferring an
// exact host match and falling back to a host-agnostic link. Expiry and click
// limits are evaluated in Handle so an expired link can still honor ExpiredURL.
func (e *Engine) Lookup(host, slug string) (*Link, bool) {
	host = stripPort(host)
	ctx := context.Background()

	var link Link
	cacheKey := "link:redirect:" + host + ":" + slug

	if e.ctx != nil && e.ctx.CacheGet != nil && e.ctx.CacheGet(ctx, cacheKey, &link) {
		if link.ID == 0 {
			// Cached negative result
			return nil, false
		}
		return &link, true
	}

	query, servable := e.scopeForHost(slug, host)
	if !servable {
		e.cacheNegative(ctx, cacheKey)
		return nil, false
	}

	err := query.
		Order("host DESC"). // non-empty host sorts first, so exact match wins
		First(&link).Error
	if err != nil {
		e.cacheNegative(ctx, cacheKey)
		return nil, false
	}
	if !link.Enabled || link.Archived {
		e.cacheNegative(ctx, cacheKey)
		return nil, false
	}
	// A host-scoped link does not resolve if its host is a temporarily disabled
	// link host. Unmanaged hosts (not listed on any domain) are unaffected.
	if link.Host != "" && e.linkHostDisabled(link.OrgID, host) {
		return nil, false
	}

	if e.ctx != nil && e.ctx.CacheSet != nil {
		_ = e.ctx.CacheSet(ctx, cacheKey, &link, time.Hour)
	}
	return &link, true
}

// scopeForHost narrows a slug query to the link rows host may legitimately
// serve, and reports whether host may serve any link at all.
//
// This is the single answer to "whose link is this hostname allowed to
// resolve", shared by the public redirect (Lookup) and by attribution
// (resolveSlug). Two copies of this policy would drift, and a drifted copy is
// how the hijack this replaced worked in the first place.
func (e *Engine) scopeForHost(slug, host string) (*gorm.DB, bool) {
	if owner, ok := e.resolver.OwnerOf(host); ok {
		return e.db.Where("slug = ? AND (host = ? OR host = '')", slug, host).
			Where("owner_id = ?", owner), true
	}
	if e.resolver.ServesTraffic(host) {
		// The host is registered — someone claims it — but OwnerOf refused to
		// name a single owner, which happens exactly when two or more orgs
		// contest it. Serving either side would land one tenant's links on
		// another tenant's hostname, so fail closed: a 404 beats a
		// cross-tenant hijack, and the victim's links are still served on
		// their own registered hosts.
		return nil, false
	}
	// No registered domain covers this host at all: the bare instance
	// hostname, the shared dashboard host (app.octarq.org), an IP literal.
	// Only host-agnostic links (host = '') are unambiguous here — the slug is
	// their only credential, so serving them from a neutral host exposes
	// nothing a link is not already public about. This is the branch mail
	// click tracking runs on (plugins/mail wrapLinksInEmail creates Host:""
	// links and serves them from the shared host when the org has no custom
	// link domain). A Link row that claims this exact host while no domain
	// owns it is an unauthorized claim and must not be served, so the
	// exact-host branch of the query is dropped.
	return e.db.Where("slug = ? AND host = ''", slug), true
}

// cacheNegative records that (host, slug) resolves to nothing for one minute,
// so a spray of invalid short-link requests does not hammer the database.
func (e *Engine) cacheNegative(ctx context.Context, cacheKey string) {
	var empty Link
	if e.ctx != nil && e.ctx.CacheSet != nil {
		_ = e.ctx.CacheSet(ctx, cacheKey, &empty, time.Minute)
	}
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

	// Priority: attribute rules (geo/device/os/language) are evaluated first.
	// Split rules only apply when no attribute rule matched.
	// Any panic or misconfiguration falls back silently to link.Target.
	var variant string
	if len(link.RoutingRules) > 0 {
		lang := r.Header.Get("Accept-Language")
		attributeHit := false
		for _, rule := range link.RoutingRules {
			if rule.Type == "split" {
				continue // handle split group separately below
			}
			if matchRule(rule, country, device, osStr, lang) {
				target = rule.Target
				attributeHit = true
				break
			}
		}
		// Apply split routing only when no attribute rule fired.
		if !attributeHit {
			fingerprint := deviceFingerprint(anonymizeIP(ip), ua, r.Header.Get("Accept-Language"))
			if splitTarget, splitVariant, ok := splitAssign(link.RoutingRules, fingerprint, link.ID); ok {
				if splitTarget != "" {
					target = splitTarget
				}
				variant = splitVariant
			}
		}
	}

	if e.rateLimiter != nil && !e.rateLimiter.Allow(ip) {
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	e.record(r, link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, device, browser, osStr, bot, variant)
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

// matchRule reports whether a non-split routing rule matches the given request attributes.
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

// splitAssign deterministically assigns a visitor to a split-rule variant.
// It returns (target, variantName, true) when a split rule is selected,
// or ("", "", false) when no split rules exist or the residual share falls
// back to the link's own default target (caller treats that as control).
//
// Priority note: this function is called only after attribute rules
// (geo/device/os/language) have been checked and none matched.
//
// Algorithm:
//   - Collect all rules with type == "split".
//   - Derive a stable bucket in [0, 100) by hashing fingerprint + linkID.
//     The link ID is mixed in so that the same visitor does not always land
//     on the same side across every short link in the system.
//   - Walk the rules in order, accumulating weight. The first rule whose
//     cumulative weight exceeds the bucket wins.
//   - Weights summing < 100 leave the remainder as the control/default.
//   - Weights summing > 100 are truncated (no errors; redirect path must
//     never return 5xx due to misconfiguration).
//
// This function is a pure function: (fingerprint, linkID) → variant.
// It never uses rand, time, or counters.
func splitAssign(rules RoutingRules, fingerprint string, linkID uint) (target, variant string, ok bool) {
	var splits []RoutingRule
	for _, r := range rules {
		if r.Type == "split" {
			splits = append(splits, r)
		}
	}
	if len(splits) == 0 {
		return "", "", false
	}

	h := sha256.Sum256([]byte(fingerprint + "\n" + fmt.Sprintf("%d", linkID)))
	bucket := int(binary.BigEndian.Uint32(h[:4]) % 100)

	cum := 0
	for _, r := range splits {
		if r.Weight <= 0 {
			continue
		}
		cum += r.Weight
		if bucket < cum {
			return r.Target, r.Target, true
		}
		if cum >= 100 {
			break // weights exceeded 100; remaining rules are unreachable
		}
	}
	// Residual share goes to the control (link.Target). Caller handles this.
	return "", "control", true
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

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// record writes a click event and increments the counter in the background.
func (e *Engine) record(r *http.Request, orgID uint, slug string, linkID uint, ip, country, region, city, ua string, device, browser, osStr string, bot bool, variant string) {
	referer := r.Referer()
	anonIP := anonymizeIP(ip)
	fingerprint := deviceFingerprint(anonIP, ua, r.Header.Get("Accept-Language"))
	utmSource := truncateString(r.URL.Query().Get("utm_source"), 128)
	utmMedium := truncateString(r.URL.Query().Get("utm_medium"), 128)
	utmCampaign := truncateString(r.URL.Query().Get("utm_campaign"), 128)
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
		variant:     variant,
		utmSource:   utmSource,
		utmMedium:   utmMedium,
		utmCampaign: utmCampaign,
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
