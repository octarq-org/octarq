package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/vault"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	testBackend = "llmgatewaytest"
	failBackend = "llmgatewayfail"
)

type stubProvider struct {
	name string
	err  error
}

func (p stubProvider) Name() string         { return p.name }
func (p stubProvider) DefaultModel() string { return "stub-model" }
func (p stubProvider) CheapModel() string   { return "stub-cheap" }
func (p stubProvider) Complete(context.Context, llmprovider.Request) (llmprovider.Response, error) {
	if p.err != nil {
		return llmprovider.Response{}, p.err
	}
	return llmprovider.Response{Text: "OK", Model: "stub-answering"}, nil
}

func init() {
	llmprovider.Register(testBackend, func(llmprovider.Options) (llmprovider.Provider, error) {
		return stubProvider{name: testBackend}, nil
	})
	llmprovider.Register(failBackend, func(llmprovider.Options) (llmprovider.Provider, error) {
		return stubProvider{name: failBackend, err: errors.New("upstream refused")}, nil
	})
}

type httpFixture struct {
	plugin   *Plugin
	server   *httptest.Server
	provided map[string]any
	resolver func(uint) (llmprovider.Provider, error)
	db       *gorm.DB
}

type dbSecretStore struct{ db *gorm.DB }

func (s dbSecretStore) Get(key string) (string, bool) {
	var row models.Setting
	if s.db.First(&row, "key = ?", key).Error != nil {
		return "", false
	}
	return row.Value, true
}

func (s dbSecretStore) Set(key, val string) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.Setting{Key: key, Value: val}).Error
}

func (s dbSecretStore) Delete(key string) error {
	return s.db.Delete(&models.Setting{Key: key}).Error
}

func (s dbSecretStore) HasEncryptedValues() (bool, error) { return false, nil }

type cipherVaultAdapter struct{ c *crypto.Cipher }

func (r cipherVaultAdapter) Encrypt(p []byte) (string, error) { return r.c.Encrypt(p) }
func (r cipherVaultAdapter) Decrypt(s string) ([]byte, error) { return r.c.Decrypt(s) }
func (r cipherVaultAdapter) EncryptWithAAD(p, aad []byte) (string, error) {
	return r.c.EncryptWithAAD(p, aad)
}
func (r cipherVaultAdapter) DecryptWithAAD(s string, aad []byte) ([]byte, error) {
	return r.c.DecryptWithAAD(s, aad)
}
func (r cipherVaultAdapter) SubjectVault(ownerID uint, subject string) (plugin.CryptoVault, error) {
	sv, err := r.c.SubjectVault(ownerID, subject)
	if err != nil {
		return nil, err
	}
	return subjectVaultAdapter{sv: sv}, nil
}
func (r cipherVaultAdapter) ShredSubjectKey(ownerID uint, subject string) error {
	return r.c.ShredSubjectKey(ownerID, subject)
}
func (r cipherVaultAdapter) HasSubjectKey(ownerID uint, subject string) bool {
	return r.c.HasSubjectKey(ownerID, subject)
}

type subjectVaultAdapter struct{ sv *crypto.SubjectVault }

func (r subjectVaultAdapter) Encrypt(p []byte) (string, error) { return r.sv.Encrypt(p) }
func (r subjectVaultAdapter) Decrypt(s string) ([]byte, error) { return r.sv.Decrypt(s) }
func (r subjectVaultAdapter) EncryptWithAAD(p, aad []byte) (string, error) {
	return r.sv.EncryptWithAAD(p, aad)
}
func (r subjectVaultAdapter) DecryptWithAAD(s string, aad []byte) ([]byte, error) {
	return r.sv.DecryptWithAAD(s, aad)
}
func (r subjectVaultAdapter) SubjectVault(ownerID uint, subject string) (plugin.CryptoVault, error) {
	sv, err := r.sv.SubjectVault(ownerID, subject)
	if err != nil {
		return nil, err
	}
	return subjectVaultAdapter{sv: sv}, nil
}
func (r subjectVaultAdapter) ShredSubjectKey(ownerID uint, subject string) error {
	return r.sv.ShredSubjectKey(ownerID, subject)
}
func (r subjectVaultAdapter) HasSubjectKey(ownerID uint, subject string) bool {
	return r.sv.HasSubjectKey(ownerID, subject)
}

type stubVaultService struct {
	getSecretFn func(ctx context.Context, orgID, itemID uint) (*plugin.VaultSecret, error)
}

