package plugin

import (
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
}
