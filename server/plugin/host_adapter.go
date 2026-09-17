package plugin

import (
	"errors"
	"net/http"
)

// EnsureHost extracts ctx.Host or adapts ctx's legacy closures to Host.
// If ctx is nil, it returns a safe no-op Host adapter so callers never encounter a nil Host.
func EnsureHost(ctx *Context) Host {
	if ctx == nil {
		return &ctxHostAdapter{ctx: nil}
	}
	if ctx.Host != nil {
		return ctx.Host
	}
	return &ctxHostAdapter{ctx: ctx}
}

// ctxHostAdapter wraps a *Context into a Host.
type ctxHostAdapter struct {
	ctx *Context
}

func (a *ctxHostAdapter) Session() HostSession {
	return &ctxSessionAdapter{ctx: a.ctx}
}

func (a *ctxHostAdapter) Crypto() CryptoVault {
	if a.ctx == nil || (a.ctx.Encrypt == nil && a.ctx.Decrypt == nil) {
		return nil
	}
	return &ctxCryptoAdapter{ctx: a.ctx}
}

func (a *ctxHostAdapter) Settings() SettingsStore {
	return &ctxSettingsAdapter{ctx: a.ctx}
}

func (a *ctxHostAdapter) Events() EventSpine {
	return &ctxEventsAdapter{ctx: a.ctx}
}

func (a *ctxHostAdapter) TenantDB(orgID uint) *TenantDB {
	if a.ctx != nil && a.ctx.TenantDB != nil {
		return a.ctx.TenantDB(orgID)
	}
	return nil
}

var _ Host = (*ctxHostAdapter)(nil)

type ctxSessionAdapter struct {
	ctx *Context
}

func (s *ctxSessionAdapter) UserID(r *http.Request) uint {
	if s.ctx != nil && s.ctx.UserID != nil {
		return s.ctx.UserID(r)
	}
	return 0
}

func (s *ctxSessionAdapter) OrgID(r *http.Request) uint {
	if s.ctx != nil && s.ctx.OrgID != nil {
		return s.ctx.OrgID(r)
	}
	return 0
}

func (s *ctxSessionAdapter) OrgRole(r *http.Request) string {
	if s.ctx != nil && s.ctx.OrgRole != nil {
		return s.ctx.OrgRole(r)
	}
	return ""
}

func (s *ctxSessionAdapter) RequireRole(r *http.Request, min string) bool {
	if s.ctx != nil && s.ctx.RequireRole != nil {
		return s.ctx.RequireRole(r, min)
	}
	return false
}

func (s *ctxSessionAdapter) RequirePerm(r *http.Request, permKey, minRole string) bool {
	if s.ctx != nil && s.ctx.RequirePerm != nil {
		return s.ctx.RequirePerm(r, permKey, minRole)
	}
	if allow, decided := ResolvePerm(r, permKey); decided {
		return allow
	}
	if s.ctx != nil && s.ctx.RequireRole != nil {
		return s.ctx.RequireRole(r, minRole)
	}
	return false
}

func (s *ctxSessionAdapter) IsInstanceAdmin(r *http.Request) bool {
	if s.ctx != nil && s.ctx.IsInstanceAdmin != nil {
		return s.ctx.IsInstanceAdmin(r)
	}
	return false
}

func (s *ctxSessionAdapter) RevokeUserOrgSessions(userID, orgID uint) int {
	if s.ctx != nil && s.ctx.RevokeUserOrgSessions != nil {
		return s.ctx.RevokeUserOrgSessions(userID, orgID)
	}
	return 0
}

var _ HostSession = (*ctxSessionAdapter)(nil)

type ctxCryptoAdapter struct {
	ctx *Context
}

func (c *ctxCryptoAdapter) Encrypt(plaintext []byte) (string, error) {
	if c.ctx != nil && c.ctx.Encrypt != nil {
		return c.ctx.Encrypt(plaintext)
	}
	return "", errors.New("crypto: encrypt unavailable")
}

func (c *ctxCryptoAdapter) Decrypt(encoded string) ([]byte, error) {
	if c.ctx != nil && c.ctx.Decrypt != nil {
		return c.ctx.Decrypt(encoded)
	}
	return nil, errors.New("crypto: decrypt unavailable")
}

var _ CryptoVault = (*ctxCryptoAdapter)(nil)

type ctxSettingsAdapter struct {
	ctx *Context
}

func (s *ctxSettingsAdapter) GetWorkspaceSetting(orgID uint, key string) string {
	if s.ctx != nil && s.ctx.GetWorkspaceSetting != nil {
		return s.ctx.GetWorkspaceSetting(orgID, key)
	}
	return ""
}

func (s *ctxSettingsAdapter) SetWorkspaceSetting(orgID uint, key, value string) error {
	if s.ctx != nil && s.ctx.SetWorkspaceSetting != nil {
		return s.ctx.SetWorkspaceSetting(orgID, key, value)
	}
	return errors.New("settings: unavailable")
}

func (s *ctxSettingsAdapter) GetGlobalSetting(key string) string {
	if s.ctx != nil && s.ctx.GetGlobalSetting != nil {
		return s.ctx.GetGlobalSetting(key)
	}
	return ""
}

func (s *ctxSettingsAdapter) SetGlobalSetting(key, value string) error {
	if s.ctx != nil && s.ctx.SetGlobalSetting != nil {
		return s.ctx.SetGlobalSetting(key, value)
	}
	return errors.New("settings: unavailable")
}

var _ SettingsStore = (*ctxSettingsAdapter)(nil)

type ctxEventsAdapter struct {
	ctx *Context
}

func (e *ctxEventsAdapter) PublishEvent(orgID uint, event string, data any) {
	if e.ctx != nil && e.ctx.PublishEvent != nil {
		e.ctx.PublishEvent(orgID, event, data)
	}
}

func (e *ctxEventsAdapter) RegisterWebhookEvent(def WebhookEventDef) {
	if e.ctx != nil && e.ctx.RegisterWebhookEvent != nil {
		e.ctx.RegisterWebhookEvent(def)
	}
}

func (e *ctxEventsAdapter) OnEmail(handler func(EmailEvent)) {
	if e.ctx != nil && e.ctx.OnEmail != nil {
		e.ctx.OnEmail(handler)
	}
}

var _ EventSpine = (*ctxEventsAdapter)(nil)
