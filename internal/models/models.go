// Package models defines the GORM persistence schema for led.
//
// Every user-facing entity (Link, Mailbox, Domain) carries a Note field —
// a free-text remark that upstream wr.do does not support. DNS records get
// their note through the provider's native comment field (see dnsprovider).
//
// All tables carry an OwnerID (constant 1 in single-user mode) so the move to
// multi-tenant later needs no schema migration.
package models

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const SingleUserID uint = 1

// StringList is a []string persisted as a JSON text column, portable across
// SQLite and Postgres.
type StringList []string

func (s StringList) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	return string(b), err
}

func (s *StringList) Scan(v any) error {
	if v == nil {
		*s = nil
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return fmt.Errorf("StringList: unsupported scan type %T", v)
	}
	if len(b) == 0 {
		*s = nil
		return nil
	}
	return json.Unmarshal(b, (*[]string)(s))
}

// Host is a single hostname with an enable flag, so a host can be temporarily
// disabled without losing its configuration.
type Host struct {
	Host    string `json:"host"`
	Enabled bool   `json:"enabled"`
}

// HostList is a []Host persisted as a JSON text column. Scan is backward
// compatible with the older format that stored a plain []string (each such
// host is treated as enabled).
type HostList []Host

func (l HostList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal([]Host(l))
	return string(b), err
}

func (l *HostList) Scan(v any) error {
	if v == nil {
		*l = nil
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return fmt.Errorf("HostList: unsupported scan type %T", v)
	}
	if len(b) == 0 {
		*l = nil
		return nil
	}
	var hosts []Host
	if err := json.Unmarshal(b, &hosts); err == nil {
		*l = hosts
		return nil
	}
	// Legacy []string format.
	var strs []string
	if err := json.Unmarshal(b, &strs); err != nil {
		return err
	}
	out := make(HostList, 0, len(strs))
	for _, s := range strs {
		out = append(out, Host{Host: s, Enabled: true})
	}
	*l = out
	return nil
}

// Enabled returns only the hostnames that are currently enabled.
func (l HostList) Enabled() []string {
	out := make([]string, 0, len(l))
	for _, h := range l {
		if h.Enabled {
			out = append(out, h.Host)
		}
	}
	return out
}

// Blocks reports whether host is listed but every listing is disabled — i.e.
// traffic to it should be dropped. An unlisted host is not blocked.
func (l HostList) Blocks(host string) bool {
	listed := false
	for _, h := range l {
		if h.Host == host {
			listed = true
			if h.Enabled {
				return false
			}
		}
	}
	return listed
}

// Token is an API token for the open API. Only the SHA-256 hash of the raw
// token is stored; the raw token is shown once at creation time. Prefix keeps
// a short, non-secret identifier for the dashboard list.
type Token struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	OwnerID    uint       `gorm:"index;default:1" json:"-"`
	Name       string     `gorm:"size:255" json:"name"`
	Hash       string     `gorm:"uniqueIndex;size:64" json:"-"` // SHA-256 hex of the raw token
	Prefix     string     `gorm:"size:32" json:"prefix"`        // e.g. "led_abcd" for identification
	Note       string     `gorm:"type:text" json:"note"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// HashToken returns the SHA-256 hex digest of a raw API token. The stored hash
// is what bearer requests are matched against.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ProviderAccount represents a DNS provider configuration (e.g. Cloudflare)
// containing the credentials needed to manage zones.
type ProviderAccount struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OwnerID   uint      `gorm:"index;default:1" json:"-"`
	Name      string    `gorm:"size:255" json:"name"` // e.g. "Personal Cloudflare"
	Type      string    `gorm:"size:32" json:"type"`  // e.g. "cloudflare", "dnspod"
	Config    string    `gorm:"type:text" json:"-"`   // AES-GCM encrypted credentials JSON
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Domain is a domain managed by led, tied to a DNS provider account.
type Domain struct {
	ID                uint   `gorm:"primaryKey" json:"id"`
	OwnerID           uint   `gorm:"index;default:1" json:"-"`
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
	LinkHosts HostList  `gorm:"type:text" json:"linkHosts"`
	MailHosts HostList  `gorm:"type:text" json:"mailHosts"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// EffectiveLinkHosts returns the enabled hostnames short links are served on.
