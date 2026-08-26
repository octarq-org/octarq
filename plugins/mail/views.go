package mail

import (
	"fmt"

	"github.com/octarq-org/octarq/plugin"
)

// RegisterViews registers the core tenant view for emails.
func RegisterViews(ctx *plugin.Context) {
	if ctx == nil || ctx.RegisterTenantView == nil {
		return
	}

	ctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_emails",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "Email message unique identifier"},
			{Name: "mailbox_id", Type: "integer", Description: "Associated mailbox identifier"},
			{Name: "message_id", Type: "text", Description: "RFC 822 Message-ID header"},
			{Name: "from_addr", Type: "text", Description: "Sender email address"},
			{Name: "to_addr", Type: "text", Description: "Recipient email address"},
			{Name: "subject", Type: "text", Description: "Email subject line (redacted)"},
			{Name: "text", Type: "text", Description: "Plain text email body content (redacted)"},
			{Name: "html", Type: "text", Description: "HTML email body content (redacted)"},
			{Name: "storage_key", Type: "text", Description: "Raw email object storage reference key"},
			{Name: "read", Type: "boolean", Description: "Whether the message has been marked as read"},
			{Name: "note", Type: "text", Description: "Operator annotation or note"},
			{Name: "attachments", Type: "text", Description: "JSON array of attachment metadata descriptors"},
			{Name: "auth_spf", Type: "text", Description: "SPF authentication verdict"},
			{Name: "auth_dkim", Type: "text", Description: "DKIM authentication verdict"},
			{Name: "auth_dmarc", Type: "text", Description: "DMARC authentication verdict"},
			{Name: "received_at", Type: "datetime", Description: "Timestamp when email was received"},
		},
		Sensitive: []string{"subject", "text", "html", "raw"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT e.id, e.mailbox_id, e.message_id, e.from_addr, e.to_addr, e.subject, e.text, e.html, e.storage_key, e.read, e.note, e.attachments, e.auth_spf, e.auth_dkim, e.auth_dmarc, e.received_at FROM emails e INNER JOIN mailboxes m ON e.mailbox_id = m.id WHERE m.owner_id = %d", orgID)
		},
	})
}