func (s *stubVaultService) PutSecret(context.Context, plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	return nil, errors.New("stub")
}
func (s *stubVaultService) UpdateSecret(context.Context, uint, uint, *string, map[string]any, map[string]any, *bool, string, uint) (*plugin.VaultItemMeta, error) {
	return nil, errors.New("stub")
}
func (s *stubVaultService) GetMeta(context.Context, uint, uint) (*plugin.VaultItemMeta, error) {
	return nil, errors.New("stub")
}
func (s *stubVaultService) GetSecret(ctx context.Context, orgID, itemID uint) (*plugin.VaultSecret, error) {
	if s.getSecretFn != nil {
		return s.getSecretFn(ctx, orgID, itemID)
	}
	return nil, errors.New("stub")
}
func (s *stubVaultService) ResolveByRef(context.Context, uint, string) (*plugin.VaultSecret, error) {
	return nil, errors.New("stub")
}
func (s *stubVaultService) ResolveInstanceSecret(context.Context, string) (*plugin.VaultSecret, error) {
	return nil, errors.New("stub")
}
func (s *stubVaultService) PutInstanceSecret(context.Context, plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	return nil, errors.New("stub")
}
func (s *stubVaultService) DeleteInstanceSecret(context.Context, string) error { return nil }
func (s *stubVaultService) ListItems(context.Context, uint, plugin.VaultFilter) ([]plugin.VaultItemMeta, error) {
	return nil, nil
}
func (s *stubVaultService) DeleteSecret(context.Context, uint, uint) error { return nil }
func (s *stubVaultService) ComputeTOTP(context.Context, uint, uint) (string, int, error) {
	return "", 0, errors.New("stub")
}

func newTestVaultService(t *testing.T, db *gorm.DB) plugin.VaultService {
	t.Helper()
	vp := vault.New()
	if err := db.AutoMigrate(append([]any{&models.Setting{}}, vp.Models()...)...); err != nil {
		t.Fatalf("migrate vault: %v", err)
	}
	c := crypto.New("llmgateway-test-secret")
	if err := c.EnableEnvelope(dbSecretStore{db: db}); err != nil {
		t.Fatalf("enable envelope: %v", err)
	}
	reg := plugin.NewRegistry()
	vp.Mount(nil, &plugin.Context{
		DB:      db,
		Host:    &plugin.TestHost{CryptoMock: cipherVaultAdapter{c: c}},
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	})
	vs, _ := plugin.LookupServiceAs[plugin.VaultService](reg.Lookup, plugin.ServiceVault)
	return vs
}

func newHTTPFixture(t *testing.T, userID, orgID uint, role string) *httpFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Provider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test", "1.0"))
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		next(huma.WithContext(c, context.Background()))
	})

	f := &httpFixture{provided: map[string]any{}, db: db}
	vs := newTestVaultService(t, db)
	f.provided[plugin.ServiceVault] = vs

	session := &plugin.TestSession{
		UserIDFn:      func(*http.Request) uint { return userID },
		OrgIDFn:       func(*http.Request) uint { return orgID },
		RequireRoleFn: func(_ *http.Request, want string) bool { return role == want },
	}
	p := New()
	p.Mount(mux, &plugin.Context{
		Huma: api,
		DB:   db,
		Host: &plugin.TestHost{SessionMock: session},

		Guard: func(next http.Handler) http.Handler { return next },
		Provide: func(name string, svc any) {
			f.provided[name] = svc
		},
		Lookup: func(name string) (any, bool) {
			v, ok := f.provided[name]
			return v, ok
		},
		SetLLMResolverForOrg: func(r func(uint) (llmprovider.Provider, error)) {
			f.resolver = r
		},
	})
	f.plugin = p

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.server = srv
	return f
}

