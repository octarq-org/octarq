// Package harness implements the AI-native orchestration micro-kernel.
// It provides the agent loop, safety guard, and trajectory tracing
// infrastructure that all channels (Web, Telegram, MCP, CLI) share.
//
// API stability: P3 前 API 易变 — exported interfaces and value types are the
// public surface (invariant #11); implementation details may change.
package harness

// Message is a single conversational turn within a session history.
// It mirrors the shape of llmprovider.Message but is defined locally
// to keep this package self-contained.
type Message struct {
	Role    string // "user" | "assistant" | "tool"
	Content string
}

// Session is the persistent unit of a multi-turn conversation.
// The harness only defines the shape; persistence is the caller's concern.
type Session struct {
	ID      string
	OrgID   uint
	Channel string
	History []Message
}

// TurnStatus tracks the lifecycle of a single turn through the agent loop.
type TurnStatus int

const (
	TurnStatusRunning          TurnStatus = iota // loop is executing steps
	TurnStatusAwaitingApproval                   // blocked on HITL approval
	TurnStatusDone                               // completed successfully
	TurnStatusFailed                             // terminated with error
)

// String returns the stable, machine-readable name of the status. Traces and
// audit records store this name, not the enum's ordinal.
func (s TurnStatus) String() string {
	switch s {
	case TurnStatusRunning:
		return "running"
	case TurnStatusAwaitingApproval:
		return "awaiting_approval"
	case TurnStatusDone:
		return "done"
	case TurnStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Step records one tool invocation within a turn.
type Step struct {
	ToolName     string
	ArgsJSON     string
	OutputFenced string // tool output wrapped in untrusted fence
	Err          error
	DurationMS   int64
}

// PendingCall is the tool call an operator was asked to approve.
//
// It records the call the guard refused, with the arguments canonicalised and
// hashed, so an approval binds to that exact call. Without it the approval
// applied to the next call the model happened to produce, and the model
// regenerates its arguments: the operator approved one command and a different
// one ran.
type PendingCall struct {
	// Tool is the tool the model asked to call.
	Tool string
	// ArgsJSON is the canonical JSON object of the arguments: the same call
	// always renders the same bytes, so two approvals of one call are one hash.
	ArgsJSON string
	// Hash identifies Tool and ArgsJSON together. A caller that stores an
	// approval records this value and passes it back with the decision, so a
	// decision cannot be replayed against another call.
	Hash string
}

// Turn is one "input → reasoning → tool calls → output" cycle.
type Turn struct {
	Input           string
	Steps           []Step
	Status          TurnStatus
	Thinking        bool
	HasDestructive  bool
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	// PendingCall is the call the loop is waiting for an operator to approve.
	// It is nil while the turn runs, and Resume takes it as the call to run.
	PendingCall *PendingCall

	// rounds counts the rounds this turn has used, so a resumed turn continues
	// the same step budget instead of starting a fresh one. An approval must
	// not buy extra rounds.
	rounds int
}
