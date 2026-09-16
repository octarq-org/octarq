package plugin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

type mockHostSession struct {
	userID          uint
	orgID           uint
	orgRole         string
	isInstanceAdmin bool
}

func (m *mockHostSession) UserID(r *http.Request) uint                     { return m.userID }
func (m *mockHostSession) OrgID(r *http.Request) uint                      { return m.orgID }
func (m *mockHostSession) OrgRole(r *http.Request) string                  { return m.orgRole }
func (m *mockHostSession) RequireRole(r *http.Request, min string) bool    { return true }
func (m *mockHostSession) RequirePerm(r *http.Request, p, min string) bool { return true }
func (m *mockHostSession) IsInstanceAdmin(r *http.Request) bool            { return m.isInstanceAdmin }
func (m *mockHostSession) RevokeUserOrgSessions(userID, orgID uint) int    { return 1 }

var _ plugin.HostSession = (*mockHostSession)(nil)

type mockCryptoVault struct{}

func (m *mockCryptoVault) Encrypt(plaintext []byte) (string, error) {
	return "enc:" + string(plaintext), nil
}
func (m *mockCryptoVault) Decrypt(encoded string) ([]byte, error) {
	return []byte(encoded[4:]), nil
}

var _ plugin.CryptoVault = (*mockCryptoVault)(nil)

type mockSettingsStore struct {
	wsSettings     map[string]string
	globalSettings map[string]string
}

func (m *mockSettingsStore) GetWorkspaceSetting(orgID uint, key string) string {
	return m.wsSettings[key]
}
func (m *mockSettingsStore) SetWorkspaceSetting(orgID uint, key, value string) error {
	m.wsSettings[key] = value
	return nil
}
func (m *mockSettingsStore) GetGlobalSetting(key string) string {
	return m.globalSettings[key]
}
func (m *mockSettingsStore) SetGlobalSetting(key, value string) error {
	m.globalSettings[key] = value
	return nil
}

var _ plugin.SettingsStore = (*mockSettingsStore)(nil)

type mockEventSpine struct {
	published []string
	events    []plugin.WebhookEventDef
	handlers  []func(plugin.EmailEvent)
}

func (m *mockEventSpine) PublishEvent(orgID uint, event string, data any) {
	m.published = append(m.published, event)
}
func (m *mockEventSpine) RegisterWebhookEvent(def plugin.WebhookEventDef) {
	m.events = append(m.events, def)
}
func (m *mockEventSpine) OnEmail(handler func(plugin.EmailEvent)) {
	m.handlers = append(m.handlers, handler)
}

var _ plugin.EventSpine = (*mockEventSpine)(nil)

type mockHost struct {
	session  plugin.HostSession
	crypto   plugin.CryptoVault
	settings plugin.SettingsStore
	events   plugin.EventSpine
}

func (m *mockHost) Session() plugin.HostSession    { return m.session }
func (m *mockHost) Crypto() plugin.CryptoVault     { return m.crypto }
func (m *mockHost) Settings() plugin.SettingsStore { return m.settings }
func (m *mockHost) Events() plugin.EventSpine      { return m.events }

var _ plugin.Host = (*mockHost)(nil)

func TestHostInterfaces(t *testing.T) {
	sess := &mockHostSession{userID: 10, orgID: 20, orgRole: "admin", isInstanceAdmin: true}
	crypto := &mockCryptoVault{}
	settings := &mockSettingsStore{
		wsSettings:     make(map[string]string),
		globalSettings: make(map[string]string),
	}
	events := &mockEventSpine{}

	host := &mockHost{
		session:  sess,
		crypto:   crypto,
		settings: settings,
		events:   events,
	}

	pctx := &plugin.Context{
		Host: host,
	}

	if pctx.Host == nil {
		t.Fatal("expected pctx.Host to be non-nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if uid := pctx.Host.Session().UserID(req); uid != 10 {
		t.Fatalf("expected userID 10, got %d", uid)
	}
	if oid := pctx.Host.Session().OrgID(req); oid != 20 {
		t.Fatalf("expected orgID 20, got %d", oid)
	}
	if role := pctx.Host.Session().OrgRole(req); role != "admin" {
		t.Fatalf("expected orgRole 'admin', got %q", role)
	}
	if !pctx.Host.Session().IsInstanceAdmin(req) {
		t.Fatal("expected isInstanceAdmin true")
	}

	enc, err := pctx.Host.Crypto().Encrypt([]byte("hello"))
	if err != nil || enc != "enc:hello" {
		t.Fatalf("unexpected encrypt result: %q, %v", enc, err)
	}
	dec, err := pctx.Host.Crypto().Decrypt(enc)
	if err != nil || string(dec) != "hello" {
		t.Fatalf("unexpected decrypt result: %q, %v", string(dec), err)
	}

	_ = pctx.Host.Settings().SetWorkspaceSetting(20, "theme", "dark")
	if v := pctx.Host.Settings().GetWorkspaceSetting(20, "theme"); v != "dark" {
		t.Fatalf("expected workspace setting 'dark', got %q", v)
	}
	_ = pctx.Host.Settings().SetGlobalSetting("title", "octarq")
	if v := pctx.Host.Settings().GetGlobalSetting("title"); v != "octarq" {
		t.Fatalf("expected global setting 'octarq', got %q", v)
	}

	pctx.Host.Events().PublishEvent(20, "test.event", nil)
	if len(events.published) != 1 || events.published[0] != "test.event" {
		t.Fatalf("expected published event 'test.event', got %+v", events.published)
	}
}
