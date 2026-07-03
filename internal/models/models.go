// Package models defines the GORM persistence schema for led.
//
// Every user-facing entity (Link, Mailbox, Domain) carries a Note field —
// a free-text remark that upstream wr.do does not support. DNS records get
// their note through the provider's native comment field (see dnsprovider).
//
// Multi-tenant: all data tables carry OrgID (DB column: owner_id, kept for
// backward-compat). Each Org is a tenant; Users belong to Orgs via OrgMember.
package models

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// SingleUserID is kept as a transition constant; handlers will replace it
// with the org ID extracted from the authenticated session.
const SingleUserID uint = 1

// Org is a tenant — every data row is scoped to exactly one Org.
type Org struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:255;not null" json:"name"`
	Slug string `gorm:"uniqueIndex;size:64;not null" json:"slug"`
	// InboundToken is this org's per-tenant secret for the inbound-email webhook.
	// It travels in the webhook URL (?token=) instead of a header to keep the
	// Cloudflare worker config to a single value; it is never exposed publicly.
	InboundToken string    `gorm:"size:64" json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// User is an authenticated human. A user can belong to multiple orgs.
type User struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Email           string     `gorm:"uniqueIndex;size:320;not null" json:"email"`
	PasswordHash    string     `gorm:"size:255;not null" json:"-"`
	InviteToken     string     `gorm:"size:255" json:"-"`
	InviteExpiresAt *time.Time `json:"inviteExpiresAt,omitempty"`
	// SessionEpoch is bumped to invalidate every outstanding signed-cookie
	// session for this user ("log out everywhere"). A cookie carries the epoch
	// it was minted under; Require rejects any cookie whose epoch is stale.
	SessionEpoch uint `gorm:"not null;default:0" json:"-"`
	// TOTPSecret is the base32 TOTP shared secret, stored AES-GCM encrypted at
	// rest (via crypto.Cipher). Empty until 2FA enrollment begins.
	TOTPSecret  string `gorm:"size:512" json:"-"`
	TOTPEnabled bool   `gorm:"not null;default:0" json:"-"`
	// RecoveryCodes is a JSON array of bcrypt-hashed one-time recovery codes.
	RecoveryCodes string    `gorm:"type:text" json:"-"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Session is a stateful login record. The random Token is stored in the
// browser cookie; validity is determined by looking up this row — no epoch
// math needed. Deleting a row instantly revokes that device's access.
type Session struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"not null;index" json:"userId"`
	OrgID      uint      `gorm:"not null" json:"orgId"`
	Token      string    `gorm:"uniqueIndex;size:64;not null" json:"-"` // random hex, stored in cookie
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:512" json:"userAgent"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

// OrgMember links a User to an Org with a role.
type OrgMember struct {
	OrgID  uint   `gorm:"primaryKey;index:idx_org_user,unique" json:"orgId"`
	UserID uint   `gorm:"primaryKey;index:idx_org_user,unique" json:"userId"`
	Role   string `gorm:"size:32;not null;default:'member'" json:"role"` // "owner" | "admin" | "member"
}

type RoutingRule struct {
	Type   string `json:"type"`   // "geo", "device", "os", "language"
	Match  string `json:"match"`  // e.g. "US", "Mobile", "iOS", "zh-CN"
	Target string `json:"target"` // redirect URL if matched
}

type RoutingRules []RoutingRule

func (r RoutingRules) Value() (driver.Value, error) {
	if len(r) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal([]RoutingRule(r))
	return string(b), err
}

func (r *RoutingRules) Scan(v any) error {
	if v == nil {
		*r = nil
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return fmt.Errorf("RoutingRules: unsupported scan type %T", v)
	}
	if len(b) == 0 {
		*r = nil
		return nil
	}
	return json.Unmarshal(b, (*[]RoutingRule)(r))
}

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
	OrgID      uint       `gorm:"column:owner_id;index;default:1" json:"-"`
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
	OrgID     uint      `gorm:"column:owner_id;index;default:1" json:"-"`
	Name      string    `gorm:"size:255" json:"name"` // e.g. "Personal Cloudflare"
	Type      string    `gorm:"size:32" json:"type"`  // e.g. "cloudflare", "dnspod"
	Config    string    `gorm:"type:text" json:"-"`   // AES-GCM encrypted credentials JSON
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Domain is a domain managed by led, tied to a DNS provider account.
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
	ID           uint         `gorm:"primaryKey" json:"id"`
	OrgID        uint         `gorm:"column:owner_id;index;default:1" json:"-"`
	Host         string       `gorm:"size:255;index:idx_host_slug,unique" json:"host"` // empty = default/any host
	Slug         string       `gorm:"size:255;index:idx_host_slug,unique" json:"slug"`
	Target       string       `gorm:"type:text" json:"target"`
	Password     string       `gorm:"size:255" json:"-"` // optional; presence exposed via HasPassword
	Note         string       `gorm:"type:text" json:"note"`
	Title        string       `gorm:"size:255" json:"title"`
	Tags         string       `gorm:"size:512" json:"tags"` // comma-separated tags
	ExpiresAt    *time.Time   `json:"expiresAt"`
	ExpiredURL   string       `gorm:"type:text" json:"expiredUrl"` // redirect here once expired / over limit (dub-style)
	ClickLimit   int64        `gorm:"default:0" json:"clickLimit"` // 0 = unlimited
	Archived     bool         `gorm:"default:false;index" json:"archived"`
	Enabled      bool         `gorm:"default:true" json:"enabled"`
	RoutingRules RoutingRules `gorm:"type:text" json:"routingRules"`
	Clicks       int64        `gorm:"default:0" json:"clicks"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// LinkEvent is a single click, recorded asynchronously for basic analytics.
type LinkEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	LinkID    uint      `gorm:"index" json:"linkId"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	IP        string    `gorm:"size:64" json:"ip"`
	Country   string    `gorm:"size:64" json:"country"`
	Region    string    `gorm:"size:128" json:"region"`
	City      string    `gorm:"size:128" json:"city"`
	Device    string    `gorm:"size:32" json:"device"`
	Browser   string    `gorm:"size:64" json:"browser"`
	OS        string    `gorm:"size:64" json:"os"`
	Referer   string    `gorm:"type:text" json:"referer"`
	UA        string    `gorm:"type:text" json:"ua"`
	IsBot     bool      `gorm:"default:false;index" json:"isBot"`
}

// Mailbox is an address that can receive mail (prefix@domain).
type Mailbox struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id;index;default:1" json:"-"`
	Address   string    `gorm:"uniqueIndex;size:320" json:"address"`
	Note      string    `gorm:"type:text" json:"note"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Unread    int64     `gorm:"-" json:"unread"` // computed, not persisted
}

// Email is a received message stored for a mailbox.
type Email struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	MailboxID   uint   `gorm:"index" json:"mailboxId"`
	MessageID   string `gorm:"size:512;index" json:"messageId"`
	FromAddr    string `gorm:"size:320" json:"from"`
	ToAddr      string `gorm:"size:320" json:"to"`
	Subject     string `gorm:"type:text" json:"subject"`
	Text        string `gorm:"type:text" json:"text"`
	HTML        string `gorm:"type:text" json:"html"`
	Raw         []byte `json:"-"` // GORM maps []byte to blob (sqlite) / bytea (postgres)
	Read        bool   `gorm:"default:false" json:"read"`
	Note        string `gorm:"type:text" json:"note"`
	Attachments string `gorm:"type:text" json:"attachments"` // JSON array of {filename,contentType,size}
	// Email authentication results from the receiving MTA (RFC 8601).
	AuthSPF    string    `gorm:"size:16" json:"authSpf"`   // pass|fail|softfail|neutral|none
	AuthDKIM   string    `gorm:"size:16" json:"authDkim"`  // pass|fail|none
	AuthDMARC  string    `gorm:"size:16" json:"authDmarc"` // pass|fail|none
	ReceivedAt time.Time `gorm:"index" json:"receivedAt"`
}

// Setting is a single key/value runtime configuration entry (reserved slugs,
// reserved mailbox prefixes, a global Cloudflare token, …).
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type SMTPSender struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	OrgID     uint      `gorm:"column:owner_id;index" json:"-"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user"`
	Pass      string    `json:"-"`
	FromEmail string    `json:"fromEmail"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type NotificationChannel struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	OrgID     uint      `gorm:"column:owner_id;index" json:"-"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`                    // e.g. "telegram", "webhook"
	Config    string    `json:"config" gorm:"type:text"` // JSON object
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AuditLog records admin actions for traceability.
// Design references: Outline (name/actorId/data), Gitea (op_type/content), Authentik (action/context).
//
// action format: "<resource>.<verb>" — e.g. "link.create", "settings.update", "domain.delete"
// meta is free-form JSON containing relevant context (IDs, before/after values, etc.)
type AuditLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	OrgID      uint      `gorm:"index" json:"orgId"`
	ActorID    uint      `gorm:"index" json:"actorId"` // 0 = API token / system
	Action     string    `gorm:"size:64;index" json:"action"`
	TargetType string    `gorm:"size:32" json:"targetType"` // "link", "domain", "mailbox", etc.
	TargetID   uint      `json:"targetId"`
	Meta       string    `gorm:"type:text" json:"meta"` // JSON detail
	IP         string    `gorm:"size:64" json:"ip"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

// AbuseReport is a user-submitted report of a short link being used for spam,
// phishing, or other policy violations.
type AbuseReport struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Slug        string    `gorm:"size:255;index" json:"slug"`
	Target      string    `gorm:"type:text" json:"target"` // resolved at report time
	Reason      string    `gorm:"size:64" json:"reason"`   // "spam", "phishing", "malware", "other"
	Description string    `gorm:"type:text" json:"description"`
	ReporterIP  string    `gorm:"size:64" json:"reporterIp"`
	Status      string    `gorm:"size:32;default:'open'" json:"status"` // "open", "reviewed", "dismissed"
	CreatedAt   time.Time `json:"createdAt"`
}

// UserSetting stores user-scoped custom preferences (e.g. menu groupings).
type UserSetting struct {
	UserID    uint      `gorm:"primaryKey" json:"userId"`
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Webhook represents a registered HTTP webhook endpoint for event streaming.
type Webhook struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"index;default:1;column:owner_id" json:"-"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	URL       string    `gorm:"size:1024;not null;column:url" json:"url"`
	Secret    string    `gorm:"size:255;not null" json:"secret"`
	Events    string    `gorm:"size:1024;not null;default:'*'" json:"events"` // comma-separated subscribed event codes, e.g. "link.click,email.receive"
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AllModels lists every model for AutoMigrate.
func AllModels() []any {
	return []any{
		&Org{}, &User{}, &OrgMember{}, &UserSetting{},
		&ProviderAccount{}, &Domain{}, &Link{}, &LinkEvent{}, &Mailbox{}, &Email{},
		&Token{}, &Setting{}, &SMTPSender{}, &NotificationChannel{},
		&AbuseReport{}, &AuditLog{}, &Webhook{}, &Session{},
	}
}
