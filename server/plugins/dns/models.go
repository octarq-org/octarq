package dns

import (
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

// ProviderAccount represents a DNS provider configuration (e.g. Cloudflare)
// containing the credentials needed to manage zones.
type ProviderAccount struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id;index;default:1" json:"-"`
	Name      string    `gorm:"size:255" json:"name"` // e.g. "Personal Cloudflare"
	Type      string    `gorm:"size:32" json:"type"`  // e.g. "cloudflare", "dnspod"
	Config    string    `gorm:"type:text" json:"-"`   // AES-GCM encrypted credentials JSON
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// HasCredentials is computed (not persisted): the encrypted Config never
	// leaves the server, so the dashboard uses this flag to show "credentials are
	// set — leave blank to keep" instead of an empty field that looks unconfigured.
	HasCredentials bool `gorm:"-" json:"hasCredentials"`
}

// Domain is a domain managed by octarq, tied to a DNS provider account.
type Domain struct {
	ID                uint   `gorm:"primaryKey" json:"id"`
	OrgID             uint   `gorm:"column:owner_id;index;default:1" json:"-"`
	Name              string `gorm:"uniqueIndex;size:255" json:"name"`
	ProviderAccountID uint   `gorm:"index" json:"providerAccountId"`
	ZoneID            string `gorm:"size:64" json:"zoneId"`
	Note              string `gorm:"type:text" json:"note"`
	ForMail           bool   `json:"forMail"` // accept inbound email for this domain
	ForLink           bool   `json:"forLink"` // serve short links on this domain
	// LinkHosts are the hostnames short links are served on for this zone — one
	// or more, typically subdomains like "go.example.com", "s.example.com".
	// MailHosts are the hostnames mailboxes live under (e.g. "example.com",
	// "mail.example.com"). An empty list with the matching toggle on falls back
	// to the apex Name.
	LinkHosts models.HostList `gorm:"type:text" json:"linkHosts"`
	MailHosts models.HostList `gorm:"type:text" json:"mailHosts"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// EffectiveLinkHosts returns the enabled hostnames short links are served on.
func (d Domain) EffectiveLinkHosts() []string { return d.LinkHosts.Enabled() }

// EffectiveMailHosts returns the enabled hostnames mailboxes live under.
func (d Domain) EffectiveMailHosts() []string { return d.MailHosts.Enabled() }

// NormalizeHost canonicalises a hostname for comparison against a Domain's
// Name, LinkHosts or MailHosts: lowercased, trimmed, port removed, trailing dot
// removed. So "Go.Example.COM:8443" and "go.example.com." both match
// "go.example.com".
//
// It lives here because both the links and mail plugins compare request
// hostnames against rows of this package's Domain, and each had grown its own
// copy — three implementations of the same four transformations, which is
// exactly how one of them ends up subtly different from the others.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	// IPv6 literals arrive bracketed ("[::1]:8080"), so cut after the closing
	// bracket rather than at the first colon.
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

// DDNSToken stores a hash-authenticated token for dynamic DNS updates.
type DDNSToken struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	OrgID      uint       `gorm:"column:owner_id;index;default:1" json:"-"`
	DomainID   uint       `gorm:"index" json:"domainId"`
	RecordName string     `gorm:"size:255" json:"recordName"`   // FQDN, e.g. home.example.com
	RecordType string     `gorm:"size:8" json:"recordType"`     // A | AAAA
	TokenHash  string     `gorm:"uniqueIndex;size:64" json:"-"` // SHA-256 of secret
	Label      string     `gorm:"size:255" json:"label"`
	LastIP     string     `gorm:"size:64" json:"lastIp"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func (DDNSToken) TableName() string { return "dns_ddns_tokens" }
