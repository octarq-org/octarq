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
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// The gateway needs a backend it can build without a vendor SDK, so the tests
// register two test-only factories: one that answers and one that refuses.
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

// httpFixture serves the gateway the way the composition root does: a huma API
// over a mux, with a middleware that stamps the caller's workspace and role onto
// the request — the part the platform auth middleware plays.
type httpFixture struct {
	plugin   *Plugin
	server   *httptest.Server
	provided map[string]any
	resolver func(uint) (llmprovider.Provider, error)
}

func newHTTPFixture(t *testing.T, userID, orgID uint, role string) *httpFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Provider{}, &ModelPrice{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test", "1.0"))
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		next(huma.WithContext(c, context.Background()))
	})

	f := &httpFixture{provided: map[string]any{}}
	session := &plugin.TestSession{
		UserIDFn:      func(*http.Request) uint { return userID },
		OrgIDFn:       func(*http.Request) uint { return orgID },
		RequireRoleFn: func(_ *http.Request, want string) bool { return role == want },
	}
	p := New()
	p.Mount(nil, &plugin.Context{
		Huma: api,
		DB:   db,
		Host: &plugin.TestHost{SessionMock: session, CryptoMock: &plugin.TestCrypto{}},
		Provide: func(name string, svc any) {
			f.provided[name] = svc
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

// TestHTTPProviderLifecycle walks the routes the settings page uses, so a
// regression here is a page that cannot configure a workspace.
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

	// The resolver the core chat path calls returns this workspace's backend.
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

// TestHTTPPricesRoundTrip pins the write route's refreshed body and the delete
// route: the page never re-reads the list after a save.
func TestHTTPPricesRoundTrip(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")

	status, saved := f.do(t, http.MethodPut, "/api/llm-gateway/prices",
		`{"model":"stub-model","input":3,"output":15,"reasoning":7.5}`)
	if status != http.StatusOK {
		t.Fatalf("set price = %d %v", status, saved)
	}
	prices, _ := saved["prices"].([]any)
	if len(prices) != 1 {
		t.Fatalf("prices = %v, want the saved row", saved["prices"])
	}
	if row, _ := prices[0].(map[string]any); row["input"].(float64) != 3 || row["reasoning"].(float64) != 7.5 {
		t.Fatalf("price = %v", row)
	}

	status, listed := f.do(t, http.MethodGet, "/api/llm-gateway/prices", "")
	if status != http.StatusOK {
		t.Fatalf("list prices = %d %v", status, listed)
	}
	if got, _ := listed["prices"].([]any); len(got) != 1 {
		t.Fatalf("prices = %v, want one", listed["prices"])
	}

	status, ok := f.do(t, http.MethodDelete, "/api/llm-gateway/prices?model=stub-model", "")
	if status != http.StatusOK || ok["ok"] != true {
		t.Fatalf("delete price = %d %v", status, ok)
	}
	_, listed = f.do(t, http.MethodGet, "/api/llm-gateway/prices", "")
	if got, _ := listed["prices"].([]any); len(got) != 0 {
		t.Fatalf("prices = %v, want none", listed["prices"])
	}

	// A model with no price is what a caller must be able to tell apart.
	if _, err := f.plugin.store.Price(context.Background(), 1, "stub-model"); !errors.Is(err, plugin.ErrModelUnpriced) {
		t.Fatalf("price after delete = %v, want ErrModelUnpriced", err)
	}
}

// TestHTTPBackendsAreTheRegisteredFactories pins the admin form's list: a name
// missing here cannot be typed in, and a name here cannot fail at save time.
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

// TestHTTPRefusals pins the status each refusal carries. The numbers matter: a
// 500 where the caller made a mistake is a bug report nobody can act on.
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
		// Org 2 cannot see, edit, or test org 1's row.
		other := newHTTPFixture(t, 1, 2, "admin")
		if status, body := other.do(t, http.MethodPut, "/api/llm-gateway/providers/1",
			`{"name":"x","backend":"`+testBackend+`"}`); status != http.StatusNotFound {
			t.Fatalf("cross-workspace update = %d %v, want 404", status, body)
		}
	})
}

