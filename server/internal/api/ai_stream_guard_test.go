package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

type fakeEndpoint struct {
	name            string
	method          string
	requireApproval bool
	executed        *bool
}

func (e *fakeEndpoint) EndpointName() string          { return e.name }
func (e *fakeEndpoint) EndpointSummary() string       { return "" }
func (e *fakeEndpoint) EndpointDescription() string   { return "" }
func (e *fakeEndpoint) EndpointMethod() string        { return e.method }
func (e *fakeEndpoint) EndpointPath() string          { return "/api/test" }
func (e *fakeEndpoint) EndpointRequireAuth() bool     { return false }
func (e *fakeEndpoint) EndpointRequireRole() []string { return nil }
func (e *fakeEndpoint) EndpointExposeMCP() bool       { return true }
func (e *fakeEndpoint) EndpointRequireApproval() bool { return e.requireApproval }
func (e *fakeEndpoint) EndpointRisk() string          { return "read" }
func (e *fakeEndpoint) EndpointInputSchema() json.RawMessage {
	return nil
}
func (e *fakeEndpoint) Execute(ctx context.Context, input any) (any, error) {
	if e.executed != nil {
		*e.executed = true
	}
	return map[string]string{"result": "ok"}, nil
}
func (e *fakeEndpoint) ExecuteAgentJSON(ctx context.Context, argsJSON string) (any, error) {
	if e.executed != nil {
		*e.executed = true
	}
	return map[string]string{"result": "ok"}, nil
}
func (e *fakeEndpoint) Spec() any { return nil }

type fakeEndpointSource struct {
	endpoints map[string]plugin.Endpoint
}

func (s *fakeEndpointSource) Lookup(name string) (plugin.Endpoint, bool) {
	ep, ok := s.endpoints[name]
	return ep, ok
}

func (s *fakeEndpointSource) Endpoints() []plugin.Endpoint {
	out := make([]plugin.Endpoint, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		out = append(out, ep)
	}
	return out
}

func TestAIChatStream_ReadOnlyGuard_BlocksNonGet(t *testing.T) {
	var postExecuted bool
	src := &fakeEndpointSource{
		endpoints: map[string]plugin.Endpoint{
			"create_link": &fakeEndpoint{
				name:     "create_link",
				method:   "POST",
				executed: &postExecuted,
			},
		},
	}

	fake := &fakeStreamingLLM{
		tools: []llmprovider.ToolCallChunk{
			{Index: 0, ID: "call_1", Name: "create_link", ArgsJSON: `{"url":"https://example.com"}`},
		},
		resp: llmprovider.Response{
			StopReason: "tool_use",
		},
	}

	srv, _ := newAIStreamTestHandlerWithEndpointSource(t, fake, src)

	body := `{"messages":[{"role":"user","content":"create a link"}]}`
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

	if postExecuted {
		t.Fatalf("POST endpoint was executed but should have been blocked by readOnlyGuard")
	}
}

func TestAIChatStream_ReadOnlyGuard_HaltsOnApprovalRequired(t *testing.T) {
	var executed bool
	src := &fakeEndpointSource{
		endpoints: map[string]plugin.Endpoint{
			"sensitive_read": &fakeEndpoint{
				name:            "sensitive_read",
				method:          "GET",
				requireApproval: true,
				executed:        &executed,
			},
		},
	}

	fake := &fakeStreamingLLM{
		tools: []llmprovider.ToolCallChunk{
			{Index: 0, ID: "call_1", Name: "sensitive_read", ArgsJSON: `{}`},
		},
		resp: llmprovider.Response{
			StopReason: "tool_use",
		},
	}

	srv, _ := newAIStreamTestHandlerWithEndpointSource(t, fake, src)

	body := `{"messages":[{"role":"user","content":"read sensitive data"}]}`
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

	if executed {
		t.Fatalf("approval required endpoint was executed but should have been vetoed")
	}

	output := rec.Body.String()
	if !strings.Contains(output, "event: error") {
		t.Fatalf("expected event: error in stream output, got %s", output)
	}
	if !strings.Contains(output, "requires operator approval") {
		t.Fatalf("expected approval required error message, got %s", output)
	}
}

// The audit trail must say what the guard did. A refusal and a failure look
// the same in a row that only records "failed", and an operator cannot tell
// a policy decision from a broken tool.
func TestAIChatStream_AuditRecordsTheStepOutcome(t *testing.T) {
	var postExecuted bool
	src := &fakeEndpointSource{
		endpoints: map[string]plugin.Endpoint{
			"create_link": &fakeEndpoint{
				name:     "create_link",
				method:   "POST",
				executed: &postExecuted,
			},
		},
	}

	fake := &fakeStreamingLLM{
		tools: []llmprovider.ToolCallChunk{
			{Index: 0, ID: "call_1", Name: "create_link", ArgsJSON: `{"url":"https://example.com"}`},
		},
		resp: llmprovider.Response{
			StopReason: "tool_use",
		},
	}

	srv, db := newAIStreamTestHandlerWithEndpointSource(t, fake, src)

	body := `{"messages":[{"role":"user","content":"create a link"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	var log models.AuditLog
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := db.Where("action = ?", "agent.turn").First(&log).Error; err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if log.ID == 0 {
		t.Fatal("no agent.turn audit row was written")
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(log.Meta), &meta); err != nil {
		t.Fatalf("audit meta is not JSON: %v (%s)", err, log.Meta)
	}
	steps, ok := meta["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("audit meta carries no step: %s", log.Meta)
	}
	first, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("audit step is not an object: %v", steps[0])
	}
	if first["outcome"] != harness.TraceStatusDenied {
		t.Errorf("step outcome = %v, want %s", first["outcome"], harness.TraceStatusDenied)
	}
	if first["tool"] != "create_link" {
		t.Errorf("step tool = %v, want create_link", first["tool"])
	}
	if _, ok := first["failed"]; ok {
		t.Errorf("the audit step still carries the old failed flag: %v", first)
	}
}
