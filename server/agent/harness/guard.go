package harness

import "context"

// Guard decides whether a tool invocation is allowed before execution.
// Returning a non-nil error vetoes the step; the error message is
// preserved as guidance in Step.Err.
//
// P3 前 API 易变.
type Guard interface {
	Allow(ctx context.Context, orgID uint, tool string) error
}

// NopGuard permits every invocation unconditionally.
type NopGuard struct{}

// Allow always returns nil (permit).
func (NopGuard) Allow(context.Context, uint, string) error { return nil }
