package models

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/pkg/hostlist"
)

const SingleUserID uint = 1

const (
	OrgKindPersonal = "personal"
	OrgKindTeam     = "team"
)

type Org struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:255;not null" json:"name"`
	Slug         string    `gorm:"uniqueIndex;size:64;not null" json:"slug"`
	Kind         string    `gorm:"size:16;not null;default:'team';index" json:"kind"`
	InboundToken string    `gorm:"size:64" json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type OrgSlugHistory struct {
	Slug      string `gorm:"primaryKey;size:64"`
	OrgID     uint   `gorm:"index;not null"`
	RetiredAt time.Time
}

type User struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Email           string     `gorm:"uniqueIndex;size:320;not null" json:"email"`
	PasswordHash    string     `gorm:"size:255;not null" json:"-"`
	InviteTokenHash string     `gorm:"index;size:64" json:"-"`
	InviteExpiresAt *time.Time `json:"inviteExpiresAt,omitempty"`

	TOTPSecret        string     `gorm:"size:512" json:"-"`
	TOTPEnabled       bool       `gorm:"not null;default:0" json:"-"`
	LastTOTPCode      string     `gorm:"size:32" json:"-"`
	RecoveryCodes     string     `gorm:"type:text" json:"-"`
	IsInstanceAdmin   bool       `gorm:"not null;default:0" json:"-"`
	ResetTokenHash    string     `gorm:"index;size:64" json:"-"`
	ResetTokenExpiry  *time.Time `json:"-"`
	EmailVerified     bool       `gorm:"not null;default:0" json:"emailVerified"`
	VerifyTokenHash   string     `gorm:"index;size:64" json:"-"`
	VerifyTokenExpiry *time.Time `json:"-"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type Session struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"not null;index" json:"userId"`
	OrgID      uint      `gorm:"not null" json:"orgId"`
	Token      string    `gorm:"uniqueIndex;size:64;not null" json:"-"`
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:512" json:"userAgent"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (Session) TableName() string {
	return "user_sessions"
}

type UserIdentity struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"userId"`
	Provider  string    `gorm:"uniqueIndex:idx_user_identity;size:32;not null" json:"provider"`
	Issuer    string    `gorm:"uniqueIndex:idx_user_identity;size:255;not null" json:"issuer"`
	Subject   string    `gorm:"uniqueIndex:idx_user_identity;size:255;not null" json:"subject"`
	Email     string    `gorm:"size:320" json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type OrgMember struct {
	OrgID     uint      `gorm:"primaryKey;index:idx_org_user,unique" json:"orgId"`
	UserID    uint      `gorm:"primaryKey;index:idx_org_user,unique" json:"userId"`
	Role      string    `gorm:"size:32;not null;default:'member'" json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

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

type Host = hostlist.Host

type HostList = hostlist.HostList

type Token struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	OrgID  uint   `gorm:"column:owner_id;index;default:1" json:"-"`
	Name   string `gorm:"size:255" json:"name"`
	Hash   string `gorm:"uniqueIndex;size:64" json:"-"`
	Prefix string `gorm:"size:32" json:"prefix"`
	Note   string `gorm:"type:text" json:"note"`
	UserID uint   `gorm:"index;not null" json:"userId"`
	Role   string `gorm:"size:32" json:"role"`

	IsAgent bool `gorm:"default:false;index" json:"isAgent"`

	AllowedTools string `gorm:"type:text" json:"allowedTools"`

	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func ParseAllowedTools(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			var out []string
			for _, item := range list {
				trimmed := strings.TrimSpace(item)
				if trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return out
		}
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (t Token) AllowsTool(toolName string) bool {
	if !t.IsAgent {
		return true
	}
	if toolName == "" {
		return false
	}
	tools := ParseAllowedTools(t.AllowedTools)
	for _, allowed := range tools {
		if allowed == toolName {
			return true
		}
	}
	return false
}

func (t Token) Expired() bool {
	return t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now())
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type WorkspaceSetting struct {
	OrgID uint   `gorm:"primaryKey" json:"orgId"`
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type NotificationChannel struct {
	ID           uint            `json:"id" gorm:"primaryKey"`
	OrgID        uint            `gorm:"column:owner_id;index" json:"-"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	VaultItemID  uint            `json:"vaultItemId,omitempty" gorm:"column:vault_item_id;default:0"`
	Config       string          `json:"config" gorm:"type:text"`
	ConfigSchema json.RawMessage `json:"configSchema,omitempty" gorm:"-"`
	Enabled      bool            `json:"enabled" gorm:"default:true"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type AuditLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	OrgID      uint      `gorm:"index" json:"orgId"`
	ActorID    uint      `gorm:"index" json:"actorId"`
	Action     string    `gorm:"size:64;index" json:"action"`
	TargetType string    `gorm:"size:32" json:"targetType"`
	TargetID   uint      `json:"targetId"`
	Meta       string    `gorm:"type:text" json:"meta"`
	IP         string    `gorm:"size:64" json:"ip"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

type UserSetting struct {
	UserID    uint      `gorm:"primaryKey" json:"userId"`
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type PluginSetting struct {
	OrgID     uint      `gorm:"primaryKey" json:"orgId"`
	Plugin    string    `gorm:"primaryKey;size:64" json:"plugin"`
	Enabled   bool      `gorm:"default:false" json:"enabled"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Webhook struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	OrgID       uint      `gorm:"index;default:1;column:owner_id" json:"-"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	URL         string    `gorm:"size:1024;not null;column:url" json:"url"`
	VaultItemID uint      `gorm:"column:vault_item_id;default:0" json:"-"`
	Events      string    `gorm:"size:1024;not null;default:'*'" json:"events"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Notification struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    string     `gorm:"size:64;index;not null" json:"userId"`
	OrgID     string     `gorm:"size:64;index;not null" json:"orgId"`
	EventType string     `gorm:"size:128;not null" json:"eventType"`
	Title     string     `gorm:"size:255;not null" json:"title"`
	Body      string     `gorm:"type:text;not null" json:"body"`
	Data      string     `gorm:"type:text" json:"data,omitempty"`
	Priority  string     `gorm:"size:32;default:'normal'" json:"priority"`
	ReadAt    *time.Time `gorm:"index" json:"readAt,omitempty"`
	CreatedAt time.Time  `gorm:"index" json:"createdAt"`
}

func (Notification) TableName() string {
	return "notifications"
}

type NotificationPreference struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       string     `gorm:"size:64;index:idx_notif_pref_user_pattern,unique;not null" json:"userId"`
	EventPattern string     `gorm:"size:128;index:idx_notif_pref_user_pattern,unique;not null" json:"eventPattern"`
	Channels     StringList `gorm:"type:text;not null" json:"channels"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (NotificationPreference) TableName() string {
	return "notification_preferences"
}

type SchemaMigration struct {
	Version   int64     `gorm:"primaryKey;autoIncrement:false" json:"version"`
	AppliedAt time.Time `json:"appliedAt"`
}

func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

func AllModels() []any {
	return []any{
		&Org{}, &User{}, &OrgMember{}, &UserIdentity{}, &UserSetting{}, &PluginSetting{},
		&Token{}, &Setting{}, &WorkspaceSetting{}, &NotificationChannel{},
		&AuditLog{}, &Webhook{}, &Session{}, &OrgSlugHistory{},
		&Notification{}, &NotificationPreference{}, &File{}, &OrgHost{},
		&SchemaMigration{},
	}
}
