package harness

import "context"

type Guard interface {
	Allow(ctx context.Context, orgID uint, tool, argsJSON string) error
}

type NopGuard struct{}

func (NopGuard) Allow(context.Context, uint, string, string) error { return nil }
