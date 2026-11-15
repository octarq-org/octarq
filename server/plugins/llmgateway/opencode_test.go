package llmgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// doWithHeaders is the fixture request with headers, needed because the
// discovery document is addressed over a proxy in production.
func doWithHeaders(t *testing.T, f *httpFixture, method, path string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
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

// opencode fetches the manifest before it holds a token, so the document must
// answer without one — and it must name no workspace data, or the discovery
// step would leak what the workspace spends.
func TestOpencodeManifestAnswersWithoutAToken(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	status, doc := doWithHeaders(t, f, http.MethodGet, AgentWellKnownPath, nil)
	if status != http.StatusOK {
		t.Fatalf("manifest = %d %v", status, doc)
	}
	auth, _ := doc["auth"].(map[string]any)
	cmd, _ := auth["command"].([]any)
	if len(cmd) == 0 {
		t.Fatalf("auth.command must be argv, got %v", auth["command"])
	}
	if auth["env"] != opencodeTokenEnv {
		t.Fatalf("auth.env = %v, want %q", auth["env"], opencodeTokenEnv)
	}
	remote, _ := doc["remote_config"].(map[string]any)
	url, _ := remote["url"].(string)
	if !strings.HasSuffix(url, AgentOpencodeConfigPath) || !strings.HasPrefix(url, "http") {
		t.Fatalf("remote_config.url = %q, want an absolute URL ending in %s", url, AgentOpencodeConfigPath)
	}
	headers, _ := remote["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer {env:"+opencodeTokenEnv+"}" {
		t.Fatalf("remote_config.headers = %v, want the token sent as a bearer", headers)
	}
	// The provider, its models and its key stay out of the public document.
	for _, leak := range []string{"provider", "models", "baseURL"} {
		if _, found := doc[leak]; found {
			t.Fatalf("the public manifest must not carry %q", leak)
		}
	}
}

// The whole point of the config route: the model list is the provider's, so a
// model added in the console appears in opencode with no second edit.
func TestOpencodeConfigListsTheProviderModels(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://127.0.0.1:9", Model: "stub-model", CheapModel: "stub-model"})
	u := &upstream{listBody: `{"data":[{"id":"m-one"},{"id":"m-two"}]}`}
	withUpstream(t, f, u, id)

	status, body := f.do(t, http.MethodGet, AgentOpencodeConfigPath, "")
	if status != http.StatusOK {
		t.Fatalf("config = %d %v", status, body)
	}
	cfg, _ := body["config"].(map[string]any)
	providers, _ := cfg["provider"].(map[string]any)
	entry, _ := providers[opencodeProviderID].(map[string]any)
	if entry["npm"] != "@ai-sdk/openai-compatible" {
		t.Fatalf("npm = %v", entry["npm"])
	}
	if envs, _ := entry["env"].([]any); len(envs) != 1 || envs[0] != opencodeTokenEnv {
		t.Fatalf("env = %v, want [%s]", entry["env"], opencodeTokenEnv)
	}
	opts, _ := entry["options"].(map[string]any)
	if base, _ := opts["baseURL"].(string); !strings.HasSuffix(base, "/v1") {
		t.Fatalf("options.baseURL = %v, want <origin>/v1", opts["baseURL"])
	}
	models, _ := entry["models"].(map[string]any)
	if len(models) != 2 {
		t.Fatalf("models = %v, want m-one and m-two", models)
	}
	for _, want := range []string{"m-one", "m-two"} {
		if _, found := models[want]; !found {
			t.Fatalf("model %q is missing from %v", want, models)
		}
	}
}

// A workspace with no provider cannot serve an agent. Saying so beats handing
// opencode an empty provider it would show as usable.
func TestOpencodeConfigRefusesWithoutAProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	status, body := f.do(t, http.MethodGet, AgentOpencodeConfigPath, "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("config without a provider = %d %v", status, body)
	}
	if _, found := body["error"]; !found {
		t.Fatalf("want the OpenAI error shape, got %v", body)
	}
}

// A provider that refuses the list is reported, not papered over with names
// someone typed once.
func TestOpencodeConfigReportsAProviderRefusal(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://127.0.0.1:9", Model: "stub-model", CheapModel: "stub-model"})
	u := &upstream{listBody: `{"error":"nope"}`, listCode: http.StatusUnauthorized}
	withUpstream(t, f, u, id)

	status, body := f.do(t, http.MethodGet, AgentOpencodeConfigPath, "")
	if status != http.StatusBadGateway {
		t.Fatalf("config with a refusing provider = %d %v", status, body)
	}
}

// Behind a TLS-terminating proxy r.TLS is nil, so the document would advertise
// http and opencode would fail to reach the config. The forwarded scheme wins.
func TestOpencodeConfigUsesTheForwardedScheme(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "http://127.0.0.1:9", Model: "stub-model", CheapModel: "stub-model"})
	u := &upstream{listBody: `{"data":[{"id":"m-one"}]}`}
	withUpstream(t, f, u, id)

	status, body := doWithHeaders(t, f, http.MethodGet, AgentOpencodeConfigPath,
		map[string]string{"X-Forwarded-Proto": "https"})
	if status != http.StatusOK {
		t.Fatalf("config = %d %v", status, body)
	}
	cfg, _ := body["config"].(map[string]any)
	providers, _ := cfg["provider"].(map[string]any)
	entry, _ := providers[opencodeProviderID].(map[string]any)
	opts, _ := entry["options"].(map[string]any)
	base, _ := opts["baseURL"].(string)
	if !strings.HasPrefix(base, "https://") {
		t.Fatalf("options.baseURL = %q, want https", base)
	}
}
