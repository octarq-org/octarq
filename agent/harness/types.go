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

// Step records one tool invocation within a turn.
type Step struct {
	ToolName     string
	ArgsJSON     string
	OutputFenced string // tool output wrapped in untrusted fence
	Err          error
	DurationMS   int64
}

// Turn is one "input → reasoning → tool calls → output" cycle.
type Turn struct {
	Input           string
	Steps           []Step
	Status          TurnStatus
	Thinking        bool
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
}
