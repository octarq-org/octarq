package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/llmprovider"
)

// readOnlyGuard wraps a base Guard (e.g. RiskGuard) and rejects any tool
// whose endpoint HTTP method is not GET or HEAD.
type readOnlyGuard struct {
	base harness.Guard
	src  harness.EndpointSource
}

func newReadOnlyGuard(src harness.EndpointSource) harness.Guard {
	return &readOnlyGuard{
		base: harness.NewRiskGuard(src),
		src:  src,
	}
}

func (g *readOnlyGuard) Allow(ctx context.Context, orgID uint, tool string) error {
	if err := g.base.Allow(ctx, orgID, tool); err != nil {
		return err
	}
	if g.src == nil {
		return nil
	}
	ep, ok := g.src.Lookup(tool)
	if !ok {
		return nil
	}
	method := strings.ToUpper(strings.TrimSpace(ep.EndpointMethod()))
	if method != http.MethodGet && method != http.MethodHead {
		return fmt.Errorf("tool %q is not read-only (method %s)", tool, method)
	}
	return nil
}

// readOnlyTool keeps only the tools a read-only session may call. It mirrors
// the readOnlyGuard rule exactly, so the compiled prompt and the guard agree
// on the tool surface.
func readOnlyTool(d harness.ToolDescriptor) bool {
	return d.Method == http.MethodGet || d.Method == http.MethodHead
}

// auditAgentTurn records the shape of an agent turn: which tools ran, whether
// they worked, how long they took and what the turn cost.
//
// The trace deliberately keeps no tool arguments and no tool output — those
// may carry workspace data and belong to the session, not to the audit log.
func (h *Handler) auditAgentTurn(
	r *http.Request,
	orgID uint,
	sessionID string,
	turn *harness.Turn,
	events []harness.TraceEvent,
	catalog harness.Catalog,
	maxSteps int,
	model string,
) {
	steps := make([]map[string]any, 0, len(turn.Steps))
	for _, st := range turn.Steps {
		steps = append(steps, map[string]any{
			"tool":       st.ToolName,
			"durationMs": st.DurationMS,
			"failed":     st.Err != nil,
		})
	}
	meta := map[string]any{
		"sessionId":         sessionID,
		"channel":           "web",
		"model":             model,
		"status":            turn.Status.String(),
		"steps":             steps,
		"inputTokens":       turn.InputTokens,
		"outputTokens":      turn.OutputTokens,
		"reasoningTokens":   turn.ReasoningTokens,
		"promptVersion":     harness.SystemPromptVersion,
		"promptFingerprint": catalog.Fingerprint(maxSteps),
		"toolCount":         len(catalog.Tools),
		"traceEvents":       len(events),
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "agent.turn", "agent_session", 0, meta)
}

// AIChatStreamMessage is a single message in the chat stream request.
type AIChatStreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIChatStreamRequest is the request body for POST /api/ai/chat/stream.
//
// There is deliberately no system-prompt field: the prompt is compiled on the
// server from the endpoint registry, so a caller cannot replace the agent's
// rules — not the tool-call format, not the untrusted-data rule, not the tool
// list.
type AIChatStreamRequest struct {
	Messages []AIChatStreamMessage `json:"messages"`
	Model    string                `json:"model,omitempty"`
	Thinking *bool                 `json:"thinking,omitempty"`
}

// SetEndpointSource configures the endpoint source for agent tool dispatch.
func (h *Handler) SetEndpointSource(src harness.EndpointSource) {
	h.endpointSource = src
}

