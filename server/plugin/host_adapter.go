package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

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
func (noopHost) Hosts() HostRegistry     { return NoopHostRegistry() }

func (noopHost) HarnessSpec(context.Context, uint) (json.RawMessage, error) { return nil, nil }

func (noopHost) LookupCredential(context.Context, uint, CredentialQuery) (*Credential, error) {
	return nil, ErrCredentialResolverUnavailable
}

func (noopHost) SpendLedger() SpendLedger { return unavailableSpendLedger{} }

type unavailableSpendLedger struct{}

func (unavailableSpendLedger) AuthorizeTurn(context.Context, uint, string, string) error {
	return ErrSpendUnavailable
}

func (unavailableSpendLedger) TurnCost(context.Context, uint, string) (func(input, output, cacheRead, cacheWrite int) float64, error) {
	return nil, ErrSpendUnavailable
}

func (unavailableSpendLedger) RecordSpend(context.Context, uint, TurnSpend) (*SpendRecord, error) {
	return nil, ErrSpendUnavailable
}

func (unavailableSpendLedger) IsVirtualKey(string) bool { return false }

func (unavailableSpendLedger) ResolveVirtualKey(context.Context, string) (*SpendAccountAuth, error) {
	return nil, ErrSpendUnavailable
}

type noopSession struct{}

func (noopSession) UserID(*http.Request) uint                      { return 0 }
func (noopSession) OrgID(*http.Request) uint                       { return 0 }
func (noopSession) PersonalOrgID(*http.Request) uint               { return 0 }
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
