package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type HostSession interface {
	UserID(r *http.Request) uint
	OrgID(r *http.Request) uint
	PersonalOrgID(r *http.Request) uint
	OrgRole(r *http.Request) string
	RequireRole(r *http.Request, min string) bool
	RequirePerm(r *http.Request, permKey, minRole string) bool
	IsInstanceAdmin(r *http.Request) bool
	RevokeUserOrgSessions(userID, orgID uint) int
}

type CryptoVault interface {
	Encrypt(plaintext []byte) (string, error)
	Decrypt(encoded string) ([]byte, error)
	EncryptWithAAD(plaintext, aad []byte) (string, error)
	DecryptWithAAD(encoded string, aad []byte) ([]byte, error)
	SubjectVault(ownerID uint, subject string) (CryptoVault, error)
	ShredSubjectKey(ownerID uint, subject string) error
	HasSubjectKey(ownerID uint, subject string) bool
}

type SettingsStore interface {
	GetWorkspaceSetting(orgID uint, key string) string
	SetWorkspaceSetting(orgID uint, key, value string) error
	GetGlobalSetting(key string) string
	SetGlobalSetting(key, value string) error
}

type WorkspaceSettings = SettingsStore

type EventSpine interface {
	PublishEvent(orgID uint, event string, data any)
	RegisterWebhookEvent(def WebhookEventDef)
	OnEmail(handler func(EmailEvent))
}

type Host interface {
	Session() HostSession
	Crypto() CryptoVault
	Settings() SettingsStore
	Events() EventSpine
	TenantDB(orgID uint) *TenantDB
	HarnessSpec(ctx context.Context, orgID uint) (json.RawMessage, error)
	LookupCredential(ctx context.Context, orgID uint, query CredentialQuery) (*Credential, error)
	Workforce() Workforce
}

type Credential struct {
	ID           uint            `json:"id"`
	OrgID        uint            `json:"orgId"`
	ProviderType string          `json:"providerType"`
	Provider     string          `json:"provider"`
	Name         string          `json:"name"`
	ConfigJSON   json.RawMessage `json:"configJson"`
	IsDefault    bool            `json:"isDefault"`
}

type CredentialQuery struct {
	ID           uint   `json:"id,omitempty"`
	ProviderType string `json:"providerType,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

var ErrCredentialNotFound = errors.New("credential not found")

var ErrCredentialResolverUnavailable = errors.New("credential resolver unavailable")
