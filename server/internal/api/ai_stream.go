package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	ssewriter "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
	"github.com/octarq-org/octarq/server/agent/agui"
	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/plugin"
)

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

func (g *readOnlyGuard) Allow(ctx context.Context, orgID uint, tool, argsJSON string) error {
	if err := g.base.Allow(ctx, orgID, tool, argsJSON); err != nil {
		return err
	}
	if g.src == nil {
		return nil
	}
	ep, ok := g.src.Lookup(tool)
	if !ok {
		return nil
	}
	risk := ep.EndpointRisk()
	if risk != plugin.RiskLevelRead {
		return fmt.Errorf("tool %q is not read-only (risk %s)", tool, risk)
	}
	return nil
}

func readOnlyTool(d harness.ToolDescriptor) bool {
	return d.Risk == plugin.RiskLevelRead
}

func composeToolFilter(policy harness.ToolFilter, spec harness.HarnessSpec) harness.ToolFilter {
	specFilter := spec.ToolFilter()
	if policy == nil {
		return specFilter
	}
	if specFilter == nil {
		return policy
	}
	return func(d harness.ToolDescriptor) bool {
		return policy(d) && specFilter(d)
	}
}

func (h *Handler) harnessSpec(ctx context.Context, orgID uint) (harness.HarnessSpec, error) {
	if h == nil || h.host == nil {
		return harness.HarnessSpec{}, errors.New("the agent harness is not configured on this instance")
	}
	raw, err := h.host.HarnessSpec(ctx, orgID)
	if err != nil {
		return harness.HarnessSpec{}, err
	}
	if len(raw) == 0 {
		return harness.DefaultHarnessSpec(), nil
	}
	spec, err := harness.DecodeSpec(raw)
	if err != nil {
		return harness.HarnessSpec{}, fmt.Errorf("published harness spec is invalid: %w", err)
	}
	if err := harness.Validate(spec); err != nil {
		return harness.HarnessSpec{}, fmt.Errorf("published harness spec is invalid: %w", err)
	}
	return spec, nil
}

func (h *Handler) auditAgentTurn(
	r *http.Request,
	orgID uint,
	sessionID string,
	turn *harness.Turn,
	events []harness.TraceEvent,
	catalog harness.Catalog,
	spec harness.HarnessSpec,
	model string,
) {
	steps := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		if ev.Kind != harness.TraceKindStep {
			continue
		}
		step := map[string]any{
			"tool":       ev.Tool,
			"outcome":    ev.Status,
			"durationMs": ev.DurationMS,
		}
		if ev.ErrorCode != "" {
			step["errorCode"] = ev.ErrorCode
		}
		steps = append(steps, step)
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
		"promptFingerprint": catalog.Fingerprint(spec),
		"specFingerprint":   spec.Fingerprint(),
		"toolCount":         len(catalog.Tools),
		"traceEvents":       len(events),
	}
	h.auditAs(r, orgID, h.auth.UserID(r), "agent.turn", "agent_session", 0, meta)
}

const spendWriteTimeout = 10 * time.Second

const spendSourceChat = "chat"

func recordTurnSpend(parent context.Context, ledger plugin.Workforce, orgID uint, in plugin.TurnSpend) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), spendWriteTimeout)
	defer cancel()
	_, err := ledger.RecordSpend(ctx, orgID, in)
	return err
}

func writeBillingRefusal(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, plugin.ErrWorkforceUnavailable):
		writeErr(w, r, http.StatusConflict, apierror.CodeWorkforceAgentNotFound, "This build has no workforce ledger. Install the workforce feature to bill agent turns.")
	case errors.Is(err, plugin.ErrWorkforceAgentNotFound):
		writeErr(w, r, http.StatusNotFound, apierror.CodeWorkforceAgentNotFound, "No such workforce agent found in this workspace.")
	case errors.Is(err, plugin.ErrWorkforceAgentPaused):
		writeErr(w, r, http.StatusConflict, apierror.CodeWorkforceAgentPaused, "This agent is paused. Resume it before you run a turn.")
	case errors.Is(err, plugin.ErrWorkforceBudgetExhausted):
		writeErr(w, r, http.StatusConflict, apierror.CodeWorkforceBudgetExceeded, "This agent reached its monthly budget limit. Raise the limit before you run a turn.")
	case errors.Is(err, plugin.ErrWorkforceModelUnpriced):
		writeErr(w, r, http.StatusBadRequest, apierror.CodeWorkforceModelPriceAbsent, "The pricing dataset has no price for this model. Choose another model.")
	default:
		writeErr(w, r, http.StatusInternalServerError, "", "Could not check the agent budget; retry the request.")
	}
}

type AIChatStreamMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type AIChatStreamRequest struct {
	ThreadID string                `json:"threadId"`
	RunID    string                `json:"runId"`
	Messages []AIChatStreamMessage `json:"messages"`

	Model    string `json:"model,omitempty"`
	Thinking *bool  `json:"thinking,omitempty"`
	AgentID  string `json:"agentId,omitempty"`

	Tools          json.RawMessage `json:"tools,omitempty"`
	State          json.RawMessage `json:"state,omitempty"`
	Context        json.RawMessage `json:"context,omitempty"`
	ForwardedProps json.RawMessage `json:"forwardedProps,omitempty"`
	Resume         json.RawMessage `json:"resume,omitempty"`
}

func emptyJSON(raw json.RawMessage) bool {
	switch strings.TrimSpace(string(raw)) {
	case "", "null", "[]", "{}":
		return true
	}
	return false
}

func parseRunInput(msgs []AIChatStreamMessage) ([]harness.Message, string, string) {
	if len(msgs) == 0 {
		return nil, "", "Messages list cannot be empty; provide at least one message."
	}
	parsed := make([]harness.Message, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" {
			return nil, "", "Only user and assistant messages are accepted."
		}
		var content string
		if err := json.Unmarshal(m.Content, &content); err != nil {
			return nil, "", "Message content must be text."
		}
		parsed = append(parsed, harness.Message{Role: role, Content: content})
	}
	last := parsed[len(parsed)-1]
	if last.Role != "user" {
		return nil, "", "The last message must come from the user."
	}
	if strings.TrimSpace(last.Content) == "" {
		return nil, "", "The last message must have text."
	}
	return parsed[:len(parsed)-1], last.Content, ""
}

