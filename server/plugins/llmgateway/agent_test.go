package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// upstream is a stand-in provider. It records what the gateway sent and answers
// with whatever the test asked for.
type upstream struct {
	mu       sync.Mutex
	requests []recorded
	listBody string
	listCode int
	chatBody string
	chatCode int
}

// recorded is one call the gateway made.
type recorded struct {
	path   string
	auth   string
	apiKey string
	body   string
	length int64
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	u.requests = append(u.requests, recorded{
		path: r.URL.Path, auth: r.Header.Get("Authorization"),
		apiKey: r.Header.Get("x-api-key"), body: string(body), length: r.ContentLength,
	})
	u.mu.Unlock()
	if r.URL.Path == "/models" {
		code := u.listCode
		if code == 0 {
			code = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, u.listBody)
		return
	}
	code := u.chatCode
	if code == 0 {
		code = http.StatusOK
	}
	if ct := r.Header.Get("Accept"); strings.Contains(ct, "event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("X-Upstream", "yes")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, u.chatBody)
}

func (u *upstream) last(t *testing.T) recorded {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.requests) == 0 {
		t.Fatal("the gateway called no upstream")
	}
	return u.requests[len(u.requests)-1]
}

// withUpstream points the fixture at a stand-in provider.
func withUpstream(t *testing.T, f *httpFixture, u *upstream, providerID uint) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	p := f.plugin
	if _, err := p.store.UpdateProvider(context.Background(), 1, providerID, ProviderInput{
		Name: "stub", Backend: "openai", APIKey: "k-test", BaseURL: srv.URL,
		Model: "stub-model", CheapModel: "stub-model",
	}, p.seal); err != nil {
		t.Fatalf("update provider: %v", err)
	}
	return srv
}

// createProvider writes one provider row through the same path the API uses.
func createProvider(t *testing.T, f *httpFixture, in ProviderInput) uint {
	t.Helper()
	row, err := f.plugin.store.CreateProvider(context.Background(), 1, in, f.plugin.seal)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return row.ID
}

// TestAgentModelsComesFromTheProvider proves the model list is the provider's
// answer, not a string a person typed. A coding agent reads this list and picks
// from it.
func TestAgentModelsComesFromTheProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test",
		BaseURL: "https://placeholder.invalid", Model: "stub-model"})
	u := &upstream{listBody: `{"object":"list","data":[{"id":"m-one","owned_by":"vendor"},{"id":"m-two"}]}`}
	withUpstream(t, f, u, id)

	status, body := f.do(t, http.MethodGet, "/v1/models", "")
	if status != http.StatusOK {
		t.Fatalf("models = %d %v", status, body)
	}
	data, _ := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("models = %v, want two", body["data"])
	}
	first, _ := data[0].(map[string]any)
	if first["id"] != "m-one" || first["object"] != "model" {
		t.Fatalf("first model = %v", first)
	}
	if got := u.last(t); got.path != "/models" || got.auth != "Bearer k-test" {
		t.Fatalf("upstream saw %+v", got)
	}
}

// TestAgentModelsReadsTheOllamaShape keeps a local model server usable without
// a translation layer: Ollama answers {"models":[...]}.
func TestAgentModelsReadsTheOllamaShape(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "local", Backend: "ollama", BaseURL: "http://placeholder.invalid"})
	u := &upstream{listBody: `{"models":[{"name":"llama3.2"},{"id":"qwen2.5"}]}`}
	withUpstream(t, f, u, id)

	status, body := f.do(t, http.MethodGet, "/v1/models", "")
	if status != http.StatusOK {
		t.Fatalf("models = %d %v", status, body)
	}
	data, _ := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("models = %v", body["data"])
	}
	got, _ := data[1].(map[string]any)
	if got["id"] != "qwen2.5" {
		t.Fatalf("second model = %v", got)
	}
	if auth := u.last(t).auth; auth != "Bearer k-test" {
		t.Fatalf("auth header = %q", auth)
	}
}