func (f *httpFixture) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestHTTPProviderLifecycle(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")

	status, list := f.do(t, http.MethodGet, "/api/llm-gateway/providers", "")
	if status != http.StatusOK {
		t.Fatalf("list = %d %v", status, list)
	}
	if got, _ := list["providers"].([]any); len(got) != 0 {
		t.Fatalf("providers = %v, want an empty list", list["providers"])
	}

	status, created := f.do(t, http.MethodPost, "/api/llm-gateway/providers",
		`{"name":"Claude","backend":"`+testBackend+`","apiKey":"secret","model":"stub"}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v", status, created)
	}
	provider, _ := created["provider"].(map[string]any)
	if provider["isDefault"] != true {
		t.Fatalf("created = %v, want the first provider to be the default", provider)
	}
	if provider["hasKey"] != true {
		t.Fatalf("created = %v, want hasKey", provider)
	}
	if _, leaked := provider["apiKey"]; leaked {
		t.Fatalf("created = %v, want the key withheld", provider)
	}
	id := uint(provider["id"].(float64))

	status, updated := f.do(t, http.MethodPut, "/api/llm-gateway/providers/1",
		`{"name":"Claude renamed","backend":"`+testBackend+`"}`)
	if status != http.StatusOK {
		t.Fatalf("update = %d %v", status, updated)
	}
	if got, _ := updated["provider"].(map[string]any); got["name"] != "Claude renamed" {
		t.Fatalf("update = %v, want the new name", updated)
	}

	status, tested := f.do(t, http.MethodPost, "/api/llm-gateway/providers/1/test", "")
	if status != http.StatusOK {
		t.Fatalf("test = %d %v", status, tested)
	}
	if tested["ok"] != true || tested["model"] != "stub-answering" {
		t.Fatalf("test = %v, want the answering model", tested)
	}

	status, ok := f.do(t, http.MethodPost, "/api/llm-gateway/providers/1/default", "")
	if status != http.StatusOK || ok["ok"] != true {
		t.Fatalf("default = %d %v", status, ok)
	}

	if f.resolver == nil {
		t.Fatal("Mount did not register the LLM resolver")
	}
	prov, err := f.resolver(1)
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if prov.Name() != testBackend {
		t.Fatalf("resolver backend = %q, want %q", prov.Name(), testBackend)
	}

	status, deleted := f.do(t, http.MethodDelete, "/api/llm-gateway/providers/"+strconv.FormatUint(uint64(id), 10), "")
	if status != http.StatusOK || deleted["ok"] != true {
		t.Fatalf("delete = %d %v", status, deleted)
	}
	status, list = f.do(t, http.MethodGet, "/api/llm-gateway/providers", "")
	if status != http.StatusOK {
		t.Fatalf("list after delete = %d %v", status, list)
	}
	if got, _ := list["providers"].([]any); len(got) != 0 {
		t.Fatalf("providers = %v, want none", list["providers"])
	}
}

func TestHTTPPricesAreReadOnly(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")

	status, listed := f.do(t, http.MethodGet, "/api/llm-gateway/prices", "")
	if status != http.StatusOK {
		t.Fatalf("list prices = %d %v", status, listed)
	}
	prices, _ := listed["prices"].([]any)
	if len(prices) == 0 {
		t.Fatalf("prices = %v, want the bundled catalog", listed["prices"])
	}
	found := false
	for _, raw := range prices {
		row, _ := raw.(map[string]any)
		deprecatedKey := "cached" + "Input"
		if _, ok := row[deprecatedKey]; ok {
			t.Fatalf("row holds %s: %v", deprecatedKey, row)
		}
		if row["model"] != "claude-opus-4-8" || row["providerId"] != "anthropic" {
			continue
		}
		found = true
		if row["input"].(float64) != 5 || row["output"].(float64) != 25 {
			t.Fatalf("row = %v, want input 5 and output 25", row)
		}
		if row["cacheRead"].(float64) != 0.5 {
			t.Fatalf("row = %v, want cacheRead 0.5", row)
		}
		if row["cacheWrite"].(float64) != 6.25 {
			t.Fatalf("row = %v, want cacheWrite 6.25", row)
		}
		if row["displayName"] != "Claude Opus 4.8" {
			t.Fatalf("row = %v, want displayName Claude Opus 4.8", row)
		}
	}
	if !found {
		t.Fatalf("prices = %v, want claude-opus-4-8", listed["prices"])
	}

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		if status, body := f.do(t, method, "/api/llm-gateway/prices", `{"model":"m","input":1}`); status == http.StatusOK {
			t.Fatalf("%s prices = %d %v, want no write route", method, status, body)
		}
	}

	if _, err := f.plugin.store.Price(context.Background(), 1, "stub-model"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("unknown model price = %v, want ErrModelUnpriced", err)
	}
}

func TestHTTPBackendsAreTheRegisteredFactories(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")

	status, out := f.do(t, http.MethodGet, "/api/llm-gateway/backends", "")
	if status != http.StatusOK {
		t.Fatalf("backends = %d %v", status, out)
	}
	names, _ := out["backends"].([]any)
	found := false
	for _, name := range names {
		if name == testBackend {
			found = true
		}
	}
	if !found {
		t.Fatalf("backends = %v, want %q", names, testBackend)
	}
}

func TestHTTPRefusals(t *testing.T) {
	t.Run("signed out", func(t *testing.T) {
		f := newHTTPFixture(t, 0, 1, "admin")
		if status, body := f.do(t, http.MethodGet, "/api/llm-gateway/providers", ""); status != http.StatusUnauthorized {
			t.Fatalf("list = %d %v, want 401", status, body)
		}
	})

	t.Run("not an admin", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		if status, body := f.do(t, http.MethodGet, "/api/llm-gateway/providers", ""); status != http.StatusForbidden {
			t.Fatalf("list = %d %v, want 403", status, body)
		}
	})

	t.Run("no workspace", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 0, "admin")
		if status, body := f.do(t, http.MethodGet, "/api/llm-gateway/providers", ""); status != http.StatusBadRequest {
			t.Fatalf("list = %d %v, want 400", status, body)
		}
	})

	t.Run("unknown backend", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "admin")
		status, body := f.do(t, http.MethodPost, "/api/llm-gateway/providers",
			`{"name":"x","backend":"no-such-backend"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("create = %d %v, want 400", status, body)
		}
	})

	t.Run("missing provider", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "admin")
		for _, tc := range []struct {
			method string
			path   string
			body   string
		}{
			{http.MethodPut, "/api/llm-gateway/providers/99", `{"name":"x","backend":"` + testBackend + `"}`},
			{http.MethodDelete, "/api/llm-gateway/providers/99", ""},
			{http.MethodPost, "/api/llm-gateway/providers/99/default", ""},
			{http.MethodPost, "/api/llm-gateway/providers/99/test", ""},
		} {
			if status, body := f.do(t, tc.method, tc.path, tc.body); status != http.StatusNotFound {
				t.Fatalf("%s %s = %d %v, want 404", tc.method, tc.path, status, body)
			}
		}
	})

	t.Run("another workspace's provider", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "admin")
		if status, body := f.do(t, http.MethodPost, "/api/llm-gateway/providers",
			`{"name":"x","backend":"`+testBackend+`"}`); status != http.StatusCreated {
			t.Fatalf("create = %d %v", status, body)
		}

		other := newHTTPFixture(t, 1, 2, "admin")
		if status, body := other.do(t, http.MethodPut, "/api/llm-gateway/providers/1",
			`{"name":"x","backend":"`+testBackend+`"}`); status != http.StatusNotFound {
			t.Fatalf("cross-workspace update = %d %v, want 404", status, body)
		}
	})
}

