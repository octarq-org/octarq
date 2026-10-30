package api

import (
	"context"
	"encoding/json"
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
	srv, db, _ := newAIStreamTestHandlerFull(t, p, src)
	return srv, db
}

// newAIStreamTestHandlerFull also hands back the handler, for tests that need
// to wire a collaborator the request path resolves.
func newAIStreamTestHandlerFull(t *testing.T, p llmprovider.Provider, src harness.EndpointSource) (http.Handler, *gorm.DB, *Handler) {
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
	// The app wires the harness store into the handler in production; a
	// handler without it answers the agent chat with a 500. These tests use
	// a workspace that published nothing.
	h.SetHarnessSpecProvider(func(context.Context, uint) (harness.HarnessSpec, error) {
		return harness.DefaultHarnessSpec(), nil
	})
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
	return srv, db, h
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

// aguiEvents decodes the AG-UI frames of a streamed response.
func aguiEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev); err != nil {
			t.Fatalf("bad AG-UI frame %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

// findAGUI returns the first event of the given type.
func findAGUI(events []map[string]any, typ string) (map[string]any, bool) {
	for _, ev := range events {
		if ev["type"] == typ {
			return ev, true
		}
	}
	return nil, false
}

// requireAGUI fails the test when the event is absent.
func requireAGUI(t *testing.T, events []map[string]any, typ string) map[string]any {
	t.Helper()
	ev, ok := findAGUI(events, typ)
	if !ok {
		t.Fatalf("missing %s event in %v", typ, events)
	}
	return ev
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

	events := aguiEvents(t, rec.Body.String())
	if events[0]["type"] != "RUN_STARTED" {
		t.Fatalf("first event = %v, want RUN_STARTED", events[0])
	}
	if ev, ok := findAGUI(events, "THINKING_TEXT_MESSAGE_CONTENT"); !ok || ev["delta"] != "Thinking..." {
		t.Errorf("missing thinking delta in %v", events)
	}
	if ev, ok := findAGUI(events, "TEXT_MESSAGE_CONTENT"); !ok || ev["delta"] != "Hello " {
		t.Errorf("missing text delta in %v", events)
	}
	finished, ok := findAGUI(events, "RUN_FINISHED")
	if !ok {
		t.Fatalf("missing RUN_FINISHED in %v", events)
	}
	usage, _ := finished["usage"].([]any)
	if len(usage) != 1 {
		t.Fatalf("usage = %v, want one entry", finished["usage"])
	}
	entry, _ := usage[0].(map[string]any)
	if entry["reasoningTokens"] != float64(4) || entry["inputTokens"] != float64(10) {
		t.Errorf("usage entry = %v, want the turn totals", entry)
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

	events := aguiEvents(t, rec.Body.String())
	errEvent, ok := findAGUI(events, "RUN_ERROR")
	if !ok {
		t.Fatalf("expected RUN_ERROR in stream, got %v", events)
	}
	if msg, _ := errEvent["message"].(string); !strings.Contains(msg, "upstream API error") {
		t.Errorf("expected upstream error message, got %q", msg)
	}
	if _, ok := findAGUI(events, "RUN_FINISHED"); ok {
		t.Errorf("a failed run must not finish successfully: %v", events)
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

	events := aguiEvents(t, rec.Body.String())
	started, ok := findAGUI(events, "TOOL_CALL_START")
	if !ok || started["toolCallName"] != "search" {
		t.Fatalf("expected a TOOL_CALL_START for search, got %v", events)
	}
	args, ok := findAGUI(events, "TOOL_CALL_ARGS")
	if !ok || args["delta"] != `{"q":"golang"}` {
		t.Fatalf("expected the tool arguments, got %v", events)
	}
	if _, ok := findAGUI(events, "TOOL_CALL_RESULT"); !ok {
		t.Fatalf("expected a tool result, got %v", events)
	}
}

// TestAIChatStream_RefusesUnsupportedRunInput pins the fields the server
// would otherwise drop. A client that sends them must learn they have no
// effect, not run with a false belief about the turn.
func TestAIChatStream_RefusesUnsupportedRunInput(t *testing.T) {
	user := `{"role":"user","content":"hi"}`
	cases := []struct {
		name string
		body string
		want string
	}{
		{"client tools", `{"messages":[` + user + `],"tools":[{"name":"x"}]}`, "Client tools are not supported"},
		{"run state", `{"messages":[` + user + `],"state":{"a":1}}`, "Run state is not supported."},
		{"context", `{"messages":[` + user + `],"context":[{"value":"x"}]}`, "Context entries are not supported."},
		{"forwarded props", `{"messages":[` + user + `],"forwardedProps":{"a":1}}`, "Forwarded properties are not supported."},
		{"resume", `{"messages":[` + user + `],"resume":[{"interruptId":"i"}]}`, "Run resume is not supported."},
		{"structured content", `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`, "Message content must be text."},
		{"tool role", `{"messages":[{"role":"tool","content":"out"},` + user + `]}`, "Only user and assistant messages are accepted."},
		{"last is assistant", `{"messages":[` + user + `,{"role":"assistant","content":"yo"}]}`, "The last message must come from the user."},
		{"empty last", `{"messages":[{"role":"user","content":"   "}]}`, "The last message must have text."},
		{"long run id", `{"runId":"` + strings.Repeat("r", 65) + `","messages":[` + user + `]}`, "The run ID must have 1 to 64 characters."},
		{"long thread id", `{"threadId":"` + strings.Repeat("t", 129) + `","messages":[` + user + `]}`, "The thread ID must have 1 to 128 characters."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newAIStreamTestHandler(t, &fakeStreamingLLM{})
			rec := do(srv, "POST", "/api/ai/chat/stream", sessionCookies(t, 1, 1), tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tc.want)
			}
		})
	}
}

// TestAIChatStream_EmptyRunInputIsAccepted pins the standard shape: an AG-UI
// client sends null or empty containers for fields it does not use.
func TestAIChatStream_EmptyRunInputIsAccepted(t *testing.T) {
	srv, _ := newAIStreamTestHandler(t, &fakeStreamingLLM{texts: []string{"ok"}})
	body := `{"threadId":"t-1","runId":"r-1","messages":[{"role":"user","content":"hi"}],"tools":[],"state":null,"context":[],"forwardedProps":{}}`
	rec := do(srv, "POST", "/api/ai/chat/stream", sessionCookies(t, 1, 1), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	events := aguiEvents(t, rec.Body.String())
	if len(events) == 0 || events[0]["type"] != "RUN_STARTED" {
		t.Fatalf("first event = %v, want RUN_STARTED", events)
	}
	if events[0]["threadId"] != "t-1" || events[0]["runId"] != "r-1" {
		t.Fatalf("run started = %v", events[0])
	}
	last := events[len(events)-1]
	if last["type"] != "RUN_FINISHED" {
		t.Fatalf("last event = %v, want RUN_FINISHED", last)
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

// TestAIChatStream_WireFraming locks the SSE shape a client parses: each
// frame is an optional "id:" line and one "data: <json>" line, frames end
// with a blank line, and the stream carries no named "event:" lines. A
// keep-alive comment is the one other frame a client can receive.
func TestAIChatStream_WireFraming(t *testing.T) {
	fake := &fakeStreamingLLM{texts: []string{"ok"}, resp: llmprovider.Response{StopReason: "end_turn"}}
	srv, _ := newAIStreamTestHandler(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream",
		strings.NewReader(`{"threadId":"thread-wire","runId":"run-wire","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("the last frame does not end with a blank line: %q", body)
	}
	frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
	if len(frames) < 3 {
		t.Fatalf("frames = %d, want at least RUN_STARTED, text and RUN_FINISHED", len(frames))
	}

	types := make([]string, 0, len(frames))
	for _, frame := range frames {
		if strings.HasPrefix(frame, ": ") {
			if frame != ": ping" {
				t.Fatalf("unexpected comment frame %q", frame)
			}
			continue
		}
		lines := strings.Split(frame, "\n")
		if lines[0] != "" && strings.HasPrefix(lines[0], "id: ") {
			lines = lines[1:]
		}
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "data: ") {
			t.Fatalf("frame %q is not one data line", frame)
		}
		raw := strings.TrimPrefix(lines[0], "data: ")
		var ev map[string]any
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			t.Fatalf("frame %q is not JSON: %v", frame, err)
		}
		typ, _ := ev["type"].(string)
		if typ == "" {
			t.Fatalf("frame has no type: %q", frame)
		}
		types = append(types, typ)
	}
	if types[0] != "RUN_STARTED" || types[len(types)-1] != "RUN_FINISHED" {
		t.Fatalf("event order = %v, want RUN_STARTED first and RUN_FINISHED last", types)
	}
	if strings.Contains(body, "event:") {
		t.Fatalf("the stream carries named event lines: %q", body)
	}
}
