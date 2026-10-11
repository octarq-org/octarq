package origin

import (
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// Resolver answers origin questions with a short-lived cache in front of the
// domains table. Zero value is not usable — call NewResolver.
type Resolver struct {
	db *gorm.DB
	ns string // cache-key namespace; see namespace()
}

// NewResolver constructs an Origin Resolver tied to db.
func NewResolver(db *gorm.DB) *Resolver {
	return &Resolver{db: db, ns: namespace(db)}
}

func (rv *Resolver) cached(key string, compute func() bool) bool {
	_, ok := rv.cachedOwner(key, func() (uint, bool) { return 0, compute() })
	return ok
}

// cachedOwner is cached() for answers that carry an org id. Both share one map,
// so the size bound covers every question the resolver answers rather than one
// per method.
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
	// Bound the map: a Host-header sprayer would otherwise grow it without
	// limit. A wholesale reset beats LRU bookkeeping for a cache this cold.
	if len(globalCache) > 1024 {
		globalCache = make(map[string]entry)
	}
	globalCache[key] = entry{ok: ok, org: org, expiry: time.Now().Add(ttl)}
	return org, ok
}

// OwnedHost is the cached form of the package-level OwnedHost.
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

// OwnerOf is the cached form of the package-level OwnerOf.
//
// This is the form public, unauthenticated endpoints should use — a storefront
// or a buyer portal asks this question on every request, including requests
// from someone spraying hostnames, and each miss is a full scan of the domains
// table. Negative answers are cached for that reason.
//
// The cost is that a newly registered domain takes up to ttl to start
// resolving. That is the same staleness OwnedHost and ServesTraffic already
// accept, and registering a domain is not an operation whose effect anyone
// expects within the same second.
func (rv *Resolver) OwnerOf(host string) (uint, bool) {
	host = NormalizeHost(host)
	if host == "" {
		return 0, false
	}
	return rv.cachedOwner("owner:"+host, func() (uint, bool) { return OwnerOf(rv.db, host) })
}

// ServesTraffic is the cached form of the package-level ServesTraffic.
func (rv *Resolver) ServesTraffic(host string) bool {
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	return rv.cached("serves:"+host, func() bool { return ServesTraffic(rv.db, host) })
}

// AnyRegistered is the cached form of the package-level AnyRegistered.
func (rv *Resolver) AnyRegistered() bool {
	return rv.cached("any", func() bool { return AnyRegistered(rv.db) })
}

// SharedHost is the cached form of the package-level SharedHost.
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

// Absolute returns the origin that absolute URLs in this request's response —
// or in mail it sends — must be built from, or "" when no origin can be
// trusted. Callers that get "" emit a relative path: unopenable from a mail
// client, but it can never be an attacker's.
//
// secure comes from Secure() at the call site, so each package applies its own
// TrustProxy setting.
func (rv *Resolver) Absolute(orgID uint, r *http.Request, secure bool) string {
	if r == nil {
		return ""
	}
	if host, ok := rv.OwnedHost(orgID, r); ok {
		// A hostname registered in the domains table is reachable from the
		// public internet, where plaintext is not an option. Local development
		// registers no domain and takes the fallback below.
		return "https://" + host
	}
	if host, ok := rv.SharedHost(r); ok {
		// A shared host is explicitly declared by the operator in the instance
		// configuration (settings table or OCTARQ_SHARED_HOSTS), meaning it is
		// safe to use. It is not inferred from the request, so a forged Host cannot match here.
		return "https://" + host
	}
	// The Host is not one this instance owns, nor is it a shared host. If ANY
	// domain is registered or shared hosts are configured, there is a whitelist
	// and this host failed it — a forged Host lands here, and honouring it would
	// be the account-takeover in the package comment.
	if rv.AnyRegistered() || len(cachedSharedHosts(rv.db)) > 0 {
		return ""
	}
	// No whitelist exists at all: nothing to check against.
	// To prevent Host header poisoning (CWE-640 / AUD-P2-03), never trust
	// an arbitrary request Host. Only loopback hosts (localhost, 127.0.0.1, ::1)
	// are permitted in this unconfigured fallback mode for local development.
	if !isLoopbackHost(r.Host) || !safeHostPort(r.Host) {
		return ""
	}
	if secure {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
