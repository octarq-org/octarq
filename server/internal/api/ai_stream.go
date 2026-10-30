package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/llmprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

// readOnlyGuard wraps a base Guard (e.g. RiskGuard) and rejects any tool
// whose endpoint RiskLevel is not plugin.RiskLevelRead.
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
	risk := ep.EndpointRisk()
	if risk != plugin.RiskLevelRead {
		return fmt.Errorf("tool %q is not read-only (risk %s)", tool, risk)
	}
	return nil
}

// readOnlyTool keeps only the tools a read-only session may call. It mirrors
// the readOnlyGuard rule exactly, so the compiled prompt and the guard agree
// on the tool surface.
func readOnlyTool(d harness.ToolDescriptor) bool {
	return d.Risk == plugin.RiskLevelRead
}

// composeToolFilter combines the session policy with the spec's allowlist.
// The policy can narrow the advertised surface, the spec can narrow it
// further; neither can widen it.
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

// harnessSpec resolves the published harness spec of a workspace.
//
// A workspace with no published spec gets the built-in default — that is the
// product's normal state, not a fallback. A spec that fails validation is an
// error: it is never silently replaced by the default.
//
// A missing provider is an error too, and that is a different thing: it
// means the harness is not wired into this process, not that the workspace
// published nothing. Answering with the default there would run a harness
// nobody published, with no error and no log.
func (h *Handler) harnessSpec(ctx context.Context, orgID uint) (harness.HarnessSpec, error) {
	if h == nil || h.harnessSpecProvider == nil {
		return harness.HarnessSpec{}, errors.New("the agent harness is not configured on this instance")
	}
	spec, err := h.harnessSpecProvider(ctx, orgID)
	if err != nil {
		return harness.HarnessSpec{}, err
	}
	if err := harness.Validate(spec); err != nil {
		return harness.HarnessSpec{}, fmt.Errorf("published harness spec is invalid: %w", err)
	}
	return spec, nil
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
	spec harness.HarnessSpec,
	model string,
) {
	// The step list comes from the trace, not from the turn. The trace knows
	// the outcome of a step: a tool that ran and failed, a tool the policy
	// denied, a tool held for approval. The turn knows only that something
	// went wrong, so a dashboard built on the turn cannot tell a refusal
	// from a failure.
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

// spendWriteTimeout bounds the detached ledger write. The turn has already run
// when this happens, so a stalled database must not hold the stream open.
const spendWriteTimeout = 10 * time.Second

// recordTurnSpend writes one ledger row on a context that outlives the request.
// A client that closes the tab cancels r.Context(), but the turn already ran and
// the provider bills its tokens, so the spend must still reach the ledger.
func recordTurnSpend(parent context.Context, store *agentstore.WorkforceStore, orgID uint, in agentstore.SpendInput) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), spendWriteTimeout)
	defer cancel()
	_, _, err := store.RecordSpend(ctx, orgID, in)
	return err
}

// writeBillingRefusal answers a turn the agent budget cannot accept. Each
// message names the next action an operator can take, and each code lets the
// client translate that action instead of showing raw prose.
func writeBillingRefusal(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, agentstore.ErrAgentNotFound):
		writeErr(w, r, http.StatusNotFound, apierror.CodeWorkforceAgentNotFound, "No such workforce agent found in this workspace.")
	case errors.Is(err, agentstore.ErrAgentPaused):
		writeErr(w, r, http.StatusConflict, apierror.CodeWorkforceAgentPaused, "This agent is paused. Resume it before you run a turn.")
	case errors.Is(err, agentstore.ErrBudgetExceeded):
		writeErr(w, r, http.StatusConflict, apierror.CodeWorkforceBudgetExceeded, "This agent reached its monthly budget limit. Raise the limit before you run a turn.")
	case errors.Is(err, agentstore.ErrModelPriceMissing):
		writeErr(w, r, http.StatusBadRequest, apierror.CodeWorkforceModelPriceAbsent, "No price is configured for this model. Add a price before you bill agent turns.")
	default:
		writeErr(w, r, http.StatusInternalServerError, "", "Could not check the agent budget; retry the request.")
	}
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
	// AgentID attributes the turn to a workforce agent. An attributed turn is
	// priced and charged to that agent's budget. An absent value bills the
	// workspace default agent when one is set, and leaves the turn unbilled
	// when it is not.
	AgentID string `json:"agentId,omitempty"`
}

// SetEndpointSource configures the endpoint source for agent tool dispatch.
func (h *Handler) SetEndpointSource(src harness.EndpointSource) {
	h.endpointSource = src
}

// SetHarnessSpecProvider configures where the published agent harness spec of
// a workspace is resolved from. Without one the built-in default applies.
func (h *Handler) SetHarnessSpecProvider(fn func(ctx context.Context, orgID uint) (harness.HarnessSpec, error)) {
	h.harnessSpecProvider = fn
}

