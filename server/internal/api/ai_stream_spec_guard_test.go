package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

// TestAIChatStream_TheSpecAllowlistIsEnforcedAtExecution is the test that
// matters most for a published harness: the allowlist is a control, not a
// hint.
//
// The prompt narrows the advertised tools; the guard is what makes the
// promise true when the model decides to call something else. Before the
// guard existed, a workspace that published "read_things only" still had an
// assistant that could run any read tool — and the offline lab, which
// enforced the allowlist, reported the candidate as restricted while
// production was not.
func TestAIChatStream_TheSpecAllowlistIsEnforcedAtExecution(t *testing.T) {
	var allowedExecuted, deniedExecuted bool
	src := &fakeEndpointSource{
		endpoints: map[string]plugin.Endpoint{
			"read_things": &fakeEndpoint{
				name:     "read_things",
				method:   http.MethodGet,
				executed: &allowedExecuted,
			},
			"read_more": &fakeEndpoint{
				name:     "read_more",
				method:   http.MethodGet,
				executed: &deniedExecuted,
			},
		},
	}

	fake := &fakeStreamingLLM{
		tools: []llmprovider.ToolCallChunk{
			{Index: 0, ID: "call_1", Name: "read_more", ArgsJSON: `{}`},
		},
		resp: llmprovider.Response{StopReason: "tool_use"},
	}

	srv, _, h := newAIStreamTestHandlerFull(t, fake, src)
	h.SetHost(hostWithPublishedHarness(t, harness.HarnessSpec{
		Version:   harness.HarnessSpecVersion,
		Name:      "narrow",
		ToolAllow: []string{"read_things"},
	}))

	body := `{"messages":[{"role":"user","content":"read more"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if deniedExecuted {
		t.Fatal("a tool outside the published allowlist was executed")
	}
}

// TestAIChatStream_TheSpecAllowlistPermitsTheListedTool is the positive
// control for the test above: the allowlist narrows, it does not disable.
func TestAIChatStream_TheSpecAllowlistPermitsTheListedTool(t *testing.T) {
	var executed bool
	src := &fakeEndpointSource{
		endpoints: map[string]plugin.Endpoint{
			"read_things": &fakeEndpoint{
				name:     "read_things",
				method:   http.MethodGet,
				executed: &executed,
			},
		},
	}

	fake := &fakeStreamingLLM{
		tools: []llmprovider.ToolCallChunk{
			{Index: 0, ID: "call_1", Name: "read_things", ArgsJSON: `{}`},
		},
		resp: llmprovider.Response{StopReason: "tool_use"},
	}

	srv, _, h := newAIStreamTestHandlerFull(t, fake, src)
	h.SetHost(hostWithPublishedHarness(t, harness.HarnessSpec{
		Version:   harness.HarnessSpecVersion,
		Name:      "narrow",
		ToolAllow: []string{"read_things"},
	}))

	body := `{"messages":[{"role":"user","content":"read things"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if !executed {
		t.Fatal("a tool inside the published allowlist was not executed")
	}
}
