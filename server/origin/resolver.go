package origin

import (
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type Resolver struct {
	db *gorm.DB
	ns string
}

func NewResolver(db *gorm.DB) *Resolver {
	return &Resolver{db: db, ns: namespace(db)}
}

func (rv *Resolver) cached(key string, compute func() bool) bool {
	_, ok := rv.cachedOwner(key, func() (uint, bool) { return 0, compute() })
	return ok
}

func (rv *Resolver) cachedOwner(key string, compute func() (uint, bool)) (uint, bool) {
	key = rv.ns + key
	globalCacheMu.RLock()
	e, hit := globalCache[key]
	globalCacheMu.RUnlock()
	if hit && time.Now().Before(e.expiry) {
		return e.org, e.ok
	}
	org, ok := compute()
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	if len(globalCache) > 1024 {
		globalCache = make(map[string]entry)
	}
	globalCache[key] = entry{ok: ok, org: org, expiry: time.Now().Add(ttl)}
	return org, ok
}

func (rv *Resolver) OwnedHost(orgID uint, r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	host := NormalizeHost(r.Host)
	if host == "" {
		return "", false
	}
	key := "own:" + strconv.FormatUint(uint64(orgID), 10) + ":" + host
	if rv.cached(key, func() bool {
		_, ok := OwnedHost(rv.db, orgID, r)
		return ok
	}) {
		return host, true
	}
	return "", false
}

func (rv *Resolver) OwnerOf(host string) (uint, bool) {
	host = NormalizeHost(host)
	if host == "" {
		return 0, false
	}
	return rv.cachedOwner("owner:"+host, func() (uint, bool) { return OwnerOf(rv.db, host) })
}

func (rv *Resolver) ServesTraffic(host string) bool {
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	return rv.cached("serves:"+host, func() bool { return ServesTraffic(rv.db, host) })
}

func (rv *Resolver) AnyRegistered() bool {
	return rv.cached("any", func() bool { return AnyRegistered(rv.db) })
}

func (rv *Resolver) SharedHost(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	host := NormalizeHost(r.Host)
	if host == "" {
		return "", false
	}
	if rv.cached("shared:"+host, func() bool {
		_, ok := SharedHostWithDB(rv.db, r)
		return ok
	}) {
		return host, true
	}
	return "", false
}

func (rv *Resolver) Absolute(orgID uint, r *http.Request, secure bool) string {
	if r == nil {
		return ""
	}
	if host, ok := rv.OwnedHost(orgID, r); ok {
		return "https://" + host
	}
	if host, ok := rv.SharedHost(r); ok {
		return "https://" + host
	}
	if rv.AnyRegistered() || len(cachedSharedHosts(rv.db)) > 0 {
		return ""
	}
	if !isLoopbackHost(r.Host) || !safeHostPort(r.Host) {
		return ""
	}
	if secure {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
