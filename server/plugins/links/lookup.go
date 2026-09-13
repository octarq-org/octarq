package links

import (
	"context"
	"time"

	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// clickLimitCacheTTL: clickLimit 生效需 5s 过期，防超发
const clickLimitCacheTTL = 5 * time.Second

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
		ttl := time.Hour
		if link.ClickLimit > 0 {
			ttl = clickLimitCacheTTL
		}
		_ = e.ctx.CacheSet(ctx, cacheKey, &link, ttl)
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
