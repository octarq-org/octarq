package origin

import (
	"net"
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

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
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

func Candidates(host string) []string {
	if host == "" || strings.HasPrefix(host, "[") {
		return nil
	}
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

func underBase(host, base string) bool {
	return base != "" && (host == base || strings.HasSuffix(host, "."+base))
}

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

type domainRow struct {
	OrgID     uint            `gorm:"column:owner_id"`
	Name      string          `gorm:"column:name"`
	ForLink   bool            `gorm:"column:for_link"`
	ForMail   bool            `gorm:"column:for_mail"`
	LinkHosts models.HostList `gorm:"column:link_hosts"`
	MailHosts models.HostList `gorm:"column:mail_hosts"`
}

func (domainRow) TableName() string { return "domains" }

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
	if len(candidates) == 0 {
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

func SharedHost(r *http.Request) (string, bool) {
	return SharedHostWithDB(nil, r)
}

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
	if underBase(host, base) {
		return false
	}
	if isLevelZero && (hostListHas(d.LinkHosts, host) || hostListHas(d.MailHosts, host)) {
		return true
	}
	return false
}

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

func AnyRegistered(db *gorm.DB) bool {
	rows, ok := domains(db, 0)
	return ok && len(rows) > 0
}

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

func hostListHas(list models.HostList, host string) bool {
	for _, h := range list.Enabled() {
		if NormalizeHost(h) == host {
			return true
		}
	}
	return false
}

const unsafeHostChars = " \t\r\n/\\?#@\"'<>%"

func safeHostPort(raw string) bool {
	return raw != "" && len(raw) <= 259 && !strings.ContainsAny(raw, unsafeHostChars)
}

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
