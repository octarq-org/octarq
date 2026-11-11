package api

import (
	"time"
)

type testDomain struct {
	ID                uint   `gorm:"primaryKey"`
	OrgID             uint   `gorm:"column:owner_id"`
	Name              string `gorm:"uniqueIndex;size:255"`
	ProviderAccountID uint
	ZoneID            string
	Note              string
	ForMail           bool
	ForLink           bool
	LinkHosts         string
	MailHosts         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (testDomain) TableName() string { return "domains" }

type testProviderAccount struct {
	ID      uint `gorm:"primaryKey"`
	OrgID   uint `gorm:"column:owner_id"`
	Type    string
	Name    string
	Secrets string
}

func (testProviderAccount) TableName() string { return "provider_accounts" }

type testLink struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Host      string
	Slug      string
	Target    string
	Title     string
	Password  string
	Clicks    int64
	Tags      string
	Archived  bool
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (testLink) TableName() string { return "links" }

type testMailbox struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Address   string
	Note      string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (testMailbox) TableName() string { return "mailboxes" }

type testEmail struct {
	ID         uint `gorm:"primaryKey"`
	MailboxID  uint
	MessageID  string
	FromAddr   string
	Subject    string
	Text       string
	Raw        []byte
	Read       bool
	ReceivedAt time.Time
}

func (testEmail) TableName() string { return "emails" }

type testSMTPSender struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Name      string
	Host      string
	Port      int
	User      string
	Pass      string
	FromEmail string
}

func (testSMTPSender) TableName() string { return "smtp_senders" }
