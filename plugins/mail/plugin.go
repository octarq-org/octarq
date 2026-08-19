package mail

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/idempotency"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/usagemetric"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

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
	// notify takes the stored config JSON directly. Configs are encrypted at rest
	// and decrypted inside core dispatch (internal/notify.Send), so do not pre-parse.
	notify       func(ctx context.Context, kind, cfgJSON, message string) error
	publishEvent func(orgID uint, event string, data any)
	recordUsage  func(orgID uint, metric string, n int64)
	requireRole  func(r *http.Request, min string) bool
	ctx          *plugin.Context
}

// Compile-time capability checks.
var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.HelpDocsFS   = (*Plugin)(nil)
)

// Compile-time service contract assertions: these methods are provided to the
// registry under the named contract types in Mount. A signature drift here
// fails the build instead of silently breaking consumers' LookupServiceAs.
var (
	_ plugin.MailSender       = (*Plugin)(nil).sendMail
	_ plugin.EmailDispatcher  = (*Plugin)(nil).OnEmail
	_ plugin.MailReady        = (*Plugin)(nil).mailReady
	_ plugin.SystemMailSender = (*Plugin)(nil).sendSystemMail
	_ plugin.ExportFunc       = (*Plugin)(nil).exportData
	_ plugin.PurgeFunc        = (*Plugin)(nil).purge
	_ plugin.OverviewFunc     = (*Plugin)(nil).overview
	_ plugin.EmailGetter      = (*Plugin)(nil).getEmailForSummarize
	_ plugin.MCPExporter      = (*Plugin)(nil).mcpExportEmails
	_ plugin.MCPExporter      = (*Plugin)(nil).mcpExportMailboxes
)

// New constructs the mail plugin.
func New() *Plugin {
	return &Plugin{
		sendLimiter: newRateLimiter("", "send", 100, time.Hour),
	}
}

func (p *Plugin) Name() string { return "mail" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Mail",
		Description:      "Transactional email sending, mailbox receiving, and SMTP configurations.",
		Category:         plugin.CategoryMessaging,
		Tags:             []string{"smtp", "inbox", "webhooks"},
		EnabledByDefault: true,
		Requires:         []string{"dns", "links"},
	}
}

func (p *Plugin) Models() []any {
	return []any{&Mailbox{}, &Email{}, &SMTPSender{}, &MailRawBlob{}, &MailSuppression{}}
}

// Menus announces this plugin's sidebar entry so /api/menus only offers it
// when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Menus() []plugin.MenuItem {
	return []plugin.MenuItem{
		{ID: "mail", Label: "Mail", Path: "/mail", Icon: "mail", Category: "Messaging", Order: 20},
	}
}

// Actions announces this plugin's global create affordances so /api/actions
// only offers them when the plugin is mounted and enabled for the workspace.
func (p *Plugin) Actions() []plugin.Action {
	return []plugin.Action{
		{ID: "create-mailbox", Label: "New Mailbox", Path: "/mail?create=1", Icon: "mail", Category: "Messaging", Order: 10},
	}
}

// docs is this plugin's documentation directory. Adding a page means adding
// "docs/<slug>.mdx" (plus its "<slug>.zh.mdx" translation) — the file name is
// the slug and the frontmatter carries the rest; see plugin.HelpDocsFS.
//
//go:embed docs
var docs embed.FS

func (p *Plugin) HelpDocsFS() fs.FS { return docs }

