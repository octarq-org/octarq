// Package models defines the GORM persistence schema for octarq.
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

// SingleUserID is a legacy placeholder org ID from before multi-tenancy,
// retained for backward compatibility. Current handlers use the org ID
// resolved from the authenticated session.
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

// OrgSlugHistory holds retired workspace addresses (slugs) to prevent squatting/takeover.
type OrgSlugHistory struct {
	Slug      string `gorm:"primaryKey;size:64"`
	OrgID     uint   `gorm:"index;not null"`
	RetiredAt time.Time
}

// User is an authenticated human. A user can belong to multiple orgs.
type User struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Email        string `gorm:"uniqueIndex;size:320;not null" json:"email"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	// InviteTokenHash is the SHA-256 hex hash of the raw invite token. The raw
	// 192-bit token is only ever shown/mailed once, never stored — mirroring
	// ResetTokenHash / VerifyTokenHash below — and is indexed for lookup. A DB
	// leak (backup, shared disk, SQLi) therefore exposes no live invite.
	InviteTokenHash string     `gorm:"index;size:64" json:"-"`
	InviteExpiresAt *time.Time `json:"inviteExpiresAt,omitempty"`

	// TOTPSecret is the base32 TOTP shared secret, stored AES-GCM encrypted at
	// rest (via crypto.Cipher). Empty until 2FA enrollment begins.
	TOTPSecret  string `gorm:"size:512" json:"-"`
	TOTPEnabled bool   `gorm:"not null;default:0" json:"-"`
	// LastTOTPCode prevents replay attacks within the 30s window.
	LastTOTPCode string `gorm:"size:32" json:"-"`
	// RecoveryCodes is a JSON array of bcrypt-hashed one-time recovery codes.
	RecoveryCodes string `gorm:"type:text" json:"-"`
	// IsInstanceAdmin marks the single bootstrap operator account (the user
	// created from OCTARQ_ADMIN_*). It is the stable identity that grants
	// instance-level administrative privileges (/api/instance-settings), set
	// deterministically at first admin login. It must NEVER be derived from
	// org_id ordering: on a fresh instance with registration/OAuth enabled an
	// attacker could otherwise register before the operator and inherit org 1.
	IsInstanceAdmin bool `gorm:"not null;default:0" json:"-"`
	// Password reset token hash and expiry
	ResetTokenHash   string     `gorm:"index;size:64" json:"-"`
	ResetTokenExpiry *time.Time `json:"-"`
	// Email verification status and token hash/expiry
	EmailVerified     bool       `gorm:"not null;default:0" json:"emailVerified"`
	VerifyTokenHash   string     `gorm:"index;size:64" json:"-"`
	VerifyTokenExpiry *time.Time `json:"-"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// Session is a stateful login record. The random Token is stored in the
