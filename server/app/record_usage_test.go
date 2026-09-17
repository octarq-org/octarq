package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/octarq-org/octarq/server/app"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/plugin"
)

func getHostRuntimePctxFields(t *testing.T) map[string]bool {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller path")
	}
	hostRuntimePath := filepath.Join(filepath.Dir(currentFile), "host_runtime.go")

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, hostRuntimePath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse host_runtime.go: %v", err)
	}

	fields := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		isPluginContext := false
		if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "plugin" && sel.Sel.Name == "Context" {
				isPluginContext = true
			}
		}

		if isPluginContext {
			for _, elt := range compLit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if keyIdent, ok := kv.Key.(*ast.Ident); ok {
						fields[keyIdent.Name] = true
					}
				}
			}
		}
		return true
	})

	return fields
}

func verifyAppGoUsesUnifiedConstructor(t *testing.T) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller path")
	}
	appGoPath := filepath.Join(filepath.Dir(currentFile), "app.go")

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, appGoPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse app.go: %v", err)
	}

	// 1. Ensure no raw &plugin.Context composite literals remain in app.go
	pctxInAppGo := 0
	ast.Inspect(node, func(n ast.Node) bool {
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "plugin" && sel.Sel.Name == "Context" {
				pctxInAppGo++
			}
		}
		return true
	})
	if pctxInAppGo > 0 {
		t.Fatalf("expected 0 direct plugin.Context literals in app.go (must use unified constructor), found %d", pctxInAppGo)
	}

	// 2. Ensure both Run and RunMCP call buildPluginContext
	buildPluginContextCalls := 0
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "buildPluginContext" {
				buildPluginContextCalls++
			}
		}
		return true
	})
	if buildPluginContextCalls < 2 {
		t.Fatalf("expected at least 2 buildPluginContext calls in app.go (Run and RunMCP), found %d", buildPluginContextCalls)
	}
}

func TestBothPctxConstructorsSetRecordUsage(t *testing.T) {
	verifyAppGoUsesUnifiedConstructor(t)

	fields := getHostRuntimePctxFields(t)
	if !fields["RecordUsage"] {
		t.Fatal("expected unified plugin.Context constructor in host_runtime.go to set RecordUsage")
	}
}

func TestRecordUsageLazyResolution(t *testing.T) {
	reg := plugin.NewRegistry()
	var capturedOrgID uint
	var capturedMetric string
	var capturedN int64

	recordUsageFn := func(orgID uint, metric string, n int64) {
		// Lazily resolved: the same LookupServiceAs path app.go's RecordUsage
		// closure uses, through the named contract type.
		if fn, ok := plugin.LookupServiceAs[plugin.UsageMeter](reg.Lookup, plugin.ServiceCloudUsage); ok {
			fn(orgID, metric, n)
		}
	}

	// 1. Call before provide -> no-op, no panic
	recordUsageFn(1, "links", 1)
	if capturedOrgID != 0 {
		t.Fatal("expected no-op when cloud.usage is not provided")
	}

	// 2. Register service after plugin mount, converted to the named contract
	// type exactly as the Pro cloud module provider does.
	reg.Provide(plugin.ServiceCloudUsage, plugin.UsageMeter(func(orgID uint, metric string, n int64) {
		capturedOrgID = orgID
		capturedMetric = metric
		capturedN = n
	}))

	// 3. Call after provide -> resolves lazily and invokes target
	recordUsageFn(42, "links", 5)
	if capturedOrgID != 42 || capturedMetric != "links" || capturedN != 5 {
		t.Fatalf("lazy resolution failed: got org=%d metric=%s n=%d", capturedOrgID, capturedMetric, capturedN)
	}
}

func TestBothPctxConstructorsSetLLMResolverForOrg(t *testing.T) {
	verifyAppGoUsesUnifiedConstructor(t)

	fields := getHostRuntimePctxFields(t)
	if !fields["SetLLMResolverForOrg"] {
		t.Fatal("expected unified plugin.Context constructor in host_runtime.go to set SetLLMResolverForOrg")
	}
}