// orgDB scopes a query to the caller's org.
func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.orgID(r))
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.ctx = ctx
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
		p.notify = ctx.Notify
	}

	if ctx.PublishEvent != nil {
		p.publishEvent = ctx.PublishEvent
	}
	if ctx.RecordUsage != nil {
		p.recordUsage = ctx.RecordUsage
	}
	if ctx.RequireRole != nil {
		p.requireRole = ctx.RequireRole
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
		var sendEmailMws huma.Middlewares
		if ctx.Lookup != nil {
			if idem, ok := plugin.LookupServiceAs[func(http.Handler) http.Handler](ctx.Lookup, idempotency.ServiceName); ok {
				sendEmailMws = append(sendEmailMws, idempotency.HumaMiddleware(idem))
			}
		}
		huma.Register(api, huma.Operation{
			Method: "POST", Path: "/api/emails/send", Summary: "Send Email", Tags: []string{"Emails"},
			Middlewares: sendEmailMws,
		}, p.sendEmail)

		huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mail/suppressions", Summary: "List Mail Suppressions", Tags: []string{"Emails"}}, p.listSuppressions)
		huma.Register(api, huma.Operation{Method: "POST", Path: "/api/mail/suppressions", Summary: "Create Mail Suppression", Tags: []string{"Emails"}, DefaultStatus: 201}, p.createSuppression)
		huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/mail/suppressions/{id}", Summary: "Delete Mail Suppression", Tags: []string{"Emails"}}, p.deleteSuppression)

		huma.Register(api, huma.Operation{
			Method: "POST", Path: "/api/webhook/{orgSlug}/email/inbound/{token}",
			Summary: "Inbound Email Webhook", Tags: []string{"Mailboxes"},
			Metadata: map[string]any{"public": true},
		}, p.inbound)
		huma.Register(api, huma.Operation{
			Method: "POST", Path: "/api/webhook/{orgSlug}/email/inbound/raw/{token}",
			Summary: "Generic Inbound Email Webhook (Raw EML)", Tags: []string{"Mailboxes"},
			Metadata: map[string]any{"public": true},
		}, p.inboundGeneric)
		huma.Register(api, huma.Operation{
			Method: "POST", Path: "/api/webhook/{orgSlug}/email/bounce/{token}",
			Summary: "Email Bounce Webhook", Tags: []string{"Mailboxes"},
			Metadata: map[string]any{"public": true},
		}, p.emailBounceWebhook)
	}

	if ctx.Provide != nil {
		ctx.Provide(plugin.ServiceMailDispatcher, plugin.EmailDispatcher(p.OnEmail))
		ctx.Provide(plugin.OverviewServiceName("mail"), plugin.OverviewFunc(p.overview))
		ctx.Provide(plugin.PurgeServiceName("mail"), plugin.PurgeFunc(p.purge))
		ctx.Provide(plugin.ExportServiceName("mail"), plugin.ExportFunc(p.exportData))
		ctx.Provide(plugin.ServiceMailSend, plugin.MailSender(p.sendMail))
		ctx.Provide(plugin.ServiceMailReady, plugin.MailReady(p.mailReady))
		ctx.Provide(plugin.ServiceMailSendSystem, plugin.SystemMailSender(p.sendSystemMail))
		ctx.Provide(plugin.ServiceMailEmailGet, plugin.EmailGetter(p.getEmailForSummarize))
		ctx.Provide(plugin.MCPExportServiceName("mailboxes"), plugin.MCPExporter(p.mcpExportMailboxes))
		ctx.Provide(plugin.MCPExportServiceName("emails"), plugin.MCPExporter(p.mcpExportEmails))
	}
}

func (p *Plugin) getStorageProvider() (plugin.StorageProvider, error) {
	backend := p.getBackendConfig()
	if backend != "" && backend != "database" && backend != "db" {
		if p.ctx != nil {
			if sp, ok := plugin.LookupAs[plugin.StorageProvider](p.ctx, plugin.ServiceMailStorageProvider); ok && sp != nil {
				if _, isDB := sp.(*DBStorageProvider); !isDB {
					return sp, nil
				}
			}
		}
		return nil, fmt.Errorf("mail storage provider %q requires Pro edition", backend)
	}

	if p.ctx != nil {
		if sp, ok := plugin.LookupAs[plugin.StorageProvider](p.ctx, plugin.ServiceMailStorageProvider); ok && sp != nil {
			return sp, nil
		}
	}
	return NewDBStorageProvider(p.db), nil
}

// getBackendConfig resolves which storage backend to use. The Pro mailstorage
// module owns the authoritative configuration table and pushes the runtime key
// here via SetGlobalSetting; absent a value the database backend — the only one
// OSS ships — applies.
func (p *Plugin) getBackendConfig() string {
	if p.getGlobalSetting != nil {
		if val := strings.TrimSpace(p.getGlobalSetting("mail_storage_backend")); val != "" {
			return val
		}
	}
	return "database"
}

func (p *Plugin) isSuppressed(orgID uint, addr string) bool {
	if addr == "" || orgID == 0 {
		return false
	}
	normAddr := strings.ToLower(strings.TrimSpace(addr))
	var count int64
	p.db.Model(&MailSuppression{}).Where("owner_id = ? AND address = ?", orgID, normAddr).Count(&count)
	return count > 0
}