func (h *Handler) SetEndpointSource(src harness.EndpointSource) {
	h.endpointSource = src
}

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
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "", "Malformed JSON body: "+err.Error()+"; check request payload")
		return
	}
	if dec.More() {
		writeErr(w, r, http.StatusBadRequest, "", "Malformed JSON body; send one object.")
		return
	}

	history, input, msg := parseRunInput(req.Messages)
	if msg != "" {
		writeErr(w, r, http.StatusBadRequest, "", msg)
		return
	}

	if !emptyJSON(req.Tools) {
		writeErr(w, r, http.StatusBadRequest, "", "Client tools are not supported; the server compiles its own tool list.")
		return
	}
	if !emptyJSON(req.State) {
		writeErr(w, r, http.StatusBadRequest, "", "Run state is not supported.")
		return
	}
	if !emptyJSON(req.Context) {
		writeErr(w, r, http.StatusBadRequest, "", "Context entries are not supported.")
		return
	}
	if !emptyJSON(req.ForwardedProps) {
		writeErr(w, r, http.StatusBadRequest, "", "Forwarded properties are not supported.")
		return
	}
	if !emptyJSON(req.Resume) {
		writeErr(w, r, http.StatusBadRequest, "", "Run resume is not supported.")
		return
	}

	threadID := strings.TrimSpace(req.ThreadID)
	if len(threadID) > 128 {
		writeErr(w, r, http.StatusBadRequest, "", "The thread ID must have 1 to 128 characters.")
		return
	}
	runID := strings.TrimSpace(req.RunID)
	if len(runID) > 64 {
		writeErr(w, r, http.StatusBadRequest, "", "The run ID must have 1 to 64 characters.")
		return
	}
	if threadID == "" {
		threadID = newStreamID("thread")
	}
	if runID == "" {
		runID = newStreamID("run")
	}

	p, err := h.llmFor(orgID)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "", err.Error())
		return
	}

	spec, err := h.harnessSpec(r.Context(), orgID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "", "The workspace's agent harness spec is invalid. Ask an administrator to republish it.")
		return
	}

	model := p.DefaultModel()
	if req.Model != "" {
		model = req.Model
	}

	ledger := h.workforce()
	billingAgent := req.AgentID
	if billingAgent != "" {
		if !h.callerHoldsRole(r, authz.RoleAdmin) {
			writeErr(w, r, http.StatusForbidden, "", "Workspace permission denied; higher workspace role required")
			return
		}
		if err := ledger.AuthorizeTurn(r.Context(), orgID, billingAgent, model); err != nil {
			writeBillingRefusal(w, r, err)
			return
		}
	}

	var cost func(input, output, cacheRead, cacheWrite int) float64
	if spec.MaxCostUSD > 0 {
		cost, err = ledger.TurnCost(r.Context(), orgID, model)
		if err != nil {
			writeBillingRefusal(w, r, err)
			return
		}
	}
	budget := harness.BudgetFromSpec(spec, cost)

	maxSteps, err := spec.EffectiveMaxSteps(false)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "", "The workspace's agent harness spec is invalid. Ask an administrator to republish it.")
		return
	}
	spec = spec.WithMaxSteps(maxSteps)

	var executor harness.ToolExecutor
	var guard harness.Guard
	if h.endpointSource != nil {
		executor = harness.NewRegistryExecutor(h.endpointSource)
		guard = newReadOnlyGuard(h.endpointSource)
	} else {
		executor = harness.NewRegistryExecutor(nil)
		guard = newReadOnlyGuard(nil)
	}
	guard = harness.ComposeGuards(guard, harness.NewSpecGuard(spec))

	catalog := harness.BuildCatalog(h.endpointSource, composeToolFilter(readOnlyTool, spec))
	systemPrompt := catalog.SystemPrompt(spec)

	tracer := harness.NewRecordingTracer()
	adapter := &harness.CompleterAdapter{Provider: p}
	var runnerOpts []harness.RunnerOption
	runnerOpts = append(runnerOpts,
		harness.WithGuard(guard),
		harness.WithTracer(tracer),
		harness.WithProfile(spec.Profile),
		harness.WithSystem(systemPrompt),
		harness.WithMaxSteps(maxSteps),
		harness.WithBudget(budget),
	)
	if req.Model != "" {
		runnerOpts = append(runnerOpts, harness.WithModel(req.Model))
	}

	runner, err := harness.NewRunner(adapter, executor, runnerOpts...)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "", "The workspace's agent harness spec is invalid. Ask an administrator to republish it.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	out := newChatStream(r.Context(), w, flusher)
	defer out.stop()

	stream := agui.New(out.emit, threadID, runID)
	out.emit(aguievents.NewRunStartedEvent(threadID, runID))

	session := &harness.Session{
		ID:      threadID,
		OrgID:   orgID,
		Channel: "web",
		History: history,
	}

	thinking := true
	if req.Thinking != nil {
		thinking = *req.Thinking
	}

	turn := &harness.Turn{
		Input:    input,
		Thinking: thinking,
	}

	var runErr error
	if sr, ok := runner.(harness.StreamRunner); ok {
		runErr = sr.Stream(r.Context(), session, turn, stream)
	} else {
		runErr = runner.Run(r.Context(), session, turn)
	}

	if !stream.WroteText() && runErr == nil {
		stream.Say(lastAssistantText(session))
	}

	h.auditAgentTurn(r, orgID, session.ID, turn, tracer.Events(), catalog, spec, model)

	if stream.Err() != nil {
		return
	}

	if billingAgent != "" {
		err := recordTurnSpend(r.Context(), ledger, orgID, plugin.TurnSpend{
			AgentID:          billingAgent,
			Model:            model,
			InputTokens:      turn.InputTokens,
			OutputTokens:     turn.OutputTokens,
			ReasoningTokens:  turn.ReasoningTokens,
			CacheReadTokens:  turn.CacheReadTokens,
			CacheWriteTokens: turn.CacheWriteTokens,
			Source:           spendSourceChat,
			SessionID:        threadID,
			TurnID:           runID,
		})
		if err != nil && !errors.Is(err, plugin.ErrWorkforceSpendReplayed) {
			slog.Error("failed to record agent spend", "orgId", orgID, "agentId", billingAgent, "err", err)
			stream.Fail(apierror.CodeWorkforceSpendNotRecorded,
				"The turn finished, but its spend was not recorded. Check the billing setup before the next turn.")
			return
		}
	}

	if runErr != nil {
		stream.Fail("", runErr.Error())
		return
	}

	stream.Finish(p.Name(), model, turn.InputTokens, turn.OutputTokens, turn.ReasoningTokens)
}

func lastAssistantText(s *harness.Session) string {
	for i := len(s.History) - 1; i >= 0; i-- {
		if s.History[i].Role == "assistant" {
			return s.History[i].Content
		}
	}
	return ""
}

func newStreamID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

type chatStream struct {
	ctx     context.Context
	mu      sync.Mutex
	w       http.ResponseWriter
	flusher http.Flusher
	enc     *ssewriter.SSEWriter
	done    chan struct{}
	once    sync.Once
}

func newChatStream(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) *chatStream {
	c := &chatStream{
		ctx:     ctx,
		w:       w,
		flusher: flusher,
		enc:     ssewriter.NewSSEWriter(),
		done:    make(chan struct{}),
	}
	go c.keepAlive()
	return c
}

func (c *chatStream) emit(ev aguievents.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enc.WriteEvent(c.ctx, c.w, ev); err != nil {
		return err
	}
	if c.flusher != nil {
		c.flusher.Flush()
	}
	return nil
}

func (c *chatStream) keepAlive() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			if _, err := fmt.Fprint(c.w, ": ping\n\n"); err == nil && c.flusher != nil {
				c.flusher.Flush()
			}
			c.mu.Unlock()
		}
	}
}

func (c *chatStream) stop() {
	c.once.Do(func() { close(c.done) })
}
