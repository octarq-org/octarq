package app

import (
	"errors"
	"net/http"
	"sync"

	"github.com/octarq-org/octarq/server/internal/api"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
)

// hostSession implements plugin.HostSession.
type hostSession struct {
	auth       *auth.Manager
	apiHandler *api.Handler
}

func (s *hostSession) UserID(r *http.Request) uint {
	if s.auth != nil {
		return s.auth.UserID(r)
	}
	return 0
}

func (s *hostSession) OrgID(r *http.Request) uint {
	if s.auth != nil {
		return s.auth.OrgID(r)
	}
	return 0
}

func (s *hostSession) OrgRole(r *http.Request) string {
	if s.apiHandler != nil {
		return s.apiHandler.OrgRole(r)
	}
	return ""
}

func (s *hostSession) RequireRole(r *http.Request, min string) bool {
	if s.apiHandler != nil {
		return s.apiHandler.RequireRole(r, min)
	}
	return false
}

func (s *hostSession) RequirePerm(r *http.Request, permKey, minRole string) bool {
	if s.apiHandler != nil {
		return s.apiHandler.RequirePerm(r, permKey, minRole)
	}
	return false
}

func (s *hostSession) IsInstanceAdmin(r *http.Request) bool {
	if s.apiHandler != nil {
		return s.apiHandler.IsInstanceAdmin(r)
	}
	return false
}

func (s *hostSession) RevokeUserOrgSessions(userID, orgID uint) int {
	if s.auth != nil {
		return s.auth.RevokeUserOrgSessions(userID, orgID)
	}
	return 0
}

var _ plugin.HostSession = (*hostSession)(nil)

// hostCryptoVault implements plugin.CryptoVault.
type hostCryptoVault struct {
	cipher *crypto.Cipher
}

func (c *hostCryptoVault) Encrypt(plaintext []byte) (string, error) {
	if c.cipher != nil {
		return c.cipher.Encrypt(plaintext)
	}
	return "", errors.New("cipher not configured")
}

func (c *hostCryptoVault) Decrypt(encoded string) ([]byte, error) {
	if c.cipher != nil {
		return c.cipher.Decrypt(encoded)
	}
	return nil, errors.New("cipher not configured")
}

var _ plugin.CryptoVault = (*hostCryptoVault)(nil)

// hostSettingsStore implements plugin.SettingsStore.
type hostSettingsStore struct {
	apiHandler *api.Handler
}

func (s *hostSettingsStore) GetWorkspaceSetting(orgID uint, key string) string {
	if s.apiHandler != nil && orgID != 0 {
		return s.apiHandler.GetWorkspaceSetting(orgID, key)
	}
	return ""
}

func (s *hostSettingsStore) SetWorkspaceSetting(orgID uint, key, value string) error {
	if orgID == 0 {
		return tenantsql.ErrZeroOrgID
	}
	if s.apiHandler != nil {
		return s.apiHandler.SetWorkspaceSetting(orgID, key, value)
	}
	return errors.New("api handler not configured")
}

func (s *hostSettingsStore) GetGlobalSetting(key string) string {
	if s.apiHandler != nil {
		return s.apiHandler.GetGlobalSetting(key)
	}
	return ""
}

func (s *hostSettingsStore) SetGlobalSetting(key, value string) error {
	if s.apiHandler != nil {
		return s.apiHandler.SetGlobalSetting(key, value)
	}
	return errors.New("api handler not configured")
}

var _ plugin.SettingsStore = (*hostSettingsStore)(nil)

// hostEventSpine implements plugin.EventSpine.
type hostEventSpine struct {
	services        *plugin.Registry
	emailMu         *sync.Mutex
	deferredOnEmail *[]func(plugin.EmailEvent)
}

func (e *hostEventSpine) PublishEvent(orgID uint, event string, data any) {
	eventbus.Publish(orgID, event, data)
}

func (e *hostEventSpine) RegisterWebhookEvent(def plugin.WebhookEventDef) {
	eventbus.RegisterEventDef(eventbus.EventDef{
		Key:         def.Key,
		Group:       def.Group,
		Title:       def.Title,
		Description: def.Description,
	})
}

func (e *hostEventSpine) OnEmail(handler func(plugin.EmailEvent)) {
	if handler == nil {
		return
	}
	if e.services != nil {
		if onEmailService, ok := plugin.LookupServiceAs[plugin.EmailDispatcher](e.services.Lookup, plugin.ServiceMailDispatcher); ok {
			onEmailService(handler)
			return
		}
	}
	if e.emailMu != nil && e.deferredOnEmail != nil {
		e.emailMu.Lock()
		*e.deferredOnEmail = append(*e.deferredOnEmail, handler)
		e.emailMu.Unlock()
	}
}

var _ plugin.EventSpine = (*hostEventSpine)(nil)
