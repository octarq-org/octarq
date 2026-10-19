package harness

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

type mockStreamProvider struct {
	streamErr error
}

func (m *mockStreamProvider) Name() string         { return "mock-stream" }
func (m *mockStreamProvider) DefaultModel() string { return "mock-model" }
func (m *mockStreamProvider) CheapModel() string   { return "mock-cheap" }

func (m *mockStreamProvider) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{Text: "resp"}, nil
}

func (m *mockStreamProvider) StreamComplete(ctx context.Context, req llmprovider.Request, h llmprovider.StreamHandler) (llmprovider.Response, error) {
	if m.streamErr != nil {
		return llmprovider.Response{}, m.streamErr
	}
	if h != nil {
		h.OnText("```tool_call\n{\"name\":\"forbidden\",\"args\":\"{}\"}\n```")
	}
	return llmprovider.Response{}, nil
}

type mockStreamOrderErrorProvider struct {
	mockStreamProvider
}

func (m *mockStreamOrderErrorProvider) StreamComplete(ctx context.Context, req llmprovider.Request, h llmprovider.StreamHandler) (llmprovider.Response, error) {
	if h != nil {
		h.OnText("text first")
		h.OnThinking("thinking after text")
	}
	return llmprovider.Response{}, nil
}

type mockNonStreamProvider struct {
	err error
}

func (m *mockNonStreamProvider) Name() string         { return "mock-nonstream" }
func (m *mockNonStreamProvider) DefaultModel() string { return "mock-model" }
func (m *mockNonStreamProvider) CheapModel() string   { return "mock-cheap" }

func (m *mockNonStreamProvider) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	if m.err != nil {
		return llmprovider.Response{}, m.err
	}
	return llmprovider.Response{Text: "ok"}, nil
}

func TestCoverageLiftStreamMaxSteps(t *testing.T) {
	// A streamer that keeps outputting tool calls until maxSteps is exceeded
	streamer := &mockStreamProvider{}
	runner := NewRunner(&CompleterAdapter{Provider: streamer}, &fakeExecutor{}, WithProfile(ProfileReactor)).(*defaultRunner)

	s := &Session{ID: "s1", OrgID: 1}
	turn := &Turn{Input: "loop"}
	err := runner.Stream(context.Background(), s, turn, nil)
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", err)
	}
	if turn.Status != TurnStatusFailed {
		t.Errorf("expected TurnStatusFailed, got %v", turn.Status)
	}
}

func TestCoverageLiftCompleterAdapter(t *testing.T) {
	ctx := context.Background()

	// 1. Complete with error
	badAdapter := &CompleterAdapter{Provider: &mockNonStreamProvider{err: errors.New("fail")}}
	_, err := badAdapter.Complete(ctx, "model", "system", nil)
	if err == nil || !strings.Contains(err.Error(), "fail") {
		t.Fatalf("expected fail error, got %v", err)
	}

	// 2. StreamComplete with provider that does NOT support streaming
	_, err = badAdapter.StreamComplete(ctx, llmprovider.Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), "does not support streaming") {
		t.Fatalf("expected does not support streaming error, got %v", err)
	}

	// 3. StreamComplete with provider that DOES support streaming
	streamAdapter := &CompleterAdapter{Provider: &mockStreamProvider{}}
	_, err = streamAdapter.StreamComplete(ctx, llmprovider.Request{}, nil)
	if err != nil {
		t.Fatalf("expected no error from streamAdapter, got %v", err)
	}
}

