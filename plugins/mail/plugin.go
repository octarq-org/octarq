package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

var errNotFound = errors.New("not found")

// Plugin implements the octarq plugin contract for mail CRUD.
type Plugin struct {
	db                  *gorm.DB
	orgID               func(*http.Request) uint
	audit               func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	encrypt             func(plaintext []byte) (string, error)
	decrypt             func(encoded string) ([]byte, error)
	getWorkspaceSetting func(orgID uint, key string) string
	getGlobalSetting    func(key string) string
	sendLimiter         *rateLimiter
	emailMu             sync.RWMutex
	emailHandlers       []func(plugin.EmailEvent)
	notify              func(ctx context.Context, kind string, config map[string]any, message string) error
	publishEvent        func(orgID uint, event string, data any)
}

// Compile-time capability checks.
var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)

// New constructs the mail plugin.
func New() *Plugin {
	return &Plugin{
		sendLimiter: newRateLimiter("", "send", 100, time.Hour),
	}
}

func (p *Plugin) Name() string { return "mail" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{Title: "Mail", Description: "Transactional email sending, mailbox receiving, and SMTP configurations.", EnabledByDefault: true, Requires: []string{"dns", "links"}}
}

func (p *Plugin) Models() []any {
	return []any{&Mailbox{}, &Email{}, &SMTPSender{}}
}

// orgDB scopes a query to the caller's org.
func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.orgID(r))
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx.DB != nil {
		p.db = ctx.DB
	}
	if ctx.OrgID != nil {
		p.orgID = ctx.OrgID
	}
	if ctx.Audit != nil {
		p.audit = ctx.Audit
	}
	if ctx.Encrypt != nil {
		p.encrypt = ctx.Encrypt
	}
	if ctx.Decrypt != nil {
		p.decrypt = ctx.Decrypt
	}
	if ctx.GetWorkspaceSetting != nil {
		p.getWorkspaceSetting = ctx.GetWorkspaceSetting
	}
	if ctx.GetGlobalSetting != nil {
		p.getGlobalSetting = ctx.GetGlobalSetting
	}
	if ctx.Notify != nil {
		p.notify = func(c context.Context, kind string, config map[string]any, message string) error {
			var cfgJSON string
			if config != nil {
				if b, err := json.Marshal(config); err == nil {
					cfgJSON = string(b)
				}
			}
			return ctx.Notify(c, kind, cfgJSON, message)
		}
	}

	if ctx.PublishEvent != nil {
		p.publishEvent = ctx.PublishEvent
	}
	if ctx.RegisterWebhookEvent != nil {
		ctx.RegisterWebhookEvent(plugin.WebhookEventDef{Key: "email.receive", Group: "Email", Title: "Email Received", Description: "An inbound email was delivered to a mailbox"})
		ctx.RegisterWebhookEvent(plugin.WebhookEventDef{Key: "email.send_failed", Group: "Email", Title: "Email Send Failed", Description: "An outbound email failed to send through the configured SMTP sender"})
	}

	api := ctx.Huma
	if api != nil {
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/smtp-senders", Summary: "List SMTP Senders", Tags: []string{"SMTP"}}, p.listSMTPSenders)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/smtp-senders", Summary: "Create SMTP Sender", Tags: []string{"SMTP"}, DefaultStatus: 201}, p.createSMTPSender)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/smtp-senders/{id}", Summary: "Update SMTP Sender", Tags: []string{"SMTP"}}, p.updateSMTPSender)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/smtp-senders/{id}", Summary: "Delete SMTP Sender", Tags: []string{"SMTP"}}, p.deleteSMTPSender)

		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mailboxes", Summary: "List Mailboxes", Tags: []string{"Mailboxes"}}, p.listMailboxes)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/mailboxes", Summary: "Create Mailbox", Tags: []string{"Mailboxes"}, DefaultStatus: 201}, p.createMailbox)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/mailboxes/{id}", Summary: "Update Mailbox", Tags: []string{"Mailboxes"}}, p.updateMailbox)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/mailboxes/{id}", Summary: "Delete Mailbox", Tags: []string{"Mailboxes"}}, p.deleteMailbox)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails", Summary: "List Emails", Tags: []string{"Emails"}}, p.listEmails)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/emails/read-all", Summary: "Mark All Emails Read", Tags: []string{"Emails"}}, p.readAllEmails)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}", Summary: "Get Email", Tags: []string{"Emails"}}, p.getEmail)
		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}/raw", Summary: "Get Raw Email EML", Tags: []string{"Emails"}}, p.rawEmail)
		huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/emails/{id}", Summary: "Update Email State", Tags: []string{"Emails"}}, p.updateEmail)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/emails/{id}", Summary: "Delete Email", Tags: []string{"Emails"}}, p.deleteEmail)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/emails/send", Summary: "Send Email", Tags: []string{"Emails"}}, p.sendEmail)

		huma.Register(api, huma.Operation{
			Method: "POST", Path: "/api/webhook/{orgSlug}/email/inbound/{token}",
			Summary: "Inbound Email Webhook", Tags: []string{"Mailboxes"},
			Metadata: map[string]any{"public": true},
		}, p.inbound)
		huma.Register(api, huma.Operation{
			Method: "POST", Path: "/api/webhook/{orgSlug}/email/bounce/{token}",
			Summary: "Email Bounce Webhook", Tags: []string{"Mailboxes"},
			Metadata: map[string]any{"public": true},
		}, p.emailBounceWebhook)
	}

	if ctx.Provide != nil {
		ctx.Provide("mail.dispatcher", p.OnEmail)
		ctx.Provide("mail.overview", p.overview)
		ctx.Provide("mail.purge", p.purge)
		ctx.Provide("mail.export", p.exportData)
		ctx.Provide("mail.send", p.sendMail)
		ctx.Provide("mail.email.get", p.getEmailForSummarize)
		ctx.Provide("mailboxes.mcp_export", p.mcpExportMailboxes)
		ctx.Provide("emails.mcp_export", p.mcpExportEmails)
	}
}