func TestHTTPTestRouteReportsTheVendorRefusal(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")
	status, created := f.do(t, http.MethodPost, "/api/llm-gateway/providers",
		`{"name":"broken","backend":"`+failBackend+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v", status, created)
	}
	status, body := f.do(t, http.MethodPost, "/api/llm-gateway/providers/1/test", "")
	if status != http.StatusBadGateway {
		t.Fatalf("test = %d %v, want 502", status, body)
	}
}

func TestMountProvidesBothServices(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")

	pricing, ok := f.provided[plugin.ServiceModelPricing]
	if !ok {
		t.Fatalf("services = %v, want %q", keys(f.provided), plugin.ServiceModelPricing)
	}
	gateway, ok := f.provided[plugin.ServiceLLMProvider]
	if !ok {
		t.Fatalf("services = %v, want %q", keys(f.provided), plugin.ServiceLLMProvider)
	}
	if _, ok := pricing.(plugin.ModelPricing); !ok {
		t.Fatalf("pricing service is %T, want plugin.ModelPricing", pricing)
	}
	if _, ok := gateway.(plugin.LLMGateway); !ok {
		t.Fatalf("gateway service is %T, want plugin.LLMGateway", gateway)
	}
}

func TestMountWithoutAHostStillProvidesServices(t *testing.T) {
	provided := map[string]any{}
	p := New()
	p.Mount(nil, &plugin.Context{Provide: func(name string, svc any) { provided[name] = svc }})

	pricing, _ := provided[plugin.ServiceModelPricing].(plugin.ModelPricing)
	if _, err := pricing.Price(context.Background(), 1, "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("price without a store = %v, want ErrPricingUnavailable", err)
	}
	gw, _ := provided[plugin.ServiceLLMProvider].(plugin.LLMGateway)
	if _, err := gw.Provider(context.Background(), 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("provider without a store = %v, want ErrNotConfigured", err)
	}
}

