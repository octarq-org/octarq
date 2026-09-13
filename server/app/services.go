package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// WithWebFS overrides the embedded open-source dashboard with a caller-supplied
// filesystem. A distribution composing extra plugins uses this to serve a dashboard
// built with those pages injected (VITE_OCTARQ_PLUGINS=...) instead of the
// core's OSS bundle, whose empty plugin registry 404-degrades those pages. Pass
// an fs.FS rooted where the core's webembed.FS() would be (index.html at root).
func (a *App) WithWebFS(f fs.FS) *App { a.webFS = f; return a }

// DB exposes the shared database handle (useful for plugin construction).
func (a *App) DB() *gorm.DB { return a.gdb }

// loginByEmail backs plugin.Context.LoginByEmail. It delegates to the auth
// manager and translates the internal registration-disabled sentinel into the
// plugin-package one so identity plugins can errors.Is against it without
// importing internal/auth.
func (a *App) loginByEmail(w http.ResponseWriter, r *http.Request, email string) (uint, error) {
	uid, err := a.auth.LoginByEmail(w, r, email)
	if errors.Is(err, auth.ErrRegistrationDisabled) {
		return uid, plugin.ErrLoginRegistrationDisabled
	}
	return uid, err
}

// loginByIdentity backs plugin.Context.LoginByIdentity, translating the
// internal sentinels into the plugin-package ones the same way loginByEmail
// does. The two refusals are deliberately distinct: "this instance won't create
// accounts" is something an admin can change, while "that email already has an
// account" tells the visitor to sign in the way they already can and link SSO
// afterwards.
func (a *App) loginByIdentity(w http.ResponseWriter, r *http.Request, id plugin.ExternalIdentity) (uint, error) {
	uid, err := a.auth.LoginByIdentity(w, r, id)
	switch {
	case errors.Is(err, auth.ErrRegistrationDisabled):
		return uid, plugin.ErrLoginRegistrationDisabled
	case errors.Is(err, auth.ErrAccountLinkRequired):
		return uid, plugin.ErrAccountLinkRequired
	}
	return uid, err
}

// Notify delivers a notification via a configured channel type ("telegram", "webhook").
func (a *App) Notify(ctx context.Context, typ, cfgJSON, text string) error {
	return notify.Send(ctx, typ, cfgJSON, text)
}

// sendMail is the implementation behind plugin.Context.SendMail. It delegates
// to the mail plugin's "mail.send" service, which resolves the org's first
// configured SMTP sender, decrypts its password, and relays the message —
// mirroring internal/api.Handler.sendEmail so plugins can send transactional
// mail without importing octarq's internal packages.
func (a *App) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	if a.services != nil {
		if fn, ok := plugin.LookupServiceAs[plugin.MailSender](a.services.Lookup, plugin.ServiceMailSend); ok {
			return fn(orgID, to, subject, htmlBody, textBody)
		}
	}
	return fmt.Errorf("no mail plugin mounted to send email for org %d", orgID)
}

// Use registers a plugin. All plugins must be registered before Run so their
// models are migrated and their routes mounted.
func (a *App) Use(p plugin.Plugin) { a.plugins = append(a.plugins, p) }

// Plugins returns the registered plugins.
func (a *App) Plugins() []plugin.Plugin {
	return a.plugins
}

// lazyDNSManager resolves the plugin.DNSManager provided by the dns Core plugin
// under plugin.ServiceDNSManager on each call. Resolution happens at request
// time, after all plugins have mounted.
type lazyDNSManager struct {
	lookup func(name string) (any, bool)
}

var _ plugin.DNSManager = (*lazyDNSManager)(nil)

func (l *lazyDNSManager) resolve() (plugin.DNSManager, error) {
	if v, ok := l.lookup(plugin.ServiceDNSManager); ok {
		if m, ok := v.(plugin.DNSManager); ok {
			return m, nil
		}
	}
	return nil, errors.New("dns manager unavailable: the dns plugin is not mounted")
}

func (l *lazyDNSManager) List(ctx context.Context, orgID, domainID uint) ([]plugin.DNSRecord, error) {
	m, err := l.resolve()
	if err != nil {
		return nil, err
	}
	return m.List(ctx, orgID, domainID)
}

func (l *lazyDNSManager) Set(ctx context.Context, orgID, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error) {
	m, err := l.resolve()
	if err != nil {
		return plugin.DNSRecord{}, err
	}
	return m.Set(ctx, orgID, domainID, r)
}

func (l *lazyDNSManager) Delete(ctx context.Context, orgID, domainID uint, recordID string) error {
	m, err := l.resolve()
	if err != nil {
		return err
	}
	return m.Delete(ctx, orgID, domainID, recordID)
}