func (p *Plugin) purge(orgID uint) error {
	ctx := context.Background()
	ctx = plugin.WithOrgID(ctx, orgID)
	mailboxIDs := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)

	storageProv, spErr := p.getStorageProvider()
	if spErr != nil {
		log.Printf("mail purge: storage provider unavailable (%v); deleting database blobs only", spErr)
	}
	dbProv := NewDBStorageProvider(p.db)
	delCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for {
		var emails []Email
		if err := p.db.Select("id", "storage_key").Where("mailbox_id IN (?)", mailboxIDs).Limit(2000).Find(&emails).Error; err != nil {
			log.Printf("mail purge: failed to query emails for org %d: %v", orgID, err)
			break
		}
		if len(emails) == 0 {
			break
		}

		for _, e := range emails {
			key := e.StorageKey
			if key == "" {
				key = fmt.Sprintf("mail/%d/%d.eml", orgID, e.ID)
			}
			if storageProv != nil {
				if err := storageProv.Delete(delCtx, key); err != nil {
					log.Printf("mail purge: failed to delete storage blob %q: %v", key, err)
				}
			}
			if err := dbProv.Delete(delCtx, key); err != nil {
				log.Printf("mail purge: failed to delete database blob %q: %v", key, err)
			}
		}

		var ids []uint
		for _, e := range emails {
			ids = append(ids, e.ID)
		}
		if err := p.db.Where("id IN (?)", ids).Delete(&Email{}).Error; err != nil {
			log.Printf("mail purge: failed to delete email records for org %d: %v", orgID, err)
			break
		}
	}

	p.db.Where("mailbox_id IN (?)", mailboxIDs).Delete(&Email{})
	p.db.Where("owner_id = ?", orgID).Delete(&Mailbox{})
	p.db.Where("owner_id = ?", orgID).Delete(&SMTPSender{})
	p.db.Where("owner_id = ?", orgID).Delete(&MailSuppression{})
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	var mailboxes []Mailbox
	var emails []Email
	var smtp []SMTPSender
	var suppressions []MailSuppression
	p.db.Where("owner_id = ?", orgID).Find(&mailboxes)
	mailboxIDs := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", orgID)
	p.db.Where("mailbox_id IN (?)", mailboxIDs).Find(&emails)
	p.db.Where("owner_id = ?", orgID).Find(&smtp)
	p.db.Where("owner_id = ?", orgID).Find(&suppressions)
	return map[string]any{
		"mailboxes":    mailboxes,
		"emails":       emails,
		"smtpSenders":  smtp,
		"suppressions": suppressions,
	}
}

// mailReady reports whether the instance's SYSTEM sender is available: at
// least one SMTP sender exists somewhere on the instance, which is exactly
// when sendSystemMail (via the mail.send.system service) can deliver.
// Consumed through the mail.ready service by the registration verification
// gate and both readiness reports: "plugin mounted" is not the same question
// as "can this instance deliver a system message".
func (p *Plugin) mailReady() bool {
	var n int64
	p.db.Model(&SMTPSender{}).Count(&n)
	return n > 0
}

// systemSender resolves the instance's system sender: the one selected in
// instance settings (mail_system_sender_id) when present and still existing,
// otherwise the lowest-id sender on the instance (deterministic fallback that
// also covers a stale reference to a deleted sender). Returns an explicit
// error when no sender exists at all.
func (p *Plugin) systemSender() (*SMTPSender, error) {
	if p.getGlobalSetting != nil {
		if idStr := strings.TrimSpace(p.getGlobalSetting("mail_system_sender_id")); idStr != "" {
			if id, err := strconv.ParseUint(idStr, 10, 64); err == nil && id != 0 {
				var s SMTPSender
				if err := p.db.First(&s, id).Error; err == nil {
					return &s, nil
				}
			}
		}
	}
	var s SMTPSender
	if err := p.db.Order("id ASC").First(&s).Error; err != nil {
		return nil, fmt.Errorf("no SMTP sender configured on this instance; system email cannot be sent. Configure an SMTP sender (Mail → SMTP senders) or mount a plugin providing mail.send")
	}
	return &s, nil
}

// sendSystemMail delivers an instance-level system message (verification,
// password reset, invite) through the system sender resolved by systemSender.
// Unlike sendMail it has no orgID: these flows must work for recipients with
// no membership yet. Usage and failure events are attributed to the sender's
// owning workspace so metering and webhooks keep a real org id.
func (p *Plugin) sendSystemMail(to, subject, htmlBody, textBody string) error {
	s, err := p.systemSender()
	if err != nil {
		return err
	}
	pass, err := p.decrypt(s.Pass)
	if err != nil {
		return err
	}
	sender := mail.NewCustomSender(s.Host, fmt.Sprint(s.Port), s.User, string(pass), s.FromEmail)
	if err := sender.Send(mail.Message{From: s.FromEmail, To: []string{to}, Subject: subject, HTML: htmlBody, Text: textBody}); err != nil {
		if p.publishEvent != nil {
			p.publishEvent(s.OrgID, "email.send_failed", map[string]any{"to": []string{to}, "subject": subject, "error": err.Error()})
		}
		return err
	}
	if p.recordUsage != nil {
		p.recordUsage(s.OrgID, usagemetric.MailOut, 1)
	}
	return nil
}

func (p *Plugin) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	if p.isSuppressed(orgID, to) {
		return fmt.Errorf("recipient address %s is in suppression list", to)
	}
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
	if p.recordUsage != nil {
		p.recordUsage(orgID, usagemetric.MailOut, 1)
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

// hasRole reports whether the caller holds at least the given workspace role.
//
// A host that never wired RequireRole is refused rather than waved through. The
// gate protects destructive and credential-bearing operations, so "the host did
// not tell us who this is" has to mean no, not yes — an unwired seam would
// otherwise silently disable every role check in this plugin.
func (p *Plugin) hasRole(r *http.Request, min string) bool {
	if p.requireRole == nil {
		return false
	}
	return p.requireRole(r, min)
}
