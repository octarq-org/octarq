package links

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Handle serves a redirect (or the password gate) and records the click. An
// expired/over-limit link redirects to its ExpiredURL when set, else 404s.
func (e *Engine) Handle(w http.ResponseWriter, r *http.Request, link *Link) {
	if expired(link) {
		if link.ExpiredURL != "" {
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, r, link.ExpiredURL, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	if link.Password != "" {
		provided := r.FormValue("pw")
		if provided == "" {
			provided = r.URL.Query().Get("pw")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(link.Password)) != 1 {
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
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	e.record(r, link.OrgID, link.Slug, link.ID, ip, country, region, city, ua, device, browser, osStr, bot, variant)
	w.Header().Set("Referrer-Policy", "no-referrer")
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
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><meta name="referrer" content="no-referrer">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Protected link</title>
<style>body{font-family:system-ui;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#0b0b0f;color:#fff}
form{background:#16161d;padding:2rem;border-radius:12px;width:300px}
input{width:100%;padding:.6rem;margin:.5rem 0;border-radius:8px;border:1px solid #333;background:#0b0b0f;color:#fff;box-sizing:border-box}
button{width:100%;padding:.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;font-weight:600;cursor:pointer}</style></head>
<body><form method="post" action="` + html.EscapeString(path) + `">
<h3>🔒 This link is protected</h3>
<input type="password" name="pw" placeholder="Password" autofocus>
<button type="submit">Continue</button></form></body></html>`))
}
