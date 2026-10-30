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
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/authz"
	"github.com/octarq-org/octarq/server/internal/models"
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

// AIChatStreamMessage is one message of the run input. Content stays raw so a
// structured payload is refused with a clear reason instead of a decode error.
type AIChatStreamMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// AIChatStreamRequest is the request body for POST /api/ai/chat/stream.
//
// The body follows the AG-UI run input (threadId, runId, messages) and adds
// the workspace options this product owns. There is deliberately no
// system-prompt field: the prompt is compiled on the server from the endpoint
// registry, so a caller cannot replace the agent's rules — not the tool-call
// format, not the untrusted-data rule, not the tool list.
//
// Anything the server would ignore is refused rather than dropped: a caller
// that sends client tools or run state learns that the server does not use
// them, instead of believing it changed the run.
type AIChatStreamRequest struct {
	ThreadID string                `json:"threadId"`
	RunID    string                `json:"runId"`
	Messages []AIChatStreamMessage `json:"messages"`

	Model    string `json:"model,omitempty"`
	Thinking *bool  `json:"thinking,omitempty"`
	// AgentID attributes the turn to a workforce agent. An attributed turn is
	// priced and charged to that agent's budget. An absent value bills the
	// workspace default agent when one is set, and leaves the turn unbilled
	// when it is not.
	AgentID string `json:"agentId,omitempty"`

	Tools          json.RawMessage `json:"tools,omitempty"`
	State          json.RawMessage `json:"state,omitempty"`
	Context        json.RawMessage `json:"context,omitempty"`
	ForwardedProps json.RawMessage `json:"forwardedProps,omitempty"`
	Resume         json.RawMessage `json:"resume,omitempty"`
}

// emptyJSON reports whether a raw field carries no value. Null, an empty
// array and an empty object are all "nothing"; anything else is a payload the
// server would otherwise drop.
func emptyJSON(raw json.RawMessage) bool {
	switch strings.TrimSpace(string(raw)) {
	case "", "null", "[]", "{}":
		return true
	}
	return false
}

// parseRunInput validates the messages and splits them into history and the
// turn input. The error string is the sentence the caller sees.
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
//
// Each SSE frame is one AG-UI event: an optional id line and one data line.
// The path stays inside this product's own namespace: AG-UI is the wire
// format, not the endpoint name.
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

	// Fail closed on run input this server does not use. A field that is
	// silently dropped teaches a client that it controlled the run.
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

	var runErr error
	if sr, ok := runner.(harness.StreamRunner); ok {
		runErr = sr.Stream(r.Context(), session, turn, stream)
	} else {
		runErr = runner.Run(r.Context(), session, turn)
	}

	// A provider without streaming support still owes the client an answer.
	if !stream.WroteText() && runErr == nil {
		stream.Say(lastAssistantText(session))
	}

	// Record what the agent did before answering: a turn that failed is
	// exactly the one an operator will want to look at later.
	h.auditAgentTurn(r, orgID, session.ID, turn, tracer.Events(), catalog, spec, model)

	// A failed sink means the client is gone; there is nobody left to answer.
	if stream.Err() != nil {
		return
	}

	// An attributed turn is charged after it runs, including a turn that
	// failed partway: the provider bills the tokens it generated. The run id
	// is the idempotency key, so a retried request never charges twice. A
	// billing failure is surfaced, not swallowed — an unrecorded turn would
	// leave the budget unaware of real spend.
	if billingAgent != "" {
		err := recordTurnSpend(r.Context(), billingStore, orgID, agentstore.SpendInput{
			AgentID:         billingAgent,
			Model:           model,
			InputTokens:     turn.InputTokens,
			OutputTokens:    turn.OutputTokens,
			ReasoningTokens: turn.ReasoningTokens,
			Source:          models.WorkforceSpendSourceChat,
			SessionID:       threadID,
			TurnID:          runID,
		})
		if err != nil && !errors.Is(err, agentstore.ErrSpendReplayed) {
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

// lastAssistantText returns the final assistant reply of the session, used
// when the provider could not stream its deltas.
func lastAssistantText(s *harness.Session) string {
	for i := len(s.History) - 1; i >= 0; i-- {
		if s.History[i].Role == "assistant" {
			return s.History[i].Content
		}
	}
	return ""
}

// newStreamID builds an identifier for a caller that sent none.
func newStreamID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

// chatStream writes AG-UI events as SSE frames. The keep-alive comment keeps
// a proxy from closing a run that spends a long time inside one tool.
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