// TestAgentModelsReportsAnUpstreamRefusal keeps a broken provider visible.
func TestAgentModelsReportsAnUpstreamRefusal(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k", BaseURL: "http://placeholder.invalid"})
	u := &upstream{listCode: http.StatusUnauthorized, listBody: `{"error":"bad key"}`}
	withUpstream(t, f, u, id)

	status, body := f.do(t, http.MethodGet, "/v1/models", "")
	if status != http.StatusBadGateway {
		t.Fatalf("models = %d %v, want 502", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if !strings.Contains(fmt.Sprint(errBody["message"]), "bad key") {
		t.Fatalf("error = %v", body)
	}
}

// TestAgentModelsRefusesARegistryWithNoModels keeps an unparsable answer from
// turning into an empty picker.
func TestAgentModelsRefusesARegistryWithNoModels(t *testing.T) {
	for name, body := range map[string]string{
		"empty list": `{"data":[]}`,
		"not json":   `nope`,
		"no ids":     `{"data":[{"id":""}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newHTTPFixture(t, 1, 1, "member")
			id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", BaseURL: "http://placeholder.invalid"})
			u := &upstream{listBody: body}
			withUpstream(t, f, u, id)
			status, _ := f.do(t, http.MethodGet, "/v1/models", "")
			if status != http.StatusBadGateway {
				t.Fatalf("models = %d, want 502", status)
			}
		})
	}
}

// TestAgentChatRelaysTheTurnByteForByte proves tools, streaming flags and
// response_format survive: the body is forwarded, not re-encoded.
func TestAgentChatRelaysTheTurnByteForByte(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test", BaseURL: "http://placeholder.invalid"})
	u := &upstream{chatBody: `{"choices":[{"message":{"content":"hi"}}]}`}
	withUpstream(t, f, u, id)

	req := `{"model":"m-one","stream":false,"tools":[{"type":"function"}],"response_format":{"type":"json_object"},"messages":[{"role":"user","content":"hello"}]}`
	status, body := f.do(t, http.MethodPost, "/v1/chat/completions", req)
	if status != http.StatusOK {
		t.Fatalf("chat = %d %v", status, body)
	}
	got := u.last(t)
	if got.path != "/chat/completions" || got.auth != "Bearer k-test" {
		t.Fatalf("upstream saw %+v", got)
	}
	if got.body != req {
		t.Fatalf("the body changed:\n sent %s\n got  %s", req, got.body)
	}
}

// TestAgentChatKeepsTheStreamingAnswer keeps a coding agent's incremental
// output incremental: the upstream stream reaches the client untouched.
func TestAgentChatKeepsTheStreamingAnswer(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k-test", BaseURL: "http://placeholder.invalid"})
	u := &upstream{chatBody: "data: {\"delta\":\"a\"}\n\ndata: [DONE]\n\n"}
	withUpstream(t, f, u, id)

	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true}`))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat = %d %s", resp.StatusCode, raw)
	}
	if string(raw) != u.chatBody {
		t.Fatalf("the stream changed: %q", raw)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	if resp.Header.Get("X-Upstream") != "yes" {
		t.Fatal("an upstream header was dropped")
	}
	// The length was declared, so the provider reads a plain body; a backend
	// that reads Content-Length only would otherwise see an empty turn.
	if got := u.last(t); got.length <= 0 || got.length != int64(len(`{"model":"m","stream":true}`)) {
		t.Fatalf("forwarded Content-Length = %d", got.length)
	}
}

// TestAgentRefusesWithoutAProvider is the fail-closed state: no provider means
// no model, never a default.
func TestAgentRefusesWithoutAProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, AgentModelsPath, ""},
		{http.MethodPost, AgentChatPath, `{}`},
	} {
		status, body := f.do(t, c.method, c.path, c.body)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("%s = %d %v", c.path, status, body)
		}
		errBody, _ := body["error"].(map[string]any)
		if !strings.Contains(fmt.Sprint(errBody["message"]), "no model provider") {
			t.Fatalf("%s error = %v", c.path, body)
		}
	}
}

// TestAgentRefusesABackendWithNoOpenAISurface keeps a native-API backend from
// being guessed at.
func TestAgentRefusesABackendWithNoOpenAISurface(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "g", Backend: "gemini", APIKey: "k", Model: "gemini-pro"})

	status, body := f.do(t, http.MethodGet, "/v1/models", "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("models = %d %v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if !strings.Contains(fmt.Sprint(errBody["message"]), "gemini") {
		t.Fatalf("error = %v", body)
	}
}

