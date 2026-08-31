package mail

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"sync"
	"time"

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
	return []any{&Mailbox{}, &Email{}, &SMTPSender{}, &MailRawBlob{}, &MailSuppression{}, &MailContact{}}
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

	RegisterViews(ctx)

	api := ctx.Huma
	if api != nil {
		registerAPIRoutes(p, api, ctx)
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
