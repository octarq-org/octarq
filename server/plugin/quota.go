package plugin

import "context"

func CheckQuota(ctx *Context, rctx context.Context, orgID uint, metric string, n int64) error {
	checker, ok := LookupAs[QuotaChecker](ctx, ServiceQuotaChecker)
	if !ok || checker == nil {
		return nil
	}
	return checker.Check(rctx, orgID, metric, n)
}
