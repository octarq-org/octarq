package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// HostSession 封装租户与用户会话、角色门禁及身份鉴权
type HostSession interface {
	UserID(r *http.Request) uint
	OrgID(r *http.Request) uint
	OrgRole(r *http.Request) string
	RequireRole(r *http.Request, min string) bool
	RequirePerm(r *http.Request, permKey, minRole string) bool
	IsInstanceAdmin(r *http.Request) bool
	RevokeUserOrgSessions(userID, orgID uint) int
}

// CryptoVault 封装实例级密钥加解密信封
type CryptoVault interface {
	Encrypt(plaintext []byte) (string, error)
	Decrypt(encoded string) ([]byte, error)
}

// SettingsStore 封装工作区及全局持久化配置读写
type SettingsStore interface {
	GetWorkspaceSetting(orgID uint, key string) string
	SetWorkspaceSetting(orgID uint, key, value string) error
	GetGlobalSetting(key string) string
	SetGlobalSetting(key, value string) error
}

// WorkspaceSettings is an alias for SettingsStore for naming flexibility.
type WorkspaceSettings = SettingsStore

// EventSpine 封装事件发布、Webhook 声明及邮件事件响应
type EventSpine interface {
	PublishEvent(orgID uint, event string, data any)
	RegisterWebhookEvent(def WebhookEventDef)
	OnEmail(handler func(EmailEvent))
}

// Host 聚合宿主运行时核心能力
type Host interface {
	Session() HostSession
	Crypto() CryptoVault
	Settings() SettingsStore
	Events() EventSpine
	TenantDB(orgID uint) *TenantDB
	// HarnessSpec 返回该工作区已发布的 agent harness（规范 JSON 文档）。
	//
	// 返回 nil 文档表示工作区没有发布过 harness，调用方应使用内置默认值。
	// 用 JSON 而不是类型化返回值，是因为 harness 包依赖本包（plugin.Endpoint），
	// 直接引用它的类型会形成导入环；调用方用 harness.DecodeSpec 解码即可。
	//
	// 返回 error 表示已发布的 harness 读不出来 —— 调用方必须 fail closed，
	// 绝不能悄悄换一个 harness 继续跑。
	HarnessSpec(ctx context.Context, orgID uint) (json.RawMessage, error)
	// LookupCredential resolves and decrypts third-party integration credentials for orgID.
	// Returns ErrCredentialNotFound if no credential matches the query or if no credential
	// provider is mounted on the host (e.g. standalone Core OSS).
	LookupCredential(ctx context.Context, orgID uint, query CredentialQuery) (*Credential, error)
}

// Credential represents a decrypted third-party integration credential.
type Credential struct {
	ID           uint            `json:"id"`
	OrgID        uint            `json:"orgId"`
	ProviderType string          `json:"providerType"` // dns, communication, storage, payment, llm
	Provider     string          `json:"provider"`     // cloudflare, dnspod, smtp, resend, telegram, slack, etc.
	Name         string          `json:"name"`
	ConfigJSON   json.RawMessage `json:"configJson"` // Decrypted JSON configuration payload
	IsDefault    bool            `json:"isDefault"`
}

// CredentialQuery specifies lookup criteria for integration credentials.
type CredentialQuery struct {
	ID           uint   `json:"id,omitempty"`           // Explicit integration ID (optional)
	ProviderType string `json:"providerType,omitempty"` // Filter by provider type (e.g. "dns", "communication")
	Provider     string `json:"provider,omitempty"`     // Filter by provider name (e.g. "cloudflare", "slack")
}

// ErrCredentialNotFound indicates the requested credential does not exist.
var ErrCredentialNotFound = errors.New("credential not found")