// TestAgentProviderSelector covers the ?provider= id and the workspace bound it
// must stay inside.
func TestAgentProviderSelector(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	u := &upstream{listBody: `{"data":[{"id":"m"}]}`}
	first := createProvider(t, f, ProviderInput{Name: "one", Backend: "openai", BaseURL: "http://placeholder.invalid"})
	second := createProvider(t, f, ProviderInput{Name: "two", Backend: "openai", BaseURL: "http://placeholder.invalid"})
	_ = first
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	if _, err := f.plugin.store.UpdateProvider(context.Background(), 1, second, ProviderInput{
		Name: "two", Backend: "openai", APIKey: "k-two", BaseURL: srv.URL,
	}, f.plugin.seal); err != nil {
		t.Fatalf("update: %v", err)
	}

	status, _ := f.do(t, http.MethodGet, fmt.Sprintf("/v1/models?provider=%d", second), "")
	if status != http.StatusOK || u.last(t).auth != "Bearer k-two" {
		t.Fatalf("selector: %d %+v", status, u.last(t))
	}
	if status, _ := f.do(t, http.MethodGet, "/v1/models?provider=abc", ""); status != http.StatusBadRequest {
		t.Fatalf("bad selector = %d, want 400", status)
	}
	if status, _ := f.do(t, http.MethodGet, "/v1/models?provider=9999", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("foreign provider = %d, want 503", status)
	}
}

// TestAgentNeedsASession keeps an unauthenticated composition from reading the
// provider table.
func TestAgentNeedsASession(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	f.plugin.pctx = nil
	status, _ := f.do(t, http.MethodGet, "/v1/models", "")
	if status != http.StatusInternalServerError {
		t.Fatalf("no session = %d, want 500", status)
	}
}

// TestProviderModelsRoute is the console's picker source: an admin reads one
// provider's models, and nobody else does.
func TestProviderModelsRoute(t *testing.T) {
	admin := newHTTPFixture(t, 1, 1, "admin")
	id := createProvider(t, admin, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k", BaseURL: "http://placeholder.invalid"})
	u := &upstream{listBody: `{"data":[{"id":"m-one"}]}`}
	withUpstream(t, admin, u, id)

	status, body := admin.do(t, http.MethodPost, "/api/llm-gateway/models", fmt.Sprintf(`{"providerId":%d}`, id))
	if status != http.StatusOK {
		t.Fatalf("provider models = %d %v", status, body)
	}
	models, _ := body["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %v", body["models"])
	}
	if u.last(t).auth != "Bearer k-test" {
		t.Fatalf("the stored key was not used: %q", u.last(t).auth)
	}

	member := newHTTPFixture(t, 2, 1, "member")
	if status, _ := member.do(t, http.MethodPost, "/api/llm-gateway/models", fmt.Sprintf(`{"providerId":%d}`, id)); status != http.StatusForbidden {
		t.Fatalf("member = %d, want 403", status)
	}

	// Another workspace names no such provider.
	other := newHTTPFixture(t, 3, 2, "admin")
	if status, _ := other.do(t, http.MethodPost, "/api/llm-gateway/models", fmt.Sprintf(`{"providerId":%d}`, id)); status != http.StatusNotFound {
		t.Fatalf("other workspace = %d, want 404", status)
	}
}

