package plugin

import (
	"context"
)

// ServiceDataScope is the well-known service name under which the data scope
// filter SPI is provided (contract type DataScopeFilter).
const ServiceDataScope = "datascope.filter"

// DataScopeFilter defines the SPI contract for scoping GORM and SQL database
// queries to tenant and user boundaries. Pro or business plugins can provide
// custom implementations (e.g. department tree, role hierarchy) to override
// the default single-layer organization filtering.
type DataScopeFilter interface {
	// ApplyScope 向 GORM/SQL 查询附加数据权限过滤条件
	// scope 参数由调用方传入当前用户的权限范围
	ApplyScope(ctx context.Context, query interface{}, userID string, orgID string) (interface{}, error)
}

// GetDataScopeFilter resolves the registered DataScopeFilter from the context lookup.
func GetDataScopeFilter(ctx *Context) (DataScopeFilter, bool) {
	if ctx == nil {
		return nil, false
	}
	return LookupAs[DataScopeFilter](ctx, ServiceDataScope)
}
