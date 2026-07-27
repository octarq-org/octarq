package api

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Resolving a workspace from the request Host.
//
// Per-tenant branding has to work BEFORE anyone logs in — the login screen is
// exactly where a white-labelled deployment must not say "Octarq". There is no
// session at that point, so the only tenant signal available is the hostname the
// browser asked for. A tenant reaching the dashboard on their own domain gets
// their branding; the same dashboard on the shared host (app.octarq.org) belongs
// to no tenant and falls back to the instance default.
//
// This deliberately reads NOTHING the client controls except Host itself. Host
// is attacker-settable, so the only thing it may ever select is *presentation*:
// never authorization, never which tenant's data a handler touches.

// domainRow mirrors the dns plugin's domains table. Core can't import the
// plugin (the dependency runs the other way), and the branding lookup needs
// exactly two columns, so a local mirror is cheaper than a service round-trip.
type domainRow struct {
	OrgID uint   `gorm:"column:owner_id"`
	Name  string `gorm:"column:name"`
}

func (domainRow) TableName() string { return "domains" }

// hostOrgTTL bounds how long a host→org answer is reused. /api/auth/config is
// public and unauthenticated, so an attacker can spray arbitrary Host headers at
// it; without a cache that is an unauthenticated DB-query amplifier. Misses are
// cached too, for the same reason.
const hostOrgTTL = 5 * time.Minute

type hostOrgEntry struct {
	orgID  uint
	expiry time.Time
}

type hostOrgCache struct {
	mu      sync.RWMutex
	entries map[string]hostOrgEntry
}

func (c *hostOrgCache) get(host string) (uint, bool) {
	c.mu.RLock()
	e, ok := c.entries[host]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiry) {
		return 0, false
	}
	return e.orgID, true
}

func (c *hostOrgCache) put(host string, orgID uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]hostOrgEntry)
	}
	// Bound the map: a Host-header sprayer would otherwise grow it without limit.
	// Wholesale reset beats LRU bookkeeping for a cache this cold.
	if len(c.entries) > 1024 {
		c.entries = make(map[string]hostOrgEntry)
	}
	c.entries[host] = hostOrgEntry{orgID: orgID, expiry: time.Now().Add(hostOrgTTL)}
}

// normalizeHost lowercases the Host header and strips the port and any trailing
// dot, so "Acme.COM:8443" and "acme.com." both resolve like "acme.com".
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	// Strip port. IPv6 literals arrive bracketed ("[::1]:8080"), so cut after the
	// closing bracket rather than at the first colon.
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

// hostCandidates returns the hostname and each parent domain down to (but not
// including) the public suffix guess, so "mail.app.acme.com" tries
// "mail.app.acme.com", "app.acme.com", "acme.com". The registered domain is
// usually what lives in the domains table while the dashboard runs on a
// subdomain of it.
//
// It stops at two labels. That over-matches on multi-part suffixes like
// "co.uk" — a tenant owning "acme.co.uk" is found at the 3-label step first, so
// the extra "co.uk" probe only ever costs one wasted lookup and can only match
// if someone actually registered "co.uk" as their own domain row.
func hostCandidates(host string) []string {
	if host == "" || strings.HasPrefix(host, "[") {
		return nil
	}
	// A bare IP is nobody's branded domain.
	if isNumericHost(host) {
		return nil
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return nil
	}
	out := make([]string, 0, len(labels)-1)
	for i := 0; i+1 < len(labels); i++ {
		out = append(out, strings.Join(labels[i:], "."))
	}
	return out
}

// isNumericHost reports whether every label is numeric (an IPv4 literal).
func isNumericHost(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		for _, c := range label {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// orgIDForHost resolves the workspace that owns the request's hostname, or 0
// when the host belongs to no tenant (the shared dashboard host, an IP, or a
// build with no dns plugin and therefore no domains table).
//
// PRESENTATION ONLY. The Host header is client-controlled: the caller has proven
// nothing by sending it. Never use this to scope a query for tenant data or to
// decide access — it selects branding and nothing else.
func (h *Handler) orgIDForHost(host string) uint {
	host = normalizeHost(host)
	if host == "" || h.db == nil {
		return 0
	}
	if orgID, ok := h.hostOrgs.get(host); ok {
		return orgID
	}
	orgID := h.lookupOrgByHost(host)
	h.hostOrgs.put(host, orgID)
	return orgID
}

func (h *Handler) lookupOrgByHost(host string) uint {
	candidates := hostCandidates(host)
	if len(candidates) == 0 {
		return 0
	}
	// The dns plugin owns the domains table; an OSS build composed without it has
	// no table and every host is simply unbranded.
	if !h.db.Migrator().HasTable("domains") {
		return 0
	}
	// One query for the whole candidate set, then pick the most specific match —
	// "app.acme.com" beats "acme.com" if a tenant registered both.
	var rows []domainRow
	if err := h.db.Model(&domainRow{}).
		Where("name IN ?", candidates).
		Find(&rows).Error; err != nil {
		return 0
	}
	byName := make(map[string]uint, len(rows))
	for _, r := range rows {
		if r.OrgID != 0 {
			byName[strings.ToLower(r.Name)] = r.OrgID
		}
	}
	for _, c := range candidates { // candidates run most-specific first
		if orgID, ok := byName[c]; ok {
			return orgID
		}
	}
	return 0
}

// brandingOrg picks the workspace whose branding a request should render: the
// authenticated session's org when there is one, otherwise the org that owns the
// request hostname. Post-login the session always wins, so a member browsing
// from any host sees their own workspace's brand.
func (h *Handler) brandingOrg(r *http.Request) uint {
	if orgID := h.auth.OrgID(r); orgID != 0 {
		return orgID
	}
	return h.orgIDForHost(r.Host)
}