// TestProviderModelsDraft covers the form before it is saved: the console asks
// for the list with the values on screen.
func TestProviderModelsDraft(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")
	u := &upstream{listBody: `{"data":[{"id":"m-draft"}]}`}
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)

	// A provider that does not exist yet: the form alone resolves it.
	status, body := f.do(t, http.MethodPost, "/api/llm-gateway/models",
		fmt.Sprintf(`{"backend":"openai","baseUrl":%q,"apiKey":"typed"}`, srv.URL))
	if status != http.StatusOK {
		t.Fatalf("draft = %d %v", status, body)
	}
	if u.last(t).auth != "Bearer typed" {
		t.Fatalf("the typed key was not used: %q", u.last(t).auth)
	}

	// The form's edits override the stored row, so the list matches the screen.
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k",
		BaseURL: "http://placeholder.invalid"})
	status, body = f.do(t, http.MethodPost, "/api/llm-gateway/models",
		fmt.Sprintf(`{"providerId":%d,"baseUrl":%q,"apiKey":"typed-two"}`, id, srv.URL))
	if status != http.StatusOK {
		t.Fatalf("edited row = %d %v", status, body)
	}
	if u.last(t).auth != "Bearer typed-two" {
		t.Fatalf("the edit was not applied: %q", u.last(t).auth)
	}

	// A form with no saved row and no backend names no provider.
	if status, _ := f.do(t, http.MethodPost, "/api/llm-gateway/models", `{"baseUrl":"http://127.0.0.1:1"}`); status != http.StatusBadRequest {
		t.Fatalf("no backend = %d, want 400", status)
	}

	// A backend with no OpenAI surface is refused before any call is made.
	if status, _ := f.do(t, http.MethodPost, "/api/llm-gateway/models", `{"backend":"gemini"}`); status != http.StatusBadRequest {
		t.Fatalf("gemini = %d, want 400", status)
	}
}

// TestProviderModelsRouteReportsATransportFailure covers the branch where the
// provider cannot be reached at all.
func TestProviderModelsRouteReportsATransportFailure(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "admin")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k",
		BaseURL: "http://127.0.0.1:1"})
	status, _ := f.do(t, http.MethodPost, "/api/llm-gateway/models",
		fmt.Sprintf(`{"providerId":%d}`, id))
	if status != http.StatusBadGateway {
		t.Fatalf("unreachable provider = %d, want 502", status)
	}
}

// TestAgentRoutesAbsentWithoutAGuard keeps a test composition from exposing the
// provider key path by accident.
func TestAgentRoutesAbsentWithoutAGuard(t *testing.T) {
	mux := http.NewServeMux()
	p := New()
	// A composition without the token guard: the app always supplies one, and a
	// test double must not expose the credential path.
	p.Mount(mux, &plugin.Context{Provide: func(string, any) {}})
	status := probe(t, mux, http.MethodGet, "/v1/models")
	if status != http.StatusNotFound {
		t.Fatalf("unguarded mount served the agent API: %d", status)
	}
}

