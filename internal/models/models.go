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

// Domain is a domain managed by led, tied to a DNS provider account.
type Domain struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	OwnerID  uint   `gorm:"index;default:1" json:"-"`
	Name     string `gorm:"uniqueIndex;size:255" json:"name"`
	Provider string `gorm:"size:32" json:"provider"` // cloudflare, ...
	ZoneID   string `gorm:"size:64" json:"zoneId"`
	Note     string `gorm:"type:text" json:"note"`
	Config   string `gorm:"type:text" json:"-"` // AES-GCM encrypted provider credentials JSON
	ForMail  bool   `json:"forMail"`            // accept inbound email for this domain
	ForLink  bool   `json:"forLink"`            // serve short links on this domain
	// LinkHosts are the hostnames short links are served on for this zone — one
	// or more, typically subdomains like "go.example.com", "s.example.com".
	// MailHosts are the hostnames mailboxes live under (e.g. "example.com",
	// "mail.example.com"). An empty list with the matching toggle on falls back
	// to the apex Name.
	LinkHosts StringList `gorm:"type:text" json:"linkHosts"`
	MailHosts StringList `gorm:"type:text" json:"mailHosts"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// EffectiveLinkHosts returns the hostnames links are served on, defaulting to
// the apex Name when links are enabled but no explicit host is set.
func (d Domain) EffectiveLinkHosts() []string {
	if len(d.LinkHosts) > 0 {
		return d.LinkHosts
	}
	if d.ForLink {
		return []string{d.Name}
	}
	return nil
}

// EffectiveMailHosts returns the hostnames mailboxes live under, defaulting to
// the apex Name when mail is enabled but no explicit host is set.
func (d Domain) EffectiveMailHosts() []string {
	if len(d.MailHosts) > 0 {
		return d.MailHosts
	}
	if d.ForMail {
		return []string{d.Name}
	}
	return nil
}

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
	Raw         []byte    `gorm:"type:blob" json:"-"`
	Read        bool      `gorm:"default:false" json:"read"`
	Note        string    `gorm:"type:text" json:"note"`
	Attachments string    `gorm:"type:text" json:"attachments"` // JSON array of {filename,contentType,size}
	ReceivedAt  time.Time `gorm:"index" json:"receivedAt"`
}

// AllModels lists every model for AutoMigrate.
func AllModels() []any {
	return []any{
		&Domain{}, &Link{}, &LinkEvent{}, &Mailbox{}, &Email{}, &Token{},
	}
}
