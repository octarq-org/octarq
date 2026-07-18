package mail

import (
	"time"
)

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
	// PassSet is computed (not persisted): the encrypted password never leaves the
	// server, so the dashboard shows "password is set — leave blank to keep it"
	// rather than an empty field that looks like nothing was ever entered.
	PassSet bool `gorm:"-" json:"passSet"`
}