func TestBothPctxConstructorsSetRegisterNotificationChannel(t *testing.T) {
	verifyAppGoUsesUnifiedConstructor(t)

	fields := getHostRuntimePctxFields(t)
	if !fields["RegisterNotificationChannel"] {
		t.Fatal("expected unified plugin.Context constructor in host_runtime.go to set RegisterNotificationChannel")
	}
}

type mockSecretStore struct {
	m map[string]string
}

func (s *mockSecretStore) Get(key string) (string, bool) {
	v, ok := s.m[key]
	return v, ok
}

func (s *mockSecretStore) Set(key, val string) error {
	s.m[key] = val
	return nil
}

func TestUnifiedPluginContextRuntimeCompleteness(t *testing.T) {
	var a app.App
	c := crypto.New("12345678901234567890123456789012")
	if err := c.EnableEnvelope(&mockSecretStore{m: make(map[string]string)}); err != nil {
		t.Fatalf("crypto.EnableEnvelope failed: %v", err)
	}

	var emailMu sync.Mutex
	var deferredEmail []func(plugin.EmailEvent)

	// Test 1: HTTP mode
	httpCtx := a.BuildPluginContext(app.PluginContextTestParams{
		Cipher:          c,
		Services:        plugin.NewRegistry(),
		OnEmailMu:       &emailMu,
		DeferredOnEmail: &deferredEmail,
		HttpMode:        true,
	})

	if httpCtx.Host == nil {
		t.Fatal("expected httpCtx.Host to be non-nil")
	}
	if httpCtx.Host.Session() == nil {
		t.Fatal("expected httpCtx.Host.Session() to be non-nil")
	}
	if httpCtx.Host.Crypto() == nil {
		t.Fatal("expected httpCtx.Host.Crypto() to be non-nil")
	}
	if httpCtx.Host.Settings() == nil {
		t.Fatal("expected httpCtx.Host.Settings() to be non-nil")
	}
	if httpCtx.Host.Events() == nil {
		t.Fatal("expected httpCtx.Host.Events() to be non-nil")
	}

	if httpCtx.RecordUsage == nil {
		t.Fatal("expected httpCtx.RecordUsage to be non-nil")
	}
	if httpCtx.SetLLMResolverForOrg == nil {
		t.Fatal("expected httpCtx.SetLLMResolverForOrg to be non-nil")
	}
	if httpCtx.RegisterNotificationChannel == nil {
		t.Fatal("expected httpCtx.RegisterNotificationChannel to be non-nil")
	}
	if httpCtx.Host == nil || httpCtx.Host.Session() == nil {
		t.Fatal("expected httpCtx.Host.Session to be non-nil")
	}
	if httpCtx.PluginActive == nil || httpCtx.FeatureActive == nil || httpCtx.ActivePlugins == nil {
		t.Fatal("expected plugin/feature active closures to be non-nil in HTTP mode")
	}

	// Verify crypto facade delegation
	ciphertext, err := httpCtx.Host.Crypto().Encrypt([]byte("secret-token"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	plaintext, err := httpCtx.Host.Crypto().Decrypt(ciphertext)
	if err != nil || string(plaintext) != "secret-token" {
		t.Fatalf("Decrypt via Host.Crypto failed: got %q, %v", string(plaintext), err)
	}

	// Verify Session contracts with nil auth/api
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if httpCtx.Host.Session().UserID(req) != 0 {
		t.Fatal("expected UserID 0 with nil auth")
	}
	if httpCtx.Host.Session().OrgID(req) != 0 {
		t.Fatal("expected OrgID 0 with nil auth")
	}
	if httpCtx.Host.Session().OrgRole(req) != "" {
		t.Fatal("expected OrgRole empty with nil api")
	}
	if httpCtx.Host.Session().RequireRole(req, "admin") {
		t.Fatal("expected RequireRole false with nil api")
	}
	if httpCtx.Host.Session().RequirePerm(req, "p", "admin") {
		t.Fatal("expected RequirePerm false with nil api")
	}
	if httpCtx.Host.Session().IsInstanceAdmin(req) {
		t.Fatal("expected IsInstanceAdmin false with nil api")
	}
	if httpCtx.Host.Session().RevokeUserOrgSessions(1, 1) != 0 {
		t.Fatal("expected RevokeUserOrgSessions 0 with nil auth")
	}

	// Verify Settings contracts with nil apiHandler
	if httpCtx.Host.Settings().GetWorkspaceSetting(1, "key") != "" {
		t.Fatal("expected empty workspace setting with nil api")
	}
	if err := httpCtx.Host.Settings().SetWorkspaceSetting(1, "key", "val"); err == nil {
		t.Fatal("expected error setting workspace setting with nil api")
	}
	if httpCtx.Host.Settings().GetGlobalSetting("key") != "" {
		t.Fatal("expected empty global setting with nil api")
	}
	if err := httpCtx.Host.Settings().SetGlobalSetting("key", "val"); err == nil {
		t.Fatal("expected error setting global setting with nil api")
	}

	// Verify Events contracts
	httpCtx.Host.Events().PublishEvent(1, "test.event", map[string]any{"ok": true})
	httpCtx.Host.Events().RegisterWebhookEvent(plugin.WebhookEventDef{
		Key:         "user.created",
		Group:       "user",
		Title:       "User Created",
		Description: "Triggered on user creation",
	})
	httpCtx.Host.Events().OnEmail(nil) // nil handler no-op
	var receivedEmail bool
	httpCtx.Host.Events().OnEmail(func(e plugin.EmailEvent) {
		receivedEmail = true
	})
	if len(deferredEmail) == 0 {
		t.Fatal("expected deferred email handler to be registered")
	}
	deferredEmail[0](plugin.EmailEvent{})
	if !receivedEmail {
		t.Fatal("expected deferred email handler invocation")
	}

	// Verify OnEmail through services registry
	servicesWithMail := plugin.NewRegistry()
	var dispatchedEmail bool
	servicesWithMail.Provide(plugin.ServiceMailDispatcher, plugin.EmailDispatcher(func(handler func(plugin.EmailEvent)) {
		dispatchedEmail = true
	}))
	ctxWithMail := a.BuildPluginContext(app.PluginContextTestParams{
		Cipher:   c,
		Services: servicesWithMail,
	})
	ctxWithMail.Host.Events().OnEmail(func(e plugin.EmailEvent) {})
	if !dispatchedEmail {
		t.Fatal("expected OnEmail to dispatch via ServiceMailDispatcher")
	}

	// Verify Crypto with nil cipher returns error
	ctxNoCipher := a.BuildPluginContext(app.PluginContextTestParams{})
	if _, err := ctxNoCipher.Host.Crypto().Encrypt([]byte("test")); err == nil {
		t.Fatal("expected error encrypting with nil cipher")
	}
	if _, err := ctxNoCipher.Host.Crypto().Decrypt("invalid"); err == nil {
		t.Fatal("expected error decrypting with nil cipher")
	}

	// Test 2: MCP mode
	mcpCtx := a.BuildPluginContext(app.PluginContextTestParams{
		Cipher:          c,
		Services:        plugin.NewRegistry(),
		OnEmailMu:       &emailMu,
		DeferredOnEmail: &deferredEmail,
		HttpMode:        false,
	})

	if mcpCtx.Host == nil {
		t.Fatal("expected mcpCtx.Host to be non-nil")
	}
	if mcpCtx.RecordUsage == nil {
		t.Fatal("expected mcpCtx.RecordUsage to be non-nil")
	}
	if mcpCtx.SetLLMResolverForOrg == nil {
		t.Fatal("expected mcpCtx.SetLLMResolverForOrg to be non-nil")
	}
	if mcpCtx.RegisterNotificationChannel == nil {
		t.Fatal("expected mcpCtx.RegisterNotificationChannel to be non-nil")
	}
	// MCP mode contract: PluginActive, FeatureActive, ActivePlugins must stay nil
	if mcpCtx.PluginActive != nil || mcpCtx.FeatureActive != nil || mcpCtx.ActivePlugins != nil {
		t.Fatal("expected plugin/feature active closures to stay nil in MCP mode")
	}
}
