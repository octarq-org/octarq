package plugin

import "context"

// CheckQuota consults the registered QuotaChecker, if any. It returns nil when
// no checker is registered — that is the self-hosted path and the normal one:
// a self-hosted install is unlimited by design, so an absent checker must be
// indistinguishable from an allowed check.
//
// A nil ctx or a host whose Lookup is unwired (an old host, or the MCP
// composition path) also reads as "no checker", never as an error or a panic.
func CheckQuota(ctx *Context, rctx context.Context, orgID uint, metric string, n int64) error {
	checker, ok := LookupAs[QuotaChecker](ctx, ServiceQuotaChecker)
	if !ok || checker == nil {
		return nil
	}
	return checker.Check(rctx, orgID, metric, n)
}