// TestHTTPTestRouteReportsTheVendorRefusal pins the 502: a provider that refuses
// the ping is reachable, so the caller is told the vendor refused, not that the
// gateway broke.
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

// TestMountProvidesBothServices pins the seam the rest of the platform reads:
// a feature prices a lens through one service and resolves its backend through
// the other, and neither imports this package.
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

// TestMountWithoutAHostStillProvidesServices pins that the services exist before
// any provider is configured: a consumer gets a sentinel error, never a nil
// pointer.
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

// TestServiceAdaptersFailClosed pins the nil-plugin branch: a shared service
// value must refuse instead of dereferencing nothing.
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

// TestProviderFromRefusesWhatItCannotBuild pins the unseal path: a row whose key
// cannot be read is an invalid provider, not a provider with no key.
func TestProviderFromRefusesWhatItCannotBuild(t *testing.T) {
	// No host: no vault, so a stored key cannot be opened.
	bare := &Plugin{}
	if _, err := bare.providerFrom(&Provider{Backend: testBackend, APIKeyEncr: "sealed"}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("no vault = %v, want ErrInvalidProvider", err)
	}

	// A vault that refuses the ciphertext.
	refusing := &Plugin{pctx: &plugin.Context{Host: &plugin.TestHost{
		CryptoMock: &plugin.TestCrypto{DecryptFn: func(string) ([]byte, error) {
			return nil, errors.New("wrong key")
		}},
	}}}
	if _, err := refusing.providerFrom(&Provider{Backend: testBackend, APIKeyEncr: "sealed"}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("unseal failure = %v, want ErrInvalidProvider", err)
	}

	// A backend no factory serves.
	ok := &Plugin{pctx: &plugin.Context{Host: &plugin.TestHost{CryptoMock: &plugin.TestCrypto{}}}}
	if _, err := ok.providerFrom(&Provider{Backend: "no-such-backend"}); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("unknown backend = %v, want ErrInvalidProvider", err)
	}

	// A row with no key is valid: local backends need none.
	if _, err := ok.providerFrom(&Provider{Backend: testBackend}); err != nil {
		t.Fatalf("keyless provider: %v", err)
	}
}

// TestSealRefusesWithoutAVault pins that saving a key without secret storage is
// a refusal, not a plaintext column.
func TestSealRefusesWithoutAVault(t *testing.T) {
	if _, err := (&Plugin{}).seal("k"); err == nil {
		t.Fatal("seal without a vault succeeded")
	}
	withVault := &Plugin{pctx: &plugin.Context{Host: &plugin.TestHost{CryptoMock: &plugin.TestCrypto{}}}}
	got, err := withVault.seal("k")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if got != "k" {
		t.Fatalf("seal = %q", got)
	}
}

// TestDescribeAndEmbeddedDocs pins the plugin's identity and its help bundle:
// the gateway is core plumbing, and its pages ship with the binary.
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
	if got := p.Models(); len(got) != 2 {
		t.Fatalf("models = %v, want the provider and price tables", got)
	}
	if _, err := p.HelpDocsFS().Open("docs"); err != nil {
		t.Fatalf("docs are not embedded: %v", err)
	}
}

// TestHTTPRoutesReportAFailedQueryAs500 pins the last line of the error map: a
// database that cannot answer is the server's problem, and the caller is told so
// without the driver's message.
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
		{http.MethodGet, "/api/llm-gateway/prices", ""},
		{http.MethodPut, "/api/llm-gateway/prices", `{"model":"m","input":1,"output":2,"reasoning":3}`},
		{http.MethodDelete, "/api/llm-gateway/prices?model=m", ""},
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

// TestMountWithoutAContextIsANoOp pins that a mount with no context leaves the
// plugin usable: the help bundle and the identity do not need a host.
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
