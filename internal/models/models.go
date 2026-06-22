// Package models defines the GORM persistence schema for led.
//
// Every user-facing entity (Link, Mailbox, Domain) carries a Note field —
// a free-text remark that upstream wr.do does not support. DNS records get
// their note through the provider's native comment field (see dnsprovider).
//
// All tables carry an OwnerID (constant 1 in single-user mode) so the move to
// multi-tenant later needs no schema migration.
package models

import "time"

const SingleUserID uint = 1

// Domain is a domain managed by led, tied to a DNS provider account.
type Domain struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	OwnerID   uint   `gorm:"index;default:1" json:"-"`
	Name      string `gorm:"uniqueIndex;size:255" json:"name"`
	Provider  string `gorm:"size:32" json:"provider"` // cloudflare, ...
	ZoneID    string `gorm:"size:64" json:"zoneId"`
	Note      string `gorm:"type:text" json:"note"`
	Config    string `gorm:"type:text" json:"-"` // AES-GCM encrypted provider credentials JSON
	ForMail   bool   `json:"forMail"`            // accept inbound email for this domain
	ForLink   bool   `json:"forLink"`            // serve short links on this domain
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Link is a short link. (Host, Slug) is unique.
type Link struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	OwnerID   uint       `gorm:"index;default:1" json:"-"`
	Host      string     `gorm:"size:255;index:idx_host_slug,unique" json:"host"` // empty = default/any host
	Slug      string     `gorm:"size:255;index:idx_host_slug,unique" json:"slug"`
	Target    string     `gorm:"type:text" json:"target"`
	Password  string     `gorm:"size:255" json:"-"` // optional; presence exposed via HasPassword
	Note      string     `gorm:"type:text" json:"note"`
	Title     string     `gorm:"size:255" json:"title"`
	ExpiresAt *time.Time `json:"expiresAt"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	Clicks    int64      `gorm:"default:0" json:"clicks"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
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
	ID         uint      `gorm:"primaryKey" json:"id"`
	MailboxID  uint      `gorm:"index" json:"mailboxId"`
	MessageID  string    `gorm:"size:512;index" json:"messageId"`
	FromAddr   string    `gorm:"size:320" json:"from"`
	ToAddr     string    `gorm:"size:320" json:"to"`
	Subject    string    `gorm:"type:text" json:"subject"`
	Text       string    `gorm:"type:text" json:"text"`
	HTML       string    `gorm:"type:text" json:"html"`
	Raw        []byte    `gorm:"type:blob" json:"-"`
	Read       bool      `gorm:"default:false" json:"read"`
	Note       string    `gorm:"type:text" json:"note"`
	ReceivedAt time.Time `gorm:"index" json:"receivedAt"`
}

// AllModels lists every model for AutoMigrate.
func AllModels() []any {
	return []any{
		&Domain{}, &Link{}, &LinkEvent{}, &Mailbox{}, &Email{},
	}
}
