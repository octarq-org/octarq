package plugin

import (
	"context"
)

const ServiceDataScope = "datascope.filter"

type DataScopeFilter interface {
	ApplyScope(ctx context.Context, query interface{}, userID string, orgID string) (interface{}, error)
}

func GetDataScopeFilter(ctx *Context) (DataScopeFilter, bool) {
	if ctx == nil {
		return nil, false
	}
	return LookupAs[DataScopeFilter](ctx, ServiceDataScope)
}
