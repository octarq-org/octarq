// Package shortlink resolves slugs to targets, records click events
// asynchronously, and renders the password gate when a link is protected.
package shortlink

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jungley/led/internal/geo"
	"github.com/jungley/led/internal/models"
	"gorm.io/gorm"
)

// Service handles redirect resolution and analytics.
type Service struct {
	db  *gorm.DB
	geo *geo.Resolver
}

func New(db *gorm.DB, g *geo.Resolver) *Service {
	return &Service{db: db, geo: g}
}

// Lookup finds an enabled, non-archived link for (host, slug), preferring an
// exact host match and falling back to a host-agnostic link. Expiry and click
// limits are evaluated in Handle so an expired link can still honor ExpiredURL.
func (s *Service) Lookup(host, slug string) (*models.Link, bool) {
	host = stripPort(host)
	var link models.Link
	err := s.db.Where("slug = ? AND (host = ? OR host = '')", slug, host).
		Order("host DESC"). // non-empty host sorts first, so exact match wins
		First(&link).Error
	if err != nil {
		return nil, false
	}
	if !link.Enabled || link.Archived {
		return nil, false
	}
	// A host-scoped link does not resolve if its host is a temporarily disabled
	// link host. Unmanaged hosts (not listed on any domain) are unaffected.
	if link.Host != "" && s.linkHostDisabled(host) {
		return nil, false
	}
	return &link, true
}

// linkHostDisabled reports whether host is listed as a link host on some domain
// but every such listing is disabled.
func (s *Service) linkHostDisabled(host string) bool {
	var doms []models.Domain
	s.db.Where("for_link = ?", true).Find(&doms)
	listed := false
	for _, d := range doms {
		for _, h := range d.LinkHosts {
			if h.Host == host {
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
func expired(link *models.Link) bool {
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
func (s *Service) Handle(w http.ResponseWriter, r *http.Request, link *models.Link) {
	if expired(link) {
		if link.ExpiredURL != "" {
			http.Redirect(w, r, link.ExpiredURL, http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	if link.Password != "" {
		if r.URL.Query().Get("pw") != link.Password {
			renderPasswordGate(w, r.URL.Path)
			return
		}
	}

	ip := clientIP(r)
	ua := r.UserAgent()
	country, region, city := s.geo.Locate(ip)
	info := geo.ParseUA(ua)
	bot := isBot(ua)

	target := link.Target

	if len(link.RoutingRules) > 0 {
		lang := r.Header.Get("Accept-Language")
		for _, rule := range link.RoutingRules {
			if matchRule(rule, country, info.Device, info.OS, lang) {
				target = rule.Target
				break
			}
		}
	}

	s.record(r, link.ID, ip, country, region, city, ua, info, bot)
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

func matchRule(rule models.RoutingRule, country, device, os, lang string) bool {
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

// record writes a click event and increments the counter in the background.
func (s *Service) record(r *http.Request, linkID uint, ip, country, region, city, ua string, info geo.UAInfo, bot bool) {
	referer := r.Referer()
	go func() {
		ev := models.LinkEvent{
			LinkID: linkID, CreatedAt: time.Now(),
			IP: ip, Country: country, Region: region, City: city,
			Device: info.Device, Browser: info.Browser, OS: info.OS,
			Referer: referer, UA: ua, IsBot: bot,
		}
		s.db.Create(&ev)
		if !bot {
			s.db.Model(&models.Link{}).Where("id = ?", linkID).
				UpdateColumn("clicks", gorm.Expr("clicks + 1"))
		}
	}()
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
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
<body><form method="get" action="` + path + `">
<h3>🔒 This link is protected</h3>
<input type="password" name="pw" placeholder="Password" autofocus>
<button type="submit">Continue</button></form></body></html>`))
}