func TestServiceAdaptersFailClosed(t *testing.T) {
	if _, err := (pricingService{}).Price(context.Background(), 1, "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("pricing = %v, want ErrPricingUnavailable", err)
	}
	if _, err := (pricingService{plugin: &Plugin{}}).Price(context.Background(), 1, "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("pricing without a store = %v, want ErrPricingUnavailable", err)
	}
	if _, err := (gatewayService{}).Provider(context.Background(), 1); !errors.Is(err, plugin.ErrLLMGatewayUnavailable) {
		t.Fatalf("gateway = %v, want ErrLLMGatewayUnavailable", err)
	}
	if _, err := (&Plugin{}).Resolve(context.Background(), 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("resolve without a store = %v, want ErrNotConfigured", err)
	}
}

func TestProviderFromRefusesWhatItCannotBuild(t *testing.T) {
	ctx := context.Background()

	bare := &Plugin{}
	if _, err := bare.providerFrom(ctx, &Provider{OrgID: 1, Backend: testBackend, VaultItemID: 1}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("no vault = %v, want ErrInvalidProvider", err)
	}

	refusing := &Plugin{pctx: &plugin.Context{
		Lookup: func(name string) (any, bool) {
			if name == plugin.ServiceVault {
				return &stubVaultService{getSecretFn: func(context.Context, uint, uint) (*plugin.VaultSecret, error) {
					return nil, errors.New("wrong key")
				}}, true
			}
			return nil, false
		},
	}}
	if _, err := refusing.providerFrom(ctx, &Provider{OrgID: 1, Backend: testBackend, VaultItemID: 1}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("unseal failure = %v, want ErrInvalidProvider", err)
	}

	ok := &Plugin{}
	if _, err := ok.providerFrom(ctx, &Provider{Backend: "no-such-backend"}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("unknown backend = %v, want ErrInvalidProvider", err)
	}

	if _, err := ok.providerFrom(ctx, &Provider{Backend: testBackend}); err != nil {
		t.Fatalf("keyless provider: %v", err)
	}
}

func TestSealRefusesWithoutAVault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Provider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := NewStore(db)
	if _, err := s.CreateProvider(context.Background(), 1, ProviderInput{Name: "A", Backend: testBackend, APIKey: "k"}, nil); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("create without vault = %v, want ErrInvalidProvider", err)
	}
	vs := newTestVaultService(t, db)
	row, err := s.CreateProvider(context.Background(), 1, ProviderInput{Name: "A", Backend: testBackend, APIKey: "k"}, vs)
	if err != nil {
		t.Fatalf("create with vault: %v", err)
	}
	if row.VaultItemID == 0 {
		t.Fatal("expected VaultItemID > 0")
	}
}

func TestDescribeAndEmbeddedDocs(t *testing.T) {
	p := New()
	if p.Name() != "llmgateway" {
		t.Fatalf("name = %q", p.Name())
	}
	info := p.Describe()
	if !info.Core || !info.EnabledByDefault {
		t.Fatalf("info = %+v, want core and enabled", info)
	}
	if info.Category != plugin.CategoryAI {
		t.Fatalf("category = %q, want %q", info.Category, plugin.CategoryAI)
	}
	if got := p.Models(); len(got) != 1 {
		t.Fatalf("models = %v, want the provider table", got)
	}
	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("validate = %v, want the embedded catalog to parse", err)
	}
	if _, err := p.HelpDocsFS().Open("docs"); err != nil {
		t.Fatalf("docs are not embedded: %v", err)
	}
}

func TestHTTPRoutesReportAFailedQueryAs500(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")
	sqlDB, err := f.plugin.store.db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/llm-gateway/providers", ""},
		{http.MethodPost, "/api/llm-gateway/providers", `{"name":"x","backend":"` + testBackend + `"}`},
		{http.MethodPut, "/api/llm-gateway/providers/1", `{"name":"x","backend":"` + testBackend + `"}`},
		{http.MethodDelete, "/api/llm-gateway/providers/1", ""},
		{http.MethodPost, "/api/llm-gateway/providers/1/default", ""},
		{http.MethodPost, "/api/llm-gateway/providers/1/test", ""},
	} {
		status, body := f.do(t, tc.method, tc.path, tc.body)
		if status != http.StatusInternalServerError {
			t.Fatalf("%s %s on a closed db = %d %v, want 500", tc.method, tc.path, status, body)
		}
		if msg, _ := body["message"].(string); strings.Contains(msg, "sql") || strings.Contains(msg, "closed") {
			t.Fatalf("%s %s message = %q, want the generic text, not the driver's", tc.method, tc.path, msg)
		}
	}
}

func TestMountWithoutAContextIsANoOp(t *testing.T) {
	p := New()
	p.Mount(nil, nil)
	if p.store != nil {
		t.Fatal("mount with no context built a store")
	}
	if p.Name() != "llmgateway" {
		t.Fatalf("name = %q", p.Name())
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