// probe sends one request through a bare mux.
func probe(t *testing.T, mux http.Handler, method, path string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// TestOpenAIBase covers every backend the gateway accepts.
func TestOpenAIBase(t *testing.T) {
	cases := []struct {
		row     Provider
		want    string
		wantErr bool
	}{
		{row: Provider{BaseURL: "https://x.example/v1/"}, want: "https://x.example/v1"},
		{row: Provider{Backend: "openai"}, want: "https://api.openai.com/v1"},
		{row: Provider{Backend: "ollama"}, want: "http://localhost:11434/v1"},
		{row: Provider{Backend: "claude"}, wantErr: true},
		{row: Provider{Backend: "gemini"}, wantErr: true},
		{row: Provider{Backend: "mistral"}, wantErr: true},
		{row: Provider{Backend: "cohere"}, wantErr: true},
		{row: Provider{Backend: "weird"}, wantErr: true},
	}
	for _, c := range cases {
		got, err := openAIBase(&c.row)
		if c.wantErr {
			if err == nil {
				t.Fatalf("%s: no error", c.row.Backend)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("%s = %q, %v; want %q", c.row.Backend, got, err, c.want)
		}
	}
}

// TestSignSendsTheCredentialTheBackendReads covers the bearer scheme and the
// provider that has no key.
func TestSignSendsTheCredentialTheBackendReads(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	agentProvider{}.sign(req)
	if req.Header.Get("Authorization") != "" {
		t.Fatal("a provider without a key sent a credential")
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	agentProvider{APIKey: "k", Backend: "openai"}.sign(req)
	if req.Header.Get("Authorization") != "Bearer k" {
		t.Fatalf("openai header = %q", req.Header.Get("Authorization"))
	}
}

// TestAgentErrorsAreOpenAIShaped keeps a client's error parser working.
func TestAgentErrorsAreOpenAIShaped(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAgentError(rec, http.StatusBadRequest, "invalid_request_error", "bad")
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["message"] != "bad" || errBody["type"] != "invalid_request_error" {
		t.Fatalf("body = %v", body)
	}
}

// errTestUnseal models a key the host cannot open.
var errTestUnseal = errors.New("test: the key does not open")

// --- resolution and transport branches -------------------------------------

// TestAgentProviderResolutionFailures keeps every refusal legible: no store, no
// key storage, and a row whose key cannot be opened.
func TestAgentProviderResolutionFailures(t *testing.T) {
	t.Run("no store", func(t *testing.T) {
		p := New()
		if _, err := p.agentProviderFor(context.Background(), 1, 0); err != ErrNotConfigured {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no secret storage", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		createProvider(t, f, ProviderInput{Name: "s", Backend: "openai", APIKey: "k"})
		f.plugin.pctx.Host = &plugin.TestHost{SessionMock: f.plugin.pctx.Host.Session()}
		if _, err := f.plugin.agentProviderFor(context.Background(), 1, 0); err == nil ||
			!strings.Contains(err.Error(), "secret storage") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unopenable key", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		createProvider(t, f, ProviderInput{Name: "s", Backend: "openai", APIKey: "k"})
		f.plugin.pctx.Host = &plugin.TestHost{
			SessionMock: f.plugin.pctx.Host.Session(),
			CryptoMock: &plugin.TestCrypto{
				DecryptFn: func(string) ([]byte, error) { return nil, errTestUnseal },
			},
		}
		if _, err := f.plugin.agentProviderFor(context.Background(), 1, 0); err == nil ||
			!strings.Contains(err.Error(), "unseal") {
			t.Fatalf("err = %v", err)
		}
	})
}

// TestAgentRejectsAnInvalidBaseURL covers the branch where the stored base URL
// is not a URL at all.
func TestAgentRejectsAnInvalidBaseURL(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "s", Backend: "openai", APIKey: "k", BaseURL: "http://bad host"})
	if status, _ := f.do(t, http.MethodGet, AgentModelsPath, ""); status != http.StatusBadGateway {
		t.Fatalf("models = %d, want 502", status)
	}
	if status, _ := f.do(t, http.MethodPost, AgentChatPath, `{}`); status != http.StatusBadRequest {
		t.Fatalf("chat = %d, want 400", status)
	}
}

// TestAgentChatReportsAnUnreachableProvider keeps a dead provider from looking
// like a client mistake.
func TestAgentChatReportsAnUnreachableProvider(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	createProvider(t, f, ProviderInput{Name: "s", Backend: "openai", APIKey: "k", BaseURL: "http://127.0.0.1:1"})
	status, body := f.do(t, http.MethodPost, AgentChatPath, `{"model":"m"}`)
	if status != http.StatusBadGateway {
		t.Fatalf("chat = %d %v", status, body)
	}
}

// TestAgentModelsReportsATruncatedAnswer covers a provider that closes the
// connection mid-body.
func TestAgentModelsReportsATruncatedAnswer(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "s", Backend: "openai", APIKey: "k"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "64")
		_, _ = io.WriteString(w, `{"data":[`)
	}))
	t.Cleanup(srv.Close)
	if _, err := f.plugin.store.UpdateProvider(context.Background(), 1, id, ProviderInput{
		Name: "s", Backend: "openai", APIKey: "k", BaseURL: srv.URL,
	}, f.plugin.seal); err != nil {
		t.Fatalf("update: %v", err)
	}
	if status, _ := f.do(t, http.MethodGet, AgentModelsPath, ""); status != http.StatusBadGateway {
		t.Fatalf("models = %d, want 502", status)
	}
}

// TestAgentRefusesAWorkspaceTheTokenDoesNotName covers the session that carries
// no workspace.
func TestAgentRefusesAWorkspaceTheTokenDoesNotName(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	f.plugin.pctx.Host = &plugin.TestHost{SessionMock: &plugin.TestSession{OrgIDFn: func(*http.Request) uint { return 0 }}}
	status, _ := f.do(t, http.MethodGet, AgentModelsPath, "")
	if status != http.StatusForbidden {
		t.Fatalf("no workspace = %d, want 403", status)
	}
}

