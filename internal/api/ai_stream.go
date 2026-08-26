package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/agent/harness"
	"github.com/octarq-org/octarq/llmprovider"
)

// AIChatStreamMessage is a single message in the chat stream request.
type AIChatStreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIChatStreamRequest is the request body for POST /api/ai/chat/stream.
type AIChatStreamRequest struct {
	Messages []AIChatStreamMessage `json:"messages"`
	Model    string                `json:"model,omitempty"`
	Thinking *bool                 `json:"thinking,omitempty"`
	System   string                `json:"system,omitempty"`
}

// SetEndpointSource configures the endpoint source for agent tool dispatch.
func (h *Handler) SetEndpointSource(src harness.EndpointSource) {
	h.endpointSource = src
}

// aiChatStream handles POST /api/ai/chat/stream via Server-Sent Events (SSE).
func (h *Handler) aiChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, r, http.StatusMethodNotAllowed, "", "method not allowed")
		return
	}

	r2, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		writeErr(w, r, http.StatusUnauthorized, "", "unauthorized")
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
		writeErr(w, r, http.StatusBadRequest, "", "invalid JSON body: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeErr(w, r, http.StatusBadRequest, "", "messages must not be empty")
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
		guard = harness.NewRiskGuard(h.endpointSource)
	} else {
		executor = harness.NewRegistryExecutor(nil)
		guard = harness.NewRiskGuard(nil)
	}

	adapter := &harness.CompleterAdapter{Provider: p}
	var runnerOpts []harness.RunnerOption
	runnerOpts = append(runnerOpts,
		harness.WithGuard(guard),
		harness.WithTracer(harness.NopTracer{}),
		harness.WithProfile(harness.ProfileNormal),
	)
	if req.Model != "" {
		runnerOpts = append(runnerOpts, harness.WithModel(req.Model))
	}
	if req.System != "" {
		runnerOpts = append(runnerOpts, harness.WithSystem(req.System))
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
