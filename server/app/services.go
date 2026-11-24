package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func (a *App) WithWebFS(f fs.FS) *App { a.webFS = f; return a }

func (a *App) WithMCPTierGate(g plugin.TierGate) *App { a.mcpTierGate = g; return a }

func (a *App) DB() *gorm.DB { return a.gdb }

func (a *App) loginByEmail(w http.ResponseWriter, r *http.Request, email string) (uint, error) {
	uid, err := a.auth.LoginByEmail(w, r, email)
	if errors.Is(err, auth.ErrRegistrationDisabled) {
		return uid, plugin.ErrLoginRegistrationDisabled
	}
	return uid, err
}

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

func (a *App) Notify(ctx context.Context, typ, cfgJSON, text string) error {
	return notification.DefaultRouter().SendDirect(ctx, typ, cfgJSON, text)
}

func (a *App) sendMail(orgID uint, to, subject, htmlBody, textBody string) error {
	if a.services != nil {
		if fn, ok := plugin.LookupServiceAs[plugin.MessagingSender](a.services.Lookup, plugin.ServiceMessagingSend); ok {
			return fn(orgID, to, subject, htmlBody, textBody)
		}
	}
	return fmt.Errorf("no mail plugin mounted to send email for org %d", orgID)
}

func (a *App) Use(p plugin.Plugin) { a.plugins = append(a.plugins, p) }

func (a *App) Plugins() []plugin.Plugin {
	return a.plugins
}

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
