package origin

import (
	"net"
	"net/http"
	"strings"

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

type hostRow struct {
	OrgID   uint   `gorm:"column:owner_id"`
	Host    string `gorm:"column:host"`
	Traffic bool   `gorm:"column:traffic"`
}

func (hostRow) TableName() string { return "org_hosts" }

func hostRows(db *gorm.DB) ([]hostRow, bool) {
	if db == nil || !db.Migrator().HasTable("org_hosts") {
		return nil, false
	}
	var rows []hostRow
	if err := db.Model(&hostRow{}).Find(&rows).Error; err != nil {
		return nil, false
	}
	return rows, true
}

func hostRowsForOrg(db *gorm.DB, orgID uint) ([]hostRow, bool) {
	if db == nil || !db.Migrator().HasTable("org_hosts") {
		return nil, false
	}
	q := db.Model(&hostRow{})
	if orgID != 0 {
		q = q.Where("owner_id = ?", orgID)
	}
	var rows []hostRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, false
	}
	return rows, true
}

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
	rows, ok := hostRowsForOrg(db, orgID)
	if !ok {
		return "", false
	}
	bound := make(map[string]bool, len(rows))
	for _, row := range rows {
		bound[NormalizeHost(row.Host)] = true
	}
	for i, c := range candidates {
		if underBase(host, base) && i > 0 {
			break
		}
		if bound[c] {
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
	rows, ok := hostRows(db)
	if !ok || len(rows) == 0 {
		return 0, false
	}

	byHost := make(map[string]uint, len(rows))
	ambiguous := make(map[string]bool)
	for _, row := range rows {
		if row.OrgID == 0 {
			continue
		}
		h := NormalizeHost(row.Host)
		if prev, ok := byHost[h]; ok {
			if prev != row.OrgID {
				ambiguous[h] = true
			}
			continue
		}
		byHost[h] = row.OrgID
	}

	for i, c := range candidates {
		if underBase(host, base) && i > 0 {
			continue
		}
		if ambiguous[c] {
			return 0, false
		}
		if orgID, ok := byHost[c]; ok {
			return orgID, true
		}
	}

	return 0, false
}

func ServesTraffic(db *gorm.DB, host string) bool {
	if db == nil {
		return false
	}
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	rows, ok := hostRows(db)
	if !ok {
		return false
	}
	for _, row := range rows {
		if row.Traffic && NormalizeHost(row.Host) == host {
			return true
		}
	}
	return false
}

func AnyRegistered(db *gorm.DB) bool {
	rows, ok := hostRows(db)
	return ok && len(rows) > 0
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
