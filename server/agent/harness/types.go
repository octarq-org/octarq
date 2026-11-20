package harness

import "time"

type Message struct {
	Role    string
	Content string
}

type Session struct {
	ID      string
	OrgID   uint
	Channel string
	History []Message
}

type TurnStatus int

const (
	TurnStatusRunning TurnStatus = iota
	TurnStatusAwaitingApproval
	TurnStatusDone
	TurnStatusFailed
)

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

type Step struct {
	ToolName     string
	ArgsJSON     string
	OutputFenced string
	Err          error
	DurationMS   int64
}

type PendingCall struct {
	Tool     string
	ArgsJSON string
	Hash     string
}

type Turn struct {
	Input           string
	Steps           []Step
	Status          TurnStatus
	Thinking        bool
	HasDestructive  bool
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	PendingCall     *PendingCall

	Rounds int

	Elapsed time.Duration
}
