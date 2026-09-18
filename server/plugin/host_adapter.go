package plugin

import (
	"errors"
	"net/http"
)

// EnsureHost extracts ctx.Host. If ctx is nil or ctx.Host is nil,
// it returns a safe no-op Host adapter so callers never encounter a nil Host.
func EnsureHost(ctx *Context) Host {
	if ctx != nil && ctx.Host != nil {
		return ctx.Host
	}
	return &noopHost{}
}

type noopHost struct{}

func (noopHost) Session() HostSession    { return noopSession{} }
func (noopHost) Crypto() CryptoVault     { return nil }
func (noopHost) Settings() SettingsStore { return noopSettings{} }
func (noopHost) Events() EventSpine      { return noopEvents{} }
func (noopHost) TenantDB(uint) *TenantDB { return nil }

type noopSession struct{}

func (noopSession) UserID(*http.Request) uint                      { return 0 }
func (noopSession) OrgID(*http.Request) uint                       { return 0 }
func (noopSession) OrgRole(*http.Request) string                   { return "" }
func (noopSession) RequireRole(*http.Request, string) bool         { return false }
func (noopSession) RequirePerm(*http.Request, string, string) bool { return false }
func (noopSession) IsInstanceAdmin(*http.Request) bool             { return false }
func (noopSession) RevokeUserOrgSessions(uint, uint) int           { return 0 }

type noopSettings struct{}

func (noopSettings) GetWorkspaceSetting(uint, string) string { return "" }
func (noopSettings) SetWorkspaceSetting(uint, string, string) error {
	return errors.New("settings store unavailable")
}
func (noopSettings) GetGlobalSetting(string) string { return "" }
func (noopSettings) SetGlobalSetting(string, string) error {
	return errors.New("settings store unavailable")
}

type noopEvents struct{}

func (noopEvents) PublishEvent(uint, string, any)       {}
func (noopEvents) RegisterWebhookEvent(WebhookEventDef) {}
func (noopEvents) OnEmail(func(EmailEvent))             {}

var (
	_ Host          = (*noopHost)(nil)
	_ HostSession   = (*noopSession)(nil)
	_ SettingsStore = (*noopSettings)(nil)
	_ EventSpine    = (*noopEvents)(nil)
)