func (p *Plugin) purge(orgID uint) error {
	mailboxIDs := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	p.db.Where("mailbox_id IN (?)", mailboxIDs).Delete(&Email{})
	p.db.Where("owner_id = ?", orgID).Delete(&Mailbox{})
	p.db.Where("owner_id = ?", orgID).Delete(&SMTPSender{})
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	var mailboxes []Mailbox
	var emails []Email
	var smtp []SMTPSender
	p.db.Where("owner_id = ?", orgID).Find(&mailboxes)
	mailboxIDs := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	p.db.Where("mailbox_id IN (?)", mailboxIDs).Find(&emails)
	p.db.Where("owner_id = ?", orgID).Find(&smtp)
	return map[string]any{
		"mailboxes":   mailboxes,
		"emails":      emails,
		"smtpSenders": smtp,
	}
}

func (p *Plugin) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	var s SMTPSender
	if err := p.db.Where("owner_id = ?", orgID).Order("id").First(&s).Error; err != nil {
		return fmt.Errorf("no SMTP sender configured for org %d", orgID)
	}
	pass, err := p.decrypt(s.Pass)
	if err != nil {
		return err
	}
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
	if err := sender.Send(mail.Message{From: s.FromEmail, To: []string{to}, Subject: subject, HTML: htmlBody, Text: textBody}); err != nil {
		if p.publishEvent != nil {
			p.publishEvent(orgID, "email.send_failed", map[string]any{"to": []string{to}, "subject": subject, "error": err.Error()})
		}
		return err
	}
	return nil
}

var htmlTagRe = regexp.MustCompile(`(?s)<style.*?</style>|<script.*?</script>|<[^>]*>`)

func (p *Plugin) getEmailForSummarize(orgID uint, id uint) (from, subject, body string, ok bool) {
	var count int64
	p.db.Model(&Email{}).
		Joins("JOIN mailboxes ON mailboxes.id = emails.mailbox_id AND mailboxes.owner_id = ?", orgID).
		Where("emails.id = ?", id).Count(&count)
	if count == 0 {
		return "", "", "", false
	}
	var e Email
	if p.db.First(&e, id).Error != nil {
		return "", "", "", false
	}
	b := e.Text
	if strings.TrimSpace(b) == "" {
		b = htmlTagRe.ReplaceAllString(e.HTML, " ")
	}
	return e.FromAddr, e.Subject, b, true
}

func (p *Plugin) overview(orgID uint, includeBot bool) map[string]any {
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	count := func(model any, conds ...any) int64 {
		var n int64
		q := p.db.Model(model).Where("owner_id = ?", orgID)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	emailCount := func(conds ...any) int64 {
		var n int64
		q := p.db.Model(&Email{}).Where("mailbox_id IN (?)", orgMailboxes)
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		q.Count(&n)
		return n
	}
	type recentEmail struct {
		ID         uint      `json:"id"`
		FromAddr   string    `json:"from"`
		Subject    string    `json:"subject"`
		Read       bool      `json:"read"`
		ReceivedAt time.Time `json:"receivedAt"`
	}
	var recent []recentEmail
	p.db.Model(&Email{}).
		Select("id, from_addr, subject, read, received_at").
		Where("mailbox_id IN (?)", orgMailboxes).
		Order("received_at DESC").Limit(6).Scan(&recent)

	return map[string]any{
		"mailboxes":    count(&Mailbox{}),
		"emails":       emailCount(),
		"unread":       emailCount("read = ?", false),
		"recentEmails": recent,
	}
}

var builtinReservedSlugs = map[string]bool{
	"admin": true, "api": true, "assets": true, "portal": true,
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (p *Plugin) isReservedSlug(slug string) bool {
	slug = strings.ToLower(slug)
	if builtinReservedSlugs[slug] {
		return true
	}
	if p.getGlobalSetting != nil {
		for _, res := range splitList(p.getGlobalSetting("reserved_slugs")) {
			if res == slug {
				return true
			}
		}
	}
	return false
}