func (d Domain) EffectiveLinkHosts() []string { return d.LinkHosts.Enabled() }

// EffectiveMailHosts returns the enabled hostnames mailboxes live under.
func (d Domain) EffectiveMailHosts() []string { return d.MailHosts.Enabled() }

// Link is a short link. (Host, Slug) is unique.
type Link struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	OwnerID    uint       `gorm:"index;default:1" json:"-"`
	Host       string     `gorm:"size:255;index:idx_host_slug,unique" json:"host"` // empty = default/any host
	Slug       string     `gorm:"size:255;index:idx_host_slug,unique" json:"slug"`
	Target     string     `gorm:"type:text" json:"target"`
	Password   string     `gorm:"size:255" json:"-"` // optional; presence exposed via HasPassword
	Note       string     `gorm:"type:text" json:"note"`
	Title      string     `gorm:"size:255" json:"title"`
	Tags       string     `gorm:"size:512" json:"tags"` // comma-separated tags
	ExpiresAt  *time.Time `json:"expiresAt"`
	ExpiredURL string     `gorm:"type:text" json:"expiredUrl"` // redirect here once expired / over limit (dub-style)
	ClickLimit int64      `gorm:"default:0" json:"clickLimit"` // 0 = unlimited
	Archived   bool       `gorm:"default:false;index" json:"archived"`
	Enabled    bool       `gorm:"default:true" json:"enabled"`
	Clicks     int64      `gorm:"default:0" json:"clicks"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// LinkEvent is a single click, recorded asynchronously for basic analytics.
type LinkEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	LinkID    uint      `gorm:"index" json:"linkId"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	IP        string    `gorm:"size:64" json:"ip"`
	Country   string    `gorm:"size:64" json:"country"`
	City      string    `gorm:"size:128" json:"city"`
	Device    string    `gorm:"size:32" json:"device"`
	Browser   string    `gorm:"size:64" json:"browser"`
	OS        string    `gorm:"size:64" json:"os"`
	Referer   string    `gorm:"type:text" json:"referer"`
	UA        string    `gorm:"type:text" json:"ua"`
}

// Mailbox is an address that can receive mail (prefix@domain).
type Mailbox struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OwnerID   uint      `gorm:"index;default:1" json:"-"`
	Address   string    `gorm:"uniqueIndex;size:320" json:"address"`
	Note      string    `gorm:"type:text" json:"note"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Unread    int64     `gorm:"-" json:"unread"` // computed, not persisted
}

// Email is a received message stored for a mailbox.
type Email struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	MailboxID   uint      `gorm:"index" json:"mailboxId"`
	MessageID   string    `gorm:"size:512;index" json:"messageId"`
	FromAddr    string    `gorm:"size:320" json:"from"`
	ToAddr      string    `gorm:"size:320" json:"to"`
	Subject     string    `gorm:"type:text" json:"subject"`
	Text        string    `gorm:"type:text" json:"text"`
	HTML        string    `gorm:"type:text" json:"html"`
	Raw         []byte    `json:"-"` // GORM maps []byte to blob (sqlite) / bytea (postgres)
	Read        bool      `gorm:"default:false" json:"read"`
	Note        string    `gorm:"type:text" json:"note"`
	Attachments string    `gorm:"type:text" json:"attachments"` // JSON array of {filename,contentType,size}
	ReceivedAt  time.Time `gorm:"index" json:"receivedAt"`
}

// Setting is a single key/value runtime configuration entry (reserved slugs,
// reserved mailbox prefixes, a global Cloudflare token, …).
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type SMTPSender struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	OwnerID   uint      `json:"-" gorm:"index"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user"`
	Pass      string    `json:"-"`
	FromEmail string    `json:"fromEmail"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AllModels lists every model for AutoMigrate.
func AllModels() []any {
	return []any{
		&ProviderAccount{}, &Domain{}, &Link{}, &LinkEvent{}, &Mailbox{}, &Email{}, &Token{}, &Setting{}, &SMTPSender{},
	}
}
