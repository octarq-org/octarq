package links

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

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
	// Fingerprint is a stable, privacy-preserving per-device hash (anonymized IP
	// + user-agent + accept-language). Used to dedup unique devices in analytics.
	Fingerprint string `gorm:"size:64;index" json:"-"`
	IsBot       bool   `gorm:"default:false;index" json:"isBot"`
}
