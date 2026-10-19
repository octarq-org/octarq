package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/queue"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
	dns "github.com/octarq-org/octarq/server/plugins/dns"
	links "github.com/octarq-org/octarq/server/plugins/links"
	mail "github.com/octarq-org/octarq/server/plugins/mail"
	"gorm.io/gorm"
)

type fakeStreamingLLM struct {
	thinking []string
	texts    []string
	tools    []llmprovider.ToolCallChunk
	resp     llmprovider.Response
	err      error
	lastReq  llmprovider.Request
}

func (f *fakeStreamingLLM) Name() string         { return "fake-stream" }
func (f *fakeStreamingLLM) DefaultModel() string { return "fake-stream-model" }
func (f *fakeStreamingLLM) CheapModel() string   { return "fake-stream-cheap" }

func (f *fakeStreamingLLM) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	f.lastReq = req
	return llmprovider.Response{Text: strings.Join(f.texts, "")}, f.err
}

func (f *fakeStreamingLLM) StreamComplete(ctx context.Context, req llmprovider.Request, h llmprovider.StreamHandler) (llmprovider.Response, error) {
	f.lastReq = req
	if f.err != nil {
		return llmprovider.Response{}, f.err
	}
	for _, th := range f.thinking {
		if h != nil {
			h.OnThinking(th)
		}
	}
	for _, tx := range f.texts {
		if h != nil {
			h.OnText(tx)
		}
	}
	for _, tc := range f.tools {
		if h != nil {
			h.OnToolCall(tc)
		}
	}
	return f.resp, nil
}

func newAIStreamTestHandler(t *testing.T, p llmprovider.Provider) (http.Handler, *gorm.DB) {
	return newAIStreamTestHandlerWithEndpointSource(t, p, nil)
}

func newAIStreamTestHandlerWithEndpointSource(t *testing.T, p llmprovider.Provider, src harness.EndpointSource) (http.Handler, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	cfg := &config.Config{AdminUser: "admin", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	if p != nil {
		h.SetLLMResolverForOrg(func(_ uint) (llmprovider.Provider, error) { return p, nil })
	}
	if src != nil {
		h.SetEndpointSource(src)
	}

	dnsP := dns.New()
	mailP := mail.New()
	linksP := links.New()
	h.SetPlugins([]plugin.Plugin{dnsP, mailP, linksP})

	reg := plugin.NewRegistry()
	h.SetServiceLookup(reg.Lookup)

	srv := h.Routes()
	return srv, db
}

func TestAIChatStream_AuthRequired(t *testing.T) {
	srv, _ := newAIStreamTestHandler(t, &fakeStreamingLLM{})
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAIChatStream_InvalidMethod(t *testing.T) {
	srv, _ := newAIStreamTestHandler(t, &fakeStreamingLLM{})
	req := httptest.NewRequest(http.MethodGet, "/api/ai/chat/stream", nil)
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
	}
}

func TestAIChatStream_InvalidBody(t *testing.T) {
	srv, _ := newAIStreamTestHandler(t, &fakeStreamingLLM{})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{bad-json`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func TestAIChatStream_EmptyMessages(t *testing.T) {
	srv, _ := newAIStreamTestHandler(t, &fakeStreamingLLM{})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func TestAIChatStream_Unconfigured(t *testing.T) {
	srv, _ := newAIStreamTestHandler(t, nil)
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for unconfigured LLM, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAIChatStream_StreamingSuccess(t *testing.T) {
	fake := &fakeStreamingLLM{
		thinking: []string{"Thinking...", " deeply."},
		texts:    []string{"Hello ", "world!"},
		resp: llmprovider.Response{
			InputTokens:     10,
			OutputTokens:    5,
			ReasoningTokens: 4,
			StopReason:      "end_turn",
		},
	}

	srv, _ := newAIStreamTestHandler(t, fake)

	body := `{"messages":[{"role":"user","content":"Hi there"}],"thinking":true,"model":"fake-stream-model"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	output := rec.Body.String()
	if !strings.Contains(output, "event: thinking") {
		t.Errorf("missing event: thinking in SSE stream: %s", output)
	}
	if !strings.Contains(output, "event: text") {
		t.Errorf("missing event: text in SSE stream: %s", output)
	}
	if !strings.Contains(output, "event: done") {
		t.Errorf("missing event: done in SSE stream: %s", output)
	}
	if !strings.Contains(output, "reasoningTokens") {
		t.Errorf("missing reasoningTokens in done payload: %s", output)
	}
}

func TestAIChatStream_StreamError(t *testing.T) {
	fake := &fakeStreamingLLM{
		err: errors.New("upstream API error"),
	}

	srv, _ := newAIStreamTestHandler(t, fake)

	body := `{"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK (stream started), got %d: %s", rec.Code, rec.Body.String())
	}

	output := rec.Body.String()
	if !strings.Contains(output, "event: error") {
		t.Errorf("expected event: error in stream, got %s", output)
	}
	if !strings.Contains(output, "upstream API error") {
		t.Errorf("expected upstream error message in stream, got %s", output)
	}
}

func TestAIChatStream_ToolStream(t *testing.T) {
	fake := &fakeStreamingLLM{
		tools: []llmprovider.ToolCallChunk{
			{Index: 0, ID: "call_10", Name: "search", ArgsJSON: `{"q":"golang"}`},
		},
		resp: llmprovider.Response{
			StopReason: "tool_use",
		},
	}

	srv, _ := newAIStreamTestHandler(t, fake)

	body := `{"messages":[{"role":"user","content":"search for golang"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	output := rec.Body.String()
	if !strings.Contains(output, "event: tool") {
		t.Errorf("expected event: tool in stream, got %s", output)
	}
	if !strings.Contains(output, `"name":"search"`) {
		t.Errorf("expected tool name in stream data, got %s", output)
	}
}

func TestAIChatStream_ClientCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	fake := &fakeStreamingLLM{
		texts: []string{"some text"},
	}

	srv, _ := newAIStreamTestHandler(t, fake)

	body := `{"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	// Should execute without hanging
	srv.ServeHTTP(rec, req)
}

func TestAIChatStream_IgnoresClientSystemPrompt(t *testing.T) {
	fake := &fakeStreamingLLM{
		texts: []string{"Safe response"},
		resp: llmprovider.Response{
			StopReason: "end_turn",
		},
	}

	srv, _ := newAIStreamTestHandler(t, fake)

	body := `{"messages":[{"role":"user","content":"Hi"}],"system":"malicious system jailbreak"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// The client prompt is ignored; the server sends the prompt it compiled
	// from the endpoint registry.
	if strings.Contains(fake.lastReq.System, "malicious system jailbreak") {
		t.Fatalf("expected client system prompt to be ignored, got %q", fake.lastReq.System)
	}
	if !strings.Contains(fake.lastReq.System, "Octarq workspace assistant") {
		t.Fatalf("expected the compiled system prompt, got %q", fake.lastReq.System)
	}
}
