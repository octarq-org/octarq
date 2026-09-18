package plugin

import "net/http"

// TestHost is a configurable Host implementation for unit and integration testing.
type TestHost struct {
	SessionMock  HostSession
	CryptoMock   CryptoVault
	SettingsMock SettingsStore
	EventsMock   EventSpine
	TenantDBMock func(orgID uint) *TenantDB
}

func (h *TestHost) Session() HostSession {
	if h != nil && h.SessionMock != nil {
		return h.SessionMock
	}
	return noopSession{}
}

func (h *TestHost) Crypto() CryptoVault {
	if h != nil {
		return h.CryptoMock
	}
	return nil
}

func (h *TestHost) Settings() SettingsStore {
	if h != nil && h.SettingsMock != nil {
		return h.SettingsMock
	}
	return noopSettings{}
}

func (h *TestHost) Events() EventSpine {
	if h != nil && h.EventsMock != nil {
		return h.EventsMock
	}
	return noopEvents{}
}

func (h *TestHost) TenantDB(orgID uint) *TenantDB {
	if h != nil && h.TenantDBMock != nil {
		return h.TenantDBMock(orgID)
	}
	return nil
}

var _ Host = (*TestHost)(nil)

// TestSession is a configurable HostSession implementation for testing.
type TestSession struct {
	UserIDFn          func(*http.Request) uint
	OrgIDFn           func(*http.Request) uint
	OrgRoleFn         func(*http.Request) string
	RequireRoleFn     func(*http.Request, string) bool
	RequirePermFn     func(*http.Request, string, string) bool
	IsInstanceAdminFn func(*http.Request) bool
	RevokeSessionsFn  func(uint, uint) int
}

func (s *TestSession) UserID(r *http.Request) uint {
	if s != nil && s.UserIDFn != nil {
		return s.UserIDFn(r)
	}
	return 0
}

func (s *TestSession) OrgID(r *http.Request) uint {
	if s != nil && s.OrgIDFn != nil {
		return s.OrgIDFn(r)
	}
	return 0
}

func (s *TestSession) OrgRole(r *http.Request) string {
	if s != nil && s.OrgRoleFn != nil {
		return s.OrgRoleFn(r)
	}
	return ""
}

func (s *TestSession) RequireRole(r *http.Request, min string) bool {
	if s != nil && s.RequireRoleFn != nil {
		return s.RequireRoleFn(r, min)
	}
	return true
}

func (s *TestSession) RequirePerm(r *http.Request, permKey, minRole string) bool {
	if s != nil && s.RequirePermFn != nil {
		return s.RequirePermFn(r, permKey, minRole)
	}
	return true
}

func (s *TestSession) IsInstanceAdmin(r *http.Request) bool {
	if s != nil && s.IsInstanceAdminFn != nil {
		return s.IsInstanceAdminFn(r)
	}
	return false
}

func (s *TestSession) RevokeUserOrgSessions(userID, orgID uint) int {
	if s != nil && s.RevokeSessionsFn != nil {
		return s.RevokeSessionsFn(userID, orgID)
	}
	return 0
}

var _ HostSession = (*TestSession)(nil)

// TestCrypto is a configurable CryptoVault implementation for testing.
type TestCrypto struct {
	EncryptFn func([]byte) (string, error)
	DecryptFn func(string) ([]byte, error)
}

func (c *TestCrypto) Encrypt(plaintext []byte) (string, error) {
	if c != nil && c.EncryptFn != nil {
		return c.EncryptFn(plaintext)
	}
	return string(plaintext), nil
}

func (c *TestCrypto) Decrypt(encoded string) ([]byte, error) {
	if c != nil && c.DecryptFn != nil {
		return c.DecryptFn(encoded)
	}
	return []byte(encoded), nil
}

var _ CryptoVault = (*TestCrypto)(nil)

// TestSettings is a configurable SettingsStore implementation for testing.
type TestSettings struct {
	GetWorkspaceSettingFn func(orgID uint, key string) string
	SetWorkspaceSettingFn func(orgID uint, key, value string) error
	GetGlobalSettingFn    func(key string) string
	SetGlobalSettingFn    func(key, value string) error
}

func (s *TestSettings) GetWorkspaceSetting(orgID uint, key string) string {
	if s != nil && s.GetWorkspaceSettingFn != nil {
		return s.GetWorkspaceSettingFn(orgID, key)
	}
	return ""
}

func (s *TestSettings) SetWorkspaceSetting(orgID uint, key, value string) error {
	if s != nil && s.SetWorkspaceSettingFn != nil {
		return s.SetWorkspaceSettingFn(orgID, key, value)
	}
	return nil
}

func (s *TestSettings) GetGlobalSetting(key string) string {
	if s != nil && s.GetGlobalSettingFn != nil {
		return s.GetGlobalSettingFn(key)
	}
	return ""
}

func (s *TestSettings) SetGlobalSetting(key, value string) error {
	if s != nil && s.SetGlobalSettingFn != nil {
		return s.SetGlobalSettingFn(key, value)
	}
	return nil
}

var _ SettingsStore = (*TestSettings)(nil)

// TestEvents is a configurable EventSpine implementation for testing.
type TestEvents struct {
	PublishEventFn         func(orgID uint, event string, data any)
	RegisterWebhookEventFn func(WebhookEventDef)
	OnEmailFn              func(func(EmailEvent))
}

func (e *TestEvents) PublishEvent(orgID uint, event string, data any) {
	if e != nil && e.PublishEventFn != nil {
		e.PublishEventFn(orgID, event, data)
	}
}

func (e *TestEvents) RegisterWebhookEvent(def WebhookEventDef) {
	if e != nil && e.RegisterWebhookEventFn != nil {
		e.RegisterWebhookEventFn(def)
	}
}

func (e *TestEvents) OnEmail(handler func(EmailEvent)) {
	if e != nil && e.OnEmailFn != nil {
		e.OnEmailFn(handler)
	}
}

var _ EventSpine = (*TestEvents)(nil)
