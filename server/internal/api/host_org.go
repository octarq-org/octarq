package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/origin"
)

type orgHostRow struct {
	OrgID uint   `gorm:"column:owner_id"`
	Host  string `gorm:"column:host"`
}

func (orgHostRow) TableName() string { return "org_hosts" }

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
	if len(c.entries) > 1024 {
		c.entries = make(map[string]hostOrgEntry)
	}
	c.entries[host] = hostOrgEntry{orgID: orgID, expiry: time.Now().Add(hostOrgTTL)}
}

func normalizeHost(host string) string    { return origin.NormalizeHost(host) }
func hostCandidates(host string) []string { return origin.Candidates(host) }

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
	var rows []orgHostRow
	if err := h.db.Model(&orgHostRow{}).
		Where("host IN ?", candidates).
		Find(&rows).Error; err != nil {
		return 0
	}
	byHost := make(map[string]uint, len(rows))
	ambiguous := make(map[string]bool)
	for _, r := range rows {
		if r.OrgID == 0 {
			continue
		}
		key := strings.ToLower(r.Host)
		if prev, ok := byHost[key]; ok {
			if prev != r.OrgID {
				ambiguous[key] = true
			}
			continue
		}
		byHost[key] = r.OrgID
	}
	for _, c := range candidates {
		if ambiguous[c] {
			return 0
		}
		if orgID, ok := byHost[c]; ok {
			return orgID
		}
	}
	return 0
}

func (h *Handler) brandingOrg(r *http.Request) uint {
	if orgID := h.auth.OrgID(r); orgID != 0 {
		return orgID
	}
	return h.orgIDForHost(r.Host)
}

func (h *Handler) anyRegisteredHost() bool {
	if h == nil || h.host == nil {
		return false
	}
	return h.host.Hosts().Any()
}