// aiChatStream handles POST /api/ai/chat/stream via Server-Sent Events (SSE).
func (h *Handler) aiChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, r, http.StatusMethodNotAllowed, "", "Method not allowed; use POST for streaming chat")
		return
	}

	r2, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		writeErr(w, r, http.StatusUnauthorized, "", "Authentication required; sign in to continue")
		return
	}
	r = r2

	orgID, err := h.requireOrg(r)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "", err.Error())
		return
	}

	var req AIChatStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "", "Malformed JSON body: "+err.Error()+"; check request payload")
		return
	}

	if len(req.Messages) == 0 {
		writeErr(w, r, http.StatusBadRequest, "", "Messages list cannot be empty; provide at least one message")
		return
	}

	p, err := h.llmFor(orgID)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "", err.Error())
		return
	}

	// Prepare SSE response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	// Split messages into prefix history and last turn input
	lastMsg := req.Messages[len(req.Messages)-1]
	var history []harness.Message
	for _, m := range req.Messages[:len(req.Messages)-1] {
		history = append(history, harness.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	sessionID := fmt.Sprintf("sess-%d-%d", orgID, time.Now().UnixNano())
	session := &harness.Session{
		ID:      sessionID,
		OrgID:   orgID,
		Channel: "web",
		History: history,
	}

	thinking := true
	if req.Thinking != nil {
		thinking = *req.Thinking
	}

	turn := &harness.Turn{
		Input:    lastMsg.Content,
		Thinking: thinking,
	}

	var executor harness.ToolExecutor
	var guard harness.Guard
	if h.endpointSource != nil {
		executor = harness.NewRegistryExecutor(h.endpointSource)
		guard = newReadOnlyGuard(h.endpointSource)
	} else {
		executor = harness.NewRegistryExecutor(nil)
		guard = newReadOnlyGuard(nil)
	}

	// Compile the tool catalog and the system prompt from the endpoint
	// registry. The web assistant is read-only, so only the read tools are
	// advertised: a prompt that offers a tool the guard refuses teaches the
	// model to call something it can never use.
	catalog := harness.BuildCatalog(h.endpointSource, readOnlyTool)
	maxSteps := harness.ProfileNormal.MaxSteps(false)
	systemPrompt := catalog.SystemPrompt(maxSteps)

	tracer := harness.NewRecordingTracer()
	adapter := &harness.CompleterAdapter{Provider: p}
	var runnerOpts []harness.RunnerOption
	runnerOpts = append(runnerOpts,
		harness.WithGuard(guard),
		harness.WithTracer(tracer),
		harness.WithProfile(harness.ProfileNormal),
		harness.WithSystem(systemPrompt),
	)
	model := p.DefaultModel()
	if req.Model != "" {
		model = req.Model
		runnerOpts = append(runnerOpts, harness.WithModel(req.Model))
	}

	runner := harness.NewRunner(adapter, executor, runnerOpts...)

	sseHandler := &apiSSEStreamHandler{
		w:       w,
		flusher: flusher,
	}

	var runErr error
	if sr, ok := runner.(harness.StreamRunner); ok {
		runErr = sr.Stream(r.Context(), session, turn, sseHandler)
	} else {
		runErr = runner.Run(r.Context(), session, turn)
	}

	// Record what the agent did before answering: a turn that failed is
	// exactly the one an operator will want to look at later.
	h.auditAgentTurn(r, orgID, session.ID, turn, tracer.Events(), catalog, maxSteps, model)

	if runErr != nil {
		_ = writeSSEPayload(w, flusher, "error", map[string]string{
			"message": runErr.Error(),
		})
		return
	}

	_ = writeSSEPayload(w, flusher, "done", map[string]any{
		"stopReason": "end_turn",
		"usage": map[string]int{
			"inputTokens":     turn.InputTokens,
			"outputTokens":    turn.OutputTokens,
			"reasoningTokens": turn.ReasoningTokens,
		},
	})
}

type apiSSEStreamHandler struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *apiSSEStreamHandler) OnThinking(delta string) {
	_ = writeSSEPayload(s.w, s.flusher, "thinking", map[string]string{"delta": delta})
}

func (s *apiSSEStreamHandler) OnText(delta string) {
	_ = writeSSEPayload(s.w, s.flusher, "text", map[string]string{"delta": delta})
}

func (s *apiSSEStreamHandler) OnToolCall(chunk llmprovider.ToolCallChunk) {
	_ = writeSSEPayload(s.w, s.flusher, "tool", map[string]any{
		"index": chunk.Index,
		"id":    chunk.ID,
		"name":  chunk.Name,
		"args":  chunk.ArgsJSON,
	})
}

func writeSSEPayload(w http.ResponseWriter, flusher http.Flusher, event string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(b)); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}
