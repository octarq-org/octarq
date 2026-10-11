package origin

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// ttl bounds how long an ownership answer is reused. The endpoints that build
// absolute URLs are unauthenticated and the dashboard gate runs on every page
// load, so without a cache an attacker spraying Host headers is a free DB-query
// amplifier. Negative answers are cached for exactly that reason.
const ttl = 5 * time.Minute

// entry holds one cached answer. org is meaningful only for OwnerOf's key
// space; the boolean questions leave it zero.
//
// The cache is package-global rather than per-Resolver because a process builds
// several Resolvers over the same database — the API, the server and the OAuth
// handler each hold one — and a domain registered through any of them has to
// become visible to all of them at once. Per-Resolver caches meant up to ttl
// where one half of the process served an origin the other half refused.
var (
	globalCacheMu sync.RWMutex
	globalCache   = make(map[string]entry)
)

type entry struct {
	ok     bool
	org    uint
	name   string   // cachedBaseDomain's value
	list   []string // cachedSharedHosts's value
	expiry time.Time
}

// namespace identifies the database an answer was computed against. Every key
// carries it, so Resolvers over *different* databases cannot read each other's
// answers.
//
// That only happens under test — production has one database — and it is not a
// theoretical tidiness: an answer here decides whether a forged Host is
// honoured. A test that registers no domain caches "no whitelist exists", and
// without this prefix the next test's Resolver reads that back over its own,
// domain-bearing database and accepts "evil.com".
//
// The identity is the *sql.DB, not the *gorm.DB: gorm hands out a fresh
// *gorm.DB for every chained call, while the connection pool underneath is
// shared and stable. Two Resolvers over one database therefore share a
// namespace, which is exactly what the cross-Resolver invalidation above needs.
func namespace(db *gorm.DB) string {
	if db == nil {
		return "nodb|"
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		return "nodb|"
	}
	return fmt.Sprintf("%p|", sqlDB)
}

// ClearDomainCache removes all cached answers related to the given domain,
// across every namespace. Call it whenever the domains table changes — a
// register, a delete, a move between workspaces — or the change is invisible
// for up to ttl.
//
// The "any" entries go too, and that is the one that matters most: it caches
// "this instance has registered no domains at all", the condition that switches
// on the fallback which honours the request Host verbatim. Left stale, an
// instance keeps honouring forged Hosts for ttl after the operator registers
// the very first domain that was supposed to end it.
func ClearDomainCache(domain string) {
	domain = NormalizeHost(domain)
	if domain == "" {
		return
	}
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	for k := range globalCache {
		if strings.HasSuffix(k, "|any") {
			delete(globalCache, k)
			continue
		}
		idx := strings.LastIndexByte(k, ':')
		if idx != -1 {
			host := k[idx+1:]
			if host == domain || strings.HasSuffix(host, "."+domain) {
				delete(globalCache, k)
			}
		}
	}
}

// baseDomainKey is the cache key for a database's reserved tenant-subdomain
// base. It carries the namespace like every other answer.
func baseDomainKey(db *gorm.DB) string {
	return namespace(db) + "base|"
}

// cachedBaseDomain returns the reserved tenant-subdomain base for db, cached
// per database with the same ttl and invalidation as the ownership answers.
//
// models.BaseDomain reads the settings table on every call, and OwnerOf and
// OwnedHost consult it on every host→org resolution — the hot path the domains
// cache above exists to protect. The base changes rarely (an operator flips it
// once), so caching it keeps resolution at one settings round-trip per ttl per
// database instead of one per request.
func cachedBaseDomain(db *gorm.DB) string {
	key := baseDomainKey(db)
	globalCacheMu.RLock()
	e, hit := globalCache[key]
	globalCacheMu.RUnlock()
	if hit && time.Now().Before(e.expiry) {
		return e.name
	}
	base := models.BaseDomain(db)
	globalCacheMu.Lock()
	if len(globalCache) > 1024 {
		globalCache = make(map[string]entry)
	}
	globalCache[key] = entry{ok: true, name: base, expiry: time.Now().Add(ttl)}
	globalCacheMu.Unlock()
	return base
}

// ClearBaseDomainCache drops every cached answer for db after the reserved
// tenant-subdomain base changed. The base itself and every per-host ownership
// answer were computed under the old base, so the whole namespace goes rather
// than one key. Call it when the base domain setting is written.
func ClearBaseDomainCache(db *gorm.DB) {
	ns := namespace(db)
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	for k := range globalCache {
		if strings.HasPrefix(k, ns) {
			delete(globalCache, k)
		}
	}
}

// sharedHostsKey is the cache key for a database's instance-wide shared hosts.
func sharedHostsKey(db *gorm.DB) string {
	return namespace(db) + "shared_hosts|"
}

// cachedSharedHosts returns the shared hosts for db, cached per database.
func cachedSharedHosts(db *gorm.DB) []string {
	key := sharedHostsKey(db)
	globalCacheMu.RLock()
	e, hit := globalCache[key]
	globalCacheMu.RUnlock()
	if hit && time.Now().Before(e.expiry) {
		return e.list
	}
	hosts := models.SharedHosts(db)
	globalCacheMu.Lock()
	if len(globalCache) > 1024 {
		globalCache = make(map[string]entry)
	}
	globalCache[key] = entry{ok: true, list: hosts, expiry: time.Now().Add(ttl)}
	globalCacheMu.Unlock()
	return hosts
}

// ClearSharedHostsCache drops every cached shared hosts answer for db.
// Call it when the shared_hosts setting is written.
func ClearSharedHostsCache(db *gorm.DB) {
	ns := namespace(db)
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	for k := range globalCache {
		if strings.HasPrefix(k, ns) {
			delete(globalCache, k)
		}
	}
}

// HasSharedHosts reports whether any shared hosts are configured for db.
func HasSharedHosts(db *gorm.DB) bool {
	return len(cachedSharedHosts(db)) > 0
}

// clearAllCache is for tests.
func clearAllCache() {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	globalCache = make(map[string]entry)
}