func TestCoverageLiftStreamRunnerBranches(t *testing.T) {
	ctx := context.Background()

	// 1. StreamComplete returning error
	failingStreamer := &mockStreamProvider{streamErr: errors.New("stream exploded")}
	r1 := NewRunner(&CompleterAdapter{Provider: failingStreamer}, &fakeExecutor{}).(*defaultRunner)
	s1 := &Session{ID: "s1", OrgID: 1}
	t1 := &Turn{Input: "hi"}
	err := r1.Stream(ctx, s1, t1, nil)
	if err == nil || !strings.Contains(err.Error(), "harness: stream error") {
		t.Fatalf("expected stream error, got %v", err)
	}
	if t1.Status != TurnStatusFailed {
		t.Errorf("expected failed status, got %v", t1.Status)
	}

	// 2. Stream execution order violation
	r2 := NewRunner(&CompleterAdapter{Provider: &mockStreamOrderErrorProvider{}}, &fakeExecutor{}).(*defaultRunner)
	s2 := &Session{ID: "s2", OrgID: 1}
	t2 := &Turn{Input: "invoke"}
	err = r2.Stream(ctx, s2, t2, nil)
	if err == nil || !strings.Contains(err.Error(), "stream order error") {
		t.Fatalf("expected stream order error, got %v", err)
	}
	if t2.Status != TurnStatusFailed {
		t.Errorf("expected failed status, got %v", t2.Status)
	}

	// 3. streamCollector early returns when c.err != nil
	collector := newStreamCollector(nil)
	collector.err = errors.New("prior error")
	collector.OnThinking("delta")
	collector.OnText("delta")
	collector.OnToolCall(llmprovider.ToolCallChunk{})
	if collector.thinking.Len() != 0 || collector.text.Len() != 0 {
		t.Errorf("collector wrote data even when error set")
	}
}

type mockBadOutputEndpoint struct{}

func (mockBadOutputEndpoint) EndpointName() string          { return "bad" }
func (mockBadOutputEndpoint) EndpointSummary() string       { return "bad" }
func (mockBadOutputEndpoint) EndpointDescription() string   { return "bad" }
func (mockBadOutputEndpoint) EndpointMethod() string        { return "POST" }
func (mockBadOutputEndpoint) EndpointPath() string          { return "/bad" }
func (mockBadOutputEndpoint) EndpointRequireAuth() bool     { return false }
func (mockBadOutputEndpoint) EndpointRequireRole() []string { return nil }
func (mockBadOutputEndpoint) EndpointExposeMCP() bool       { return false }
func (mockBadOutputEndpoint) EndpointRequireApproval() bool { return false }
func (mockBadOutputEndpoint) EndpointRisk() string          { return "write" }
func (mockBadOutputEndpoint) EndpointInputSchema() *huma.Schema {
	return nil
}
func (mockBadOutputEndpoint) Execute(_ context.Context, _ any) (any, error) {
	return nil, nil
}
func (mockBadOutputEndpoint) Spec() any { return nil }
func (mockBadOutputEndpoint) ExecuteAgentJSON(_ context.Context, _ string) (any, error) {
	// Channels cannot be JSON-marshaled
	return make(chan int), nil
}

type mockBadOutputSource struct{}

func (mockBadOutputSource) Lookup(name string) (plugin.Endpoint, bool) {
	return mockBadOutputEndpoint{}, true
}

func (mockBadOutputSource) Endpoints() []plugin.Endpoint {
	return []plugin.Endpoint{mockBadOutputEndpoint{}}
}

func TestCoverageLiftWiringEdgeCases(t *testing.T) {
	ctx := context.Background()

	// 1. RegistryExecutor with nil source
	reNil := NewRegistryExecutor(nil)
	_, err := reNil.Execute(ctx, 1, "any_tool", "{}")
	if err == nil || !strings.Contains(err.Error(), "UNKNOWN_TOOL") {
		t.Fatalf("expected UNKNOWN_TOOL error for nil source, got %v", err)
	}

	// 2. RiskGuard with nil source
	rgNil := NewRiskGuard(nil)
	err = rgNil.Allow(ctx, 1, "any_tool")
	if err == nil || !strings.Contains(err.Error(), "UNKNOWN_TOOL") {
		t.Fatalf("expected UNKNOWN_TOOL error for nil source, got %v", err)
	}

	// 3. RegistryExecutor with unmarshalable tool result
	reBad := NewRegistryExecutor(mockBadOutputSource{})
	_, err = reBad.Execute(ctx, 1, "bad", "{}")
	if err == nil || !strings.Contains(err.Error(), "marshal tool output") {
		t.Fatalf("expected marshal tool output error, got %v", err)
	}
}
