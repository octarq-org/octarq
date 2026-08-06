package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/internal/origin"
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

// normalizeHost and hostCandidates live in internal/origin, which owns hostname
// parsing for the whole product: the same two transformations decide whether a
// request host may become an outbound link's origin, so a second copy here
// would be a second answer to the same question.
func normalizeHost(host string) string    { return origin.NormalizeHost(host) }
func hostCandidates(host string) []string { return origin.Candidates(host) }

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
