package harness

import "context"

// Guard decides whether a tool invocation is allowed before execution.
// Returning a non-nil error vetoes the step; the error message is
// preserved as guidance in Step.Err.
//
// argsJSON is the canonical JSON object of the call's arguments. A guard that
// only needs the tool name ignores it; a guard that decides on what the call
// would do — which path it touches, which host it contacts — cannot, and
// refusing to pass it forced those rules into the tool implementation, after
// the point of no return.
//
// P3 前 API 易变.
type Guard interface {
	Allow(ctx context.Context, orgID uint, tool, argsJSON string) error
}

// NopGuard permits every invocation unconditionally.
type NopGuard struct{}

// Allow always returns nil (permit).
func (NopGuard) Allow(context.Context, uint, string, string) error { return nil }
