package mail

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
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

// Describe marks the plugin Core: always-on plumbing.
func (p *Plugin) Describe() plugin.Info { return plugin.Info{Title: "Mail", Core: true} }

func (p *Plugin) Models() []any {
	return []any{&Mailbox{}, &Email{}, &SMTPSender{}}
}

// orgDB scopes a query to the caller's org.
func (p *Plugin) orgDB(r *http.Request) *gorm.DB {
	return p.db.Where("owner_id = ?", p.orgID(r))
}

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	p.db = ctx.DB
	p.orgID = ctx.OrgID
	p.audit = ctx.Audit
	p.encrypt = ctx.Encrypt
	p.decrypt = ctx.Decrypt
	p.getWorkspaceSetting = ctx.GetWorkspaceSetting
	p.getGlobalSetting = ctx.GetGlobalSetting

	api := ctx.Huma

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