// TestUpstreamMessageKeepsAnErrorShort covers both truncation and an empty body.
func TestUpstreamMessageKeepsAnErrorShort(t *testing.T) {
	if got := upstreamMessage(500, nil); got != "HTTP 500" {
		t.Fatalf("empty = %q", got)
	}
	long := strings.Repeat("x", 400)
	got := upstreamMessage(500, []byte(long))
	if len(got) > 320 || !strings.HasPrefix(got, "HTTP 500: xxx") {
		t.Fatalf("long = %d chars", len(got))
	}
}

// failingWriter is a client that went away.
type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header       { return w.header }
func (w *failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (w *failingWriter) WriteHeader(int)           {}

// bareWriter has no Flusher, which is a legitimate ResponseWriter.
type bareWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func (w *bareWriter) Header() http.Header         { return w.header }
func (w *bareWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *bareWriter) WriteHeader(code int)        { w.code = code }

// TestRelayCopiesWithoutAFlusherAndStopsOnAWriteError covers both relay exits.
func TestRelayCopiesWithoutAFlusherAndStopsOnAWriteError(t *testing.T) {
	up := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}, "Connection": {"close"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
	bare := &bareWriter{header: http.Header{}}
	relay(bare, up)
	if bare.body.String() != `{"ok":true}` || bare.code != http.StatusOK {
		t.Fatalf("bare relay = %q %d", bare.body.String(), bare.code)
	}
	if bare.header.Get("Connection") != "" {
		t.Fatal("a hop-by-hop header was relayed")
	}

	up.Body = io.NopCloser(strings.NewReader("more"))
	relay(&failingWriter{header: http.Header{}}, up)
}

// TestAgentModelsSkipsNamelessEntries covers a provider that lists an entry with
// neither an id nor a name. Such an entry names no model, so it is dropped.
func TestAgentModelsSkipsNamelessEntries(t *testing.T) {
	f := newHTTPFixture(t, 1, 1, "member")
	id := createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k",
		BaseURL: "http://placeholder.invalid"})
	u := &upstream{listBody: `{"models":[{"name":""},{"name":"kept"}]}`}
	withUpstream(t, f, u, id)

	status, body := f.do(t, http.MethodGet, "/v1/models", "")
	if status != http.StatusOK {
		t.Fatalf("models = %d %v", status, body)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data = %v", body["data"])
	}
	if entry, _ := data[0].(map[string]any); entry["id"] != "kept" {
		t.Fatalf("entry = %v", data[0])
	}
}

// TestAgentChatRefusesAnUnusableRequest covers the three ways a chat request
// stops before the provider is called.
func TestAgentChatRefusesAnUnusableRequest(t *testing.T) {
	t.Run("no session", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		f.plugin.pctx = nil
		if status, _ := f.do(t, http.MethodPost, "/v1/chat/completions", "{}"); status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", status)
		}
	})
	t.Run("bad selector", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		if status, _ := f.do(t, http.MethodPost, "/v1/chat/completions?provider=x", "{}"); status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
	t.Run("unbuildable url", func(t *testing.T) {
		f := newHTTPFixture(t, 1, 1, "member")
		createProvider(t, f, ProviderInput{Name: "stub", Backend: "openai", APIKey: "k",
			BaseURL: "http://exa mple.invalid"})
		if status, _ := f.do(t, http.MethodPost, "/v1/chat/completions", "{}"); status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
}

// TestPricingServicesWithoutAPluginFailClosed keeps a half-built composition
// from answering a price of zero.
func TestPricingServicesWithoutAPluginFailClosed(t *testing.T) {
	if _, err := (pricingService{}).Price(context.Background(), 1, "m"); !errors.Is(err, plugin.ErrPricingUnavailable) {
		t.Fatalf("pricing err = %v", err)
	}
	if _, err := (gatewayService{}).Provider(context.Background(), 1); !errors.Is(err, plugin.ErrLLMGatewayUnavailable) {
		t.Fatalf("gateway err = %v", err)
	}
}
