package tenantsql_test

import (
	"fmt"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

type sqlTestLink struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Host      string
	Slug      string
	Target    string
	Password  string
	Note      string
	Title     string
	Tags      string
	Clicks    int64
	Archived  bool
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (sqlTestLink) TableName() string { return "links" }

type sqlTestLinkEvent struct {
	ID          uint `gorm:"primaryKey"`
	LinkID      uint
	CreatedAt   time.Time
	IP          string
	Country     string
	Region      string
	City        string
	Device      string
	Browser     string
	OS          string
	Referer     string
	UA          string
	Fingerprint string
	IsBot       bool
	Variant     string
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
}

func (sqlTestLinkEvent) TableName() string { return "link_events" }

type sqlTestMailbox struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Address   string
	Note      string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (sqlTestMailbox) TableName() string { return "mailboxes" }

type sqlTestEmail struct {
	ID         uint `gorm:"primaryKey"`
	MailboxID  uint
	MessageID  string
	FromAddr   string
	ToAddr     string
	Subject    string
	Text       string
	HTML       string
	StorageKey string
	Read       bool
	Note       string
	AuthSPF    string
	AuthDKIM   string
	AuthDMARC  string
	ReceivedAt time.Time
}

func (sqlTestEmail) TableName() string { return "emails" }

func registerTestViews(ctx *plugin.Context) {
	if ctx == nil || ctx.RegisterTenantView == nil {
		return
	}

	ctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_links",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer"},
			{Name: "owner_id", Type: "integer"},
			{Name: "host", Type: "text"},
			{Name: "slug", Type: "text"},
			{Name: "target", Type: "text"},
			{Name: "password", Type: "text"},
			{Name: "title", Type: "text"},
			{Name: "tags", Type: "text"},
			{Name: "clicks", Type: "integer"},
			{Name: "archived", Type: "boolean"},
			{Name: "enabled", Type: "boolean"},
			{Name: "created_at", Type: "datetime"},
		},
		Sensitive: []string{"password"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT id, owner_id, host, slug, target, password, title, tags, clicks, archived, enabled, created_at FROM links WHERE owner_id = %d", orgID)
		},
	})

	ctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_link_events",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer"},
			{Name: "link_id", Type: "integer"},
			{Name: "created_at", Type: "datetime"},
			{Name: "ip", Type: "text"},
			{Name: "fingerprint", Type: "text"},
		},
		Sensitive: []string{"fingerprint"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT le.id, le.link_id, le.created_at, le.ip, le.fingerprint FROM link_events le INNER JOIN links l ON le.link_id = l.id WHERE l.owner_id = %d", orgID)
		},
	})

	ctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_emails",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer"},
			{Name: "mailbox_id", Type: "integer"},
			{Name: "message_id", Type: "text"},
			{Name: "from_addr", Type: "text"},
			{Name: "to_addr", Type: "text"},
			{Name: "subject", Type: "text"},
			{Name: "text", Type: "text"},
			{Name: "html", Type: "text"},
			{Name: "storage_key", Type: "text"},
		},
		Sensitive: []string{"subject", "text", "html", "storage_key"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT e.id, e.mailbox_id, e.message_id, e.from_addr, e.to_addr, e.subject, e.text, e.html, e.storage_key FROM emails e INNER JOIN mailboxes m ON e.mailbox_id = m.id WHERE m.owner_id = %d", orgID)
		},
	})
}
