// Package origin derives the absolute origin — "https://app.example.com" — that
// outbound URLs must be built from, plus the transport security of the request
// that asked for them.
//
// Every absolute URL octarq emits is built here: password-reset links,
// email-verification links, workspace invite links, OAuth redirect_uri. They
// used to come from a single OCTARQ_BASE_URL; they now come from the request,
// because one instance legitimately answers on several hostnames (a link
// shortener domain, a tenant's custom domain, the dashboard host) and a single
// instance-wide constant cannot describe that.
//
// # Host is a lookup key, never a value
//
// The Host header is set by whoever sends the request. Building a password-reset
// link straight from it is an account takeover (CWE-640): an attacker POSTs
// /api/auth/forgot with "Host: evil.com" and the victim's mailbox receives a
// link carrying a VALID reset token pointed at the attacker's site. One click
// and the token is theirs.
//
// So Host may only ever *select* among origins somebody has already proven they
// own — rows in the domains table, which the dns plugin only writes after the
// operator has attached the zone to a provider account they hold credentials
// for. An unrecognised Host selects nothing. This mirrors, and is the shared
// implementation for, octarq-pro's pkg/baseurl: exact hostname matching against
// registered names and parsed host lists, never a substring scan over the raw
// JSON column, where one org's hostname can appear inside another's.
//
// # The no-whitelist fallback
//
// A fresh self-hosted instance has registered no domains at all, so there is
// nothing to check Host against. Refusing to build any absolute URL there would
// mean password reset never works until a domain is attached — for a build
// composed without the dns plugin, never at all. Such an instance therefore
// falls back to using the request host as-is.
//
// The residual risk of that fallback is real but bounded: an attacker who can
// reach the instance can still aim a reset link at their own host. What contains
// it is that the fallback is switched off the moment ANY domain is registered —
// see Resolver.Absolute. It is never a "try the whitelist, then trust the host
// anyway" path, which would make the whitelist decorative. An operator who wants
// the guarantee turns it on by registering the domain they serve on.
package origin

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// Secure reports whether the request reached octarq over TLS, which is what
// decides the Secure attribute on the session cookie and the scheme of a
// fallback origin.
//
// X-Forwarded-Proto is honoured ONLY when the operator has declared a trusted
// reverse proxy (OCTARQ_TRUST_PROXY). Without that declaration the header is
// client-supplied: anyone could send "X-Forwarded-Proto: https" over plain HTTP
// and have their session cookie marked Secure — which the browser would then
// refuse to send back over that same plain-HTTP connection, locking the user
// out. Callers pass their own package's trustProxy flag; this package
// deliberately keeps no copy of it.
func Secure(r *http.Request, trustProxy bool) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if !trustProxy {
		return false
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	// A chain of proxies appends: "https, http". The left-most entry is the
	// scheme the client actually used.
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// NormalizeHost canonicalises a hostname for comparison: lowercased, trimmed,
// port removed, trailing dot removed. So "Acme.COM:8443" and "acme.com." both
// normalise to "acme.com".
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	// Strip the port. IPv6 literals arrive bracketed ("[::1]:8080"), so cut
	// after the closing bracket rather than at the first colon.
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

// Candidates returns host and each parent domain down to two labels, so
// "mail.app.acme.com" yields "mail.app.acme.com", "app.acme.com", "acme.com".
// Whoever controls a registered zone controls its subdomains, so a dashboard on
// a subdomain of a registered domain counts as owned.
//
// Stopping at two labels over-matches multi-part suffixes like "co.uk": a
// tenant owning "acme.co.uk" matches at the three-label step first, so the extra
// "co.uk" probe only ever costs a wasted comparison and can only match if
// somebody genuinely registered "co.uk" as a domain of their own.
func Candidates(host string) []string {
	if host == "" || strings.HasPrefix(host, "[") {
		return nil
	}
	// A bare IP literal is nobody's registered domain.
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

// domainRow mirrors the dns plugin's domains table. Core cannot import the
// plugin (the dependency runs the other way) and this needs five columns, so a
// local mirror is cheaper than a service round-trip. The host-list columns are
// typed models.HostList so the JSON decoding — including the legacy plain
// []string encoding — stays in the one place that owns it.
type domainRow struct {
	OrgID     uint            `gorm:"column:owner_id"`
	Name      string          `gorm:"column:name"`
	ForLink   bool            `gorm:"column:for_link"`
	ForMail   bool            `gorm:"column:for_mail"`
	LinkHosts models.HostList `gorm:"column:link_hosts"`
	MailHosts models.HostList `gorm:"column:mail_hosts"`
}

func (domainRow) TableName() string { return "domains" }

// OwnedHost reports whether r arrived on a hostname that is registered in the
// domains table, returning it normalised.
//
// orgID scopes the question. Pass a workspace ID to ask "does THIS org own the
// host" — the strict form, for URLs a single tenant's users land on. Pass 0 to
// ask "does anyone on this instance own it", which is the right question for
// instance-wide flows such as password reset, where the recipient's workspace
// is not yet known and every registered host belongs to this instance anyway.
//
// A hostname counts as owned when it, or one of its parent domains, is a
// registered domain name, or when it is an enabled entry in that domain's link
// or mail host list. Matching is always on whole, normalised hostnames.
func OwnedHost(db *gorm.DB, orgID uint, r *http.Request) (string, bool) {
	if db == nil || r == nil {
		return "", false
	}
	host := NormalizeHost(r.Host)
	if host == "" || host == "localhost" || host == "[::1]" {
		return "", false
	}
	candidates := Candidates(host)
	if len(candidates) == 0 { // IP literal or single-label name
		return "", false
	}
	rows, ok := domains(db, orgID)
	if !ok {
		return "", false
	}
	for _, d := range rows {
		if matchRow(d, host, candidates) {
			return host, true
		}
	}
	return "", false
}

// OwnerOf reports which org owns host.
//
// Matching evaluates Candidates from most specific to least specific. When
// multiple distinct orgs match at the same specificity level, OwnerOf returns
// (0, false) rather than guessing: selecting an arbitrary org could serve tenant
// A's portal or catalog to tenant B's buyers.
//
// Unrecognised hostnames, localhost, IP literals, single-label hosts, nil db,
// or builds without the domains table return (0, false). OwnerOf deliberately
// does not fallback to org 1 — single-tenant defaults belong in caller logic,
// not in ownership resolution.
func OwnerOf(db *gorm.DB, host string) (uint, bool) {
	if db == nil {
		return 0, false
	}
	host = NormalizeHost(host)
	if host == "" || host == "localhost" || host == "[::1]" {
		return 0, false
	}
	candidates := Candidates(host)
	if len(candidates) == 0 {
		return 0, false
	}
	rows, ok := domains(db, 0)
	if !ok || len(rows) == 0 {
		return 0, false
	}

	for i, c := range candidates {
		isLevelZero := (i == 0)
		var matchedOrg uint
		var found bool
		for _, d := range rows {
			if matchRowAt(d, host, c, isLevelZero) {
				if !found {
					matchedOrg = d.OrgID
					found = true
				} else if matchedOrg != d.OrgID {
					return 0, false
				}
			}
		}
		if found {
			return matchedOrg, true
		}
	}

	return 0, false
}

func matchRow(d domainRow, host string, candidates []string) bool {
	for i, c := range candidates {
		if matchRowAt(d, host, c, i == 0) {
			return true
		}
	}
	return false
}

func matchRowAt(d domainRow, host string, candidate string, isLevelZero bool) bool {
	if candidate == NormalizeHost(d.Name) {
		return true
	}
	if isLevelZero && (hostListHas(d.LinkHosts, host) || hostListHas(d.MailHosts, host)) {
		return true
	}
	return false
}

// ServesTraffic reports whether host is registered as a hostname that serves
// short links or receives mail. Such a host exists to serve the public, not the
// operator, so the dashboard is withheld from it.
//
// Unlike OwnedHost this does NOT walk parent domains: registering "acme.com"
// for short links must not take the dashboard away from "app.acme.com".
func ServesTraffic(db *gorm.DB, host string) bool {
	if db == nil {
		return false
	}
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	rows, ok := domains(db, 0)
	if !ok {
		return false
	}
	for _, d := range rows {
		// An empty host list with the toggle on means the apex itself serves.
		if d.ForLink {
			if len(d.LinkHosts.Enabled()) == 0 && NormalizeHost(d.Name) == host {
				return true
			}
			if hostListHas(d.LinkHosts, host) {
				return true
			}
		}
		if d.ForMail {
			if len(d.MailHosts.Enabled()) == 0 && NormalizeHost(d.Name) == host {
				return true
			}
			if hostListHas(d.MailHosts, host) {
				return true
			}
		}
	}
	return false
}

// AnyRegistered reports whether this instance has any registered domain at all,
// i.e. whether a whitelist to check Host against exists. See the package comment
// for why that gates the fallback.
func AnyRegistered(db *gorm.DB) bool {
	rows, ok := domains(db, 0)
	return ok && len(rows) > 0
}

// domains loads the domain rows, optionally scoped to one org. ok is false when
// there is no domains table — an OSS build composed without the dns plugin —
// which is indistinguishable from "nothing registered" for every caller here.
func domains(db *gorm.DB, orgID uint) ([]domainRow, bool) {
	if db == nil || !db.Migrator().HasTable("domains") {
		return nil, false
	}
	q := db.Model(&domainRow{})
	if orgID != 0 {
		q = q.Where("owner_id = ?", orgID)
	}
	var rows []domainRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, false
	}
	return rows, true
}

// hostListHas reports whether host is an enabled entry of list.
func hostListHas(list models.HostList, host string) bool {
	for _, h := range list.Enabled() {
		if NormalizeHost(h) == host {
			return true
		}
	}
	return false
}

// unsafeHostChars are characters that cannot appear in a hostname and would let
// a forged Host header break out of the origin it is pasted into — a path, a
// query, userinfo, or a header split in the mail that carries the link.
const unsafeHostChars = " \t\r\n/\\?#@\"'<>%"

// safeHostPort reports whether raw is usable verbatim as the authority part of a
// URL. Go's HTTP server already rejects malformed request lines, but the
// fallback path pastes r.Host into a URL, and a test double or a future
// transport is not bound by that.
func safeHostPort(raw string) bool {
	// 253 is the maximum DNS name length; the rest is room for ":port".
	return raw != "" && len(raw) <= 259 && !strings.ContainsAny(raw, unsafeHostChars)
}

// ttl bounds how long an ownership answer is reused. The endpoints that build
// absolute URLs are unauthenticated and the dashboard gate runs on every page
// load, so without a cache an attacker spraying Host headers is a free DB-query
// amplifier. Negative answers are cached for exactly that reason.
const ttl = 5 * time.Minute

type entry struct {
	ok     bool
	expiry time.Time
}

// Resolver answers origin questions with a short-lived cache in front of the
// domains table. Zero value is not usable — call NewResolver.
type Resolver struct {
	db      *gorm.DB
	mu      sync.RWMutex
	entries map[string]entry
}

func NewResolver(db *gorm.DB) *Resolver {
	return &Resolver{db: db, entries: make(map[string]entry)}
}

func (rv *Resolver) cached(key string, compute func() bool) bool {
	rv.mu.RLock()
	e, hit := rv.entries[key]
	rv.mu.RUnlock()
	if hit && time.Now().Before(e.expiry) {
		return e.ok
	}
	ok := compute()
	rv.mu.Lock()
	defer rv.mu.Unlock()
	// Bound the map: a Host-header sprayer would otherwise grow it without
	// limit. A wholesale reset beats LRU bookkeeping for a cache this cold.
	if len(rv.entries) > 1024 {
		rv.entries = make(map[string]entry)
	}
	rv.entries[key] = entry{ok: ok, expiry: time.Now().Add(ttl)}
	return ok
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
	// The Host is not one this instance owns. If ANY domain is registered there
	// is a whitelist and this host failed it — a forged Host lands here, and
	// honouring it would be the account-takeover in the package comment.
	if rv.AnyRegistered() {
		return ""
	}
	// No whitelist exists at all: nothing to check against. See the package
	// comment for the scope of, and the residual risk in, this fallback.
	if !safeHostPort(r.Host) {
		return ""
	}
	if secure {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