// browser cookie; validity is determined by looking up this row — no epoch
// math needed. Deleting a row instantly revokes that device's access.
type Session struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"not null;index" json:"userId"`
	OrgID      uint      `gorm:"not null" json:"orgId"`
	Token      string    `gorm:"uniqueIndex;size:64;not null" json:"-"` // SHA-256 hash of the cookie token
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:512" json:"userAgent"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (Session) TableName() string {
	return "user_sessions"
}

// UserIdentity binds a User to one external identity, keyed on the triple the
// identity provider actually asserts: (provider, issuer, subject).
//
// Email is deliberately NOT part of the key. Core identity is matched globally
// by email, orgs can be created by anyone, and an org's SSO issuer / client ID
// are filled in by that org — so an attacker who points their own workspace at
// an OIDC server they run can sign any email claim they like. The subject is
// scoped to the issuer, which is the one part of the assertion the attacker
// cannot forge for somebody else's IdP.
//
// The stored Email is what the provider asserted at binding time, kept for the
// account-settings list ("signed in with alice@acme.com at accounts.acme.com").
// Nothing looks a user up by it.
type UserIdentity struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"index;not null" json:"userId"`
	// Provider is the identity mechanism ("oidc"). Issuer must be normalized
	// with plugin.NormalizeIssuer before it is written or queried — an IdP that
	// advertises https://idp.example/ and one that advertises
	// https://idp.example are the same IdP, and storing both spellings silently
	// unbinds every user of whichever one wasn't stored.
	Provider  string    `gorm:"uniqueIndex:idx_user_identity;size:32;not null" json:"provider"`
	Issuer    string    `gorm:"uniqueIndex:idx_user_identity;size:255;not null" json:"issuer"`
	Subject   string    `gorm:"uniqueIndex:idx_user_identity;size:255;not null" json:"subject"`
	Email     string    `gorm:"size:320" json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrgMember links a User to an Org with a role.
type OrgMember struct {
	OrgID  uint   `gorm:"primaryKey;index:idx_org_user,unique" json:"orgId"`
	UserID uint   `gorm:"primaryKey;index:idx_org_user,unique" json:"userId"`
	Role   string `gorm:"size:32;not null;default:'member'" json:"role"` // "owner" | "admin" | "member"
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

// HostList is a []Host persisted as a JSON text column.
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
	if err := json.Unmarshal(b, &hosts); err != nil {
		return err
	}
	*l = hosts
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
	ID     uint   `gorm:"primaryKey" json:"id"`
	OrgID  uint   `gorm:"column:owner_id;index;default:1" json:"-"`
	Name   string `gorm:"size:255" json:"name"`
	Hash   string `gorm:"uniqueIndex;size:64" json:"-"` // SHA-256 hex of the raw token
	Prefix string `gorm:"size:32" json:"prefix"`        // e.g. "oct_abcd" for identification
	Note   string `gorm:"type:text" json:"note"`
	// UserID is the person the token acts as. A token borrows its holder's
	// membership rather than carrying standalone authority, so removing someone
	// from the workspace takes their tokens with them — the role is read live on
	// every request, not frozen at mint time. It also means an API call lands in
	// the audit log attributed to a person instead of to nobody.
	UserID uint `gorm:"index;not null" json:"userId"`
	// Role narrows the token *below* its holder — it never widens it. The
	// effective role is min(the user's role in OrgID, this). That is what lets an
	// owner hand CI a read-only token instead of a copy of their own account.
	// Empty is read as "member", matching what minting defaults to.
	Role       string     `gorm:"size:32" json:"role"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// ExpiresAt bounds the token's validity. NULL = never expires (back-compat for
	// tokens minted before expiry support). Auth paths reject an expired token.
	ExpiresAt *time.Time `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Expired reports whether the token has a set expiry that is in the past.
func (t Token) Expired() bool {
	return t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now())
}

// HashToken returns the SHA-256 hex digest of a raw API token. The stored hash
// is what bearer requests are matched against.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Setting is a single key/value runtime configuration entry (reserved slugs,
// reserved mailbox prefixes, a global Cloudflare token, …).
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

// WorkspaceSetting is a key/value runtime configuration entry scoped to a specific org.
type WorkspaceSetting struct {
	OrgID uint   `gorm:"primaryKey" json:"orgId"`
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type NotificationChannel struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	OrgID     uint      `gorm:"column:owner_id;index" json:"-"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`                    // e.g. "telegram", "webhook"
	Config    string    `json:"config" gorm:"type:text"` // AES-GCM encrypted JSON object at rest; readers MUST decrypt with failure fallback to raw string
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
	OrgID       uint      `gorm:"index;default:1;column:owner_id" json:"-"` // org owning the reported link, if resolved
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

// PluginSetting records whether a Pro plugin is enabled for a given workspace
// (org). Absence of a row means disabled — plugins are opt-in per workspace.
type PluginSetting struct {
	OrgID     uint      `gorm:"primaryKey" json:"orgId"`
	Plugin    string    `gorm:"primaryKey;size:64" json:"plugin"`
	Enabled   bool      `gorm:"default:false" json:"enabled"`
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
		&Org{}, &User{}, &OrgMember{}, &UserIdentity{}, &UserSetting{}, &PluginSetting{},
		&Token{}, &Setting{}, &WorkspaceSetting{}, &NotificationChannel{},
		&AbuseReport{}, &AuditLog{}, &Webhook{}, &Session{}, &OrgSlugHistory{},
	}
}
