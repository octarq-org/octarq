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
// implementation for, downstream baseurl resolution: exact hostname matching against
// registered names and parsed host lists, never a substring scan over the raw
// JSON column, where one org's hostname can appear inside another's.
//
// Alternatively, the operator can explicitly declare instance-wide shared hosts
// via the OCTARQ_SHARED_HOSTS environment variable. This is safe because it is
// an explicit deployment declaration, not inferred from the request.
//
// # The local development loopback fallback
//
// A fresh self-hosted instance running locally has registered no domains at all,
// so there is nothing to check Host against. To support local development while
// strictly preventing Host header poisoning (CWE-640 / AUD-P2-03), the fallback path
// permits ONLY verified loopback hosts (localhost, 127.0.0.1, [::1], *.localhost).
//
// Any untrusted or external host header is rejected (returning "") unless explicitly
// declared by the operator via OCTARQ_SHARED_HOSTS or registered in the domains table.
package origin

import (
	"net"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
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
//
// zoneCandidates additionally cuts the list at the reserved tenant-subdomain
// base: a host that lives under the base must never resolve through anything
// above it — an unprovisioned tenant label would otherwise bubble up to the
// operator's registered apex and land in the vendor org, which is how one
// tenant's unassigned address turns into another tenant's data exposure.
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

// zoneCandidates returns the candidate list for host, truncated so that nothing
// above base is considered when host lives under it. base may be "" (no
// reserved zone), in which case the result is exactly Candidates(host).
func zoneCandidates(host, base string) []string {
	cands := Candidates(host)
	if base == "" || !underBase(host, base) {
		return cands
	}
	for i, c := range cands {
		if c == base {
			return cands[:i+1]
		}
	}
	return cands
}

// underBase reports whether host is the reserved tenant-subdomain base itself
// or lives beneath it (host == base or "x.base" for any non-empty x). base
// must already be normalized; an empty base never reserves anything.
func underBase(host, base string) bool {
	return base != "" && (host == base || strings.HasSuffix(host, "."+base))
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
// plugin (the dependency runs the other way) and this needs six columns, so a
// local mirror is cheaper than a service round-trip. The host-list columns are
// typed models.HostList so the JSON decoding stays in the one place that owns it.
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
	base := cachedBaseDomain(db)
	candidates := zoneCandidates(host, base)
	if len(candidates) == 0 { // IP literal or single-label name
		return "", false
	}
	rows, ok := domains(db, orgID)
	if !ok {
		return "", false
	}
	for _, d := range rows {
		if matchRow(d, host, candidates, base) {
			return host, true
		}
	}
	return "", false
}

// SharedHost reports whether r arrived on a hostname that is explicitly declared
// as an instance-wide shared host in the settings table or OCTARQ_SHARED_HOSTS environment variable.
// It returns the normalised hostname if it is a declared shared host.
func SharedHost(r *http.Request) (string, bool) {
	return SharedHostWithDB(nil, r)
}

// SharedHostWithDB reports whether r arrived on a hostname that is declared
// as an instance-wide shared host in db settings or the environment.
func SharedHostWithDB(db *gorm.DB, r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	host := NormalizeHost(r.Host)
	if host == "" {
		return "", false
	}
	for _, h := range cachedSharedHosts(db) {
		if host == NormalizeHost(h) {
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
	base := cachedBaseDomain(db)
	candidates := zoneCandidates(host, base)
	if len(candidates) == 0 {
		return 0, false
	}
	rows, ok := domains(db, 0)
	if !ok || len(rows) == 0 {
		return 0, false
	}

	for i, c := range candidates {
		// Within the reserved tenant-subdomain zone only an exact label counts:
		// a deeper, unprovisioned host must not resolve through the base row
		// (the operator's dashboard) nor through the apex above it. Provisioned
		// tenants always match at level zero, so nothing legitimate is lost.
		if underBase(host, base) && i > 0 {
			continue
		}
		isLevelZero := (i == 0)
		var matchedOrg uint
		var found bool
		for _, d := range rows {
			if matchRowAt(d, host, c, isLevelZero, base) {
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

func matchRow(d domainRow, host string, candidates []string, base string) bool {
	for i, c := range candidates {
		// See OwnerOf: under the reserved zone only exact (level zero) matches
		// are meaningful; within it there are no legitimate intermediate rows.
		if underBase(host, base) && i > 0 {
			break
		}
		if matchRowAt(d, host, c, i == 0, base) {
			return true
		}
	}
	return false
}

func matchRowAt(d domainRow, host string, candidate string, isLevelZero bool, base string) bool {
	if candidate == NormalizeHost(d.Name) {
		return true
	}
	// Inside the reserved tenant-subdomain zone, host lists never participate —
	// not even at level zero, which is exactly the level this matching lets
	// through. Another org's link/mail host list must not be able to claim a
	// label under the base that belongs to no org yet (or to a retired slug);
	// only the row whose Name IS the host (a provisioned address) resolves there.
	if underBase(host, base) {
		return false
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

// isLoopbackHost reports whether raw represents a loopback host (localhost, 127.0.0.1, [::1], etc.).
func isLoopbackHost(raw string) bool {
	host := raw
	if h, port, err := net.SplitHostPort(raw); err == nil {
		for _, c := range port {
			if c < '0' || c > '9' {
				return false
			}
		}
		host = h
	}
	host = strings.Trim(host, "[]")
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