// SetHarnessStore configures the store that persists the published harness
// versions of a workspace.
func (h *Handler) SetHarnessStore(store *agentstore.HarnessStore) {
	h.harnessStore = store
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

	// Resolve the published harness spec before the response starts: an
	// invalid spec must fail the request, not degrade into the built-in
	// default halfway through a stream.
	spec, err := h.harnessSpec(r.Context(), orgID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "", "The workspace's agent harness spec is invalid. Ask an administrator to republish it.")
		return
	}

	// The model is resolved here so the billing gate sees the exact model the
	// runner will use.
	model := p.DefaultModel()
	if req.Model != "" {
		model = req.Model
	}

	// A turn attributed to a workforce agent is billed to that agent. The
	// price and the budget are checked before the stream starts: once the
	// response is open, a refusal can no longer become an HTTP error.
	//
	// A caller that names an agent spends workspace money, so it carries the
	// same authority as the workforce page that configures the agent. A turn
	// that names none bills the workspace default agent: an admin chose that
	// agent for every member, so a member needs no extra role to use it. When
	// the default cannot be read, the turn is refused rather than run free.
	var billingStore *agentstore.WorkforceStore
	billingAgent := req.AgentID
	if billingAgent != "" && !h.callerHoldsRole(r, authz.RoleAdmin) {
		writeErr(w, r, http.StatusForbidden, "", "Workspace permission denied; higher workspace role required")
		return
	}
	billingStore = h.getWorkforceStore()
	if billingStore == nil {
		writeErr(w, r, http.StatusInternalServerError, "", "The workforce store is not configured; retry the request.")
		return
	}
	if billingAgent == "" {
		defaultAgent, err := billingStore.GetDefaultAgent(r.Context(), orgID)
		if err != nil {
			writeErr(w, r, http.StatusInternalServerError, "", "Could not read the default agent; retry the request.")
			return
		}
		billingAgent = defaultAgent
	}
	if billingAgent != "" {
		if _, err := billingStore.AuthorizeTurn(r.Context(), orgID, billingAgent, model); err != nil {
			writeBillingRefusal(w, r, err)
			return
		}
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
	// The published spec narrows the tool surface, and the guard enforces it.
	// The prompt tells the model which tools exist; the guard is what makes
	// the promise true when the model decides otherwise.
	guard = harness.ComposeGuards(guard, harness.NewSpecGuard(spec))

	// Compile the tool catalog and the system prompt from the endpoint
	// registry. The web assistant is read-only, so only the read tools are
	// advertised: a prompt that offers a tool the guard refuses teaches the
	// model to call something it can never use.
	//
	// The spec is the published harness description. Until a workspace
	// publishes one, the built-in default applies; a spec that fails
	// validation never reaches this point.
	catalog := harness.BuildCatalog(h.endpointSource, composeToolFilter(readOnlyTool, spec))
	// The spec may only tighten the profile's step ceiling, and the prompt
	// must state the budget the runner will actually enforce.
	maxSteps := spec.EffectiveMaxSteps(harness.ProfileNormal.MaxSteps(false))
	spec = spec.WithMaxSteps(maxSteps)
	systemPrompt := catalog.SystemPrompt(spec)

	tracer := harness.NewRecordingTracer()
	adapter := &harness.CompleterAdapter{Provider: p}
	var runnerOpts []harness.RunnerOption
	runnerOpts = append(runnerOpts,
		harness.WithGuard(guard),
		harness.WithTracer(tracer),
		harness.WithProfile(harness.ProfileNormal),
		harness.WithSystem(systemPrompt),
		harness.WithMaxSteps(maxSteps),
	)
	if req.Model != "" {
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
	h.auditAgentTurn(r, orgID, session.ID, turn, tracer.Events(), catalog, spec, model)

	// An attributed turn is charged after it runs, including a turn that
	// failed partway: the provider bills the tokens it generated. A billing
	// failure is surfaced, not swallowed — an unrecorded turn would leave the
	// budget unaware of real spend.
	if billingAgent != "" {
		if err := recordTurnSpend(r.Context(), billingStore, orgID, agentstore.SpendInput{
			AgentID:         billingAgent,
			Model:           model,
			InputTokens:     turn.InputTokens,
			OutputTokens:    turn.OutputTokens,
			ReasoningTokens: turn.ReasoningTokens,
			Source:          models.WorkforceSpendSourceChat,
			SessionID:       session.ID,
		}); err != nil {
			slog.Error("failed to record agent spend", "orgId", orgID, "agentId", billingAgent, "err", err)
			_ = writeSSEPayload(w, flusher, "error", map[string]string{
				"code":    apierror.CodeWorkforceSpendNotRecorded,
				"message": "The turn finished, but its spend was not recorded. Check the billing setup before the next turn.",
			})
			return
		}
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
