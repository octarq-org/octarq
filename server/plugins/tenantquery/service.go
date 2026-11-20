package tenantquery

import (
	"context"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	registry *tenantsql.Registry
}

func NewService(db *gorm.DB, reg *tenantsql.Registry) *Service {
	if reg == nil {
		reg = tenantsql.DefaultRegistry()
	}
	return &Service{
		db:       db,
		registry: reg,
	}
}

func HasScopeColumn(view plugin.TenantView) bool {
	return tenantsql.HasScopeColumn(view)
}

func (s *Service) EnforceScopeSQL(ctx context.Context, rawSQL string) (string, bool, error) {
	reg := s.registry
	if reg == nil {
		reg = tenantsql.DefaultRegistry()
	}

	refNames, err := tenantsql.ExtractReferencedViews(rawSQL)
	if err != nil {
		return rawSQL, false, err
	}

	hasScopedView := false
	for _, name := range refNames {
		if view, ok := reg.Lookup(name); ok && HasScopeColumn(view) {
			hasScopedView = true
			break
		}
	}

	if !hasScopedView || plugin.AllowsPersonalScope(ctx) {
		return rawSQL, hasScopedView, nil
	}

	return injectWorkScopePredicate(rawSQL, refNames, reg), true, nil
}

func injectWorkScopePredicate(rawSQL string, refNames []string, reg *tenantsql.Registry) string {
	trimmed := strings.TrimSpace(rawSQL)
	trimmed = strings.TrimRight(trimmed, ";")
	if len(refNames) != 1 {
		return trimmed
	}
	view, ok := reg.Lookup(refNames[0])
	if !ok || !HasScopeColumn(view) {
		return trimmed
	}

	upper := strings.ToUpper(trimmed)
	for _, kw := range []string{" GROUP ", " ORDER ", " LIMIT ", " OFFSET ", " UNION ", " HAVING "} {
		if strings.Contains(upper, kw) {
			return trimmed
		}
	}

	if strings.Contains(upper, " WHERE ") {
		idx := strings.Index(upper, " WHERE ")
		prefix := trimmed[:idx+len(" WHERE ")]
		cond := trimmed[idx+len(" WHERE "):]
		return fmt.Sprintf("%s(scope_key LIKE 'work:%%') AND (%s)", prefix, cond)
	}
	return fmt.Sprintf("%s WHERE scope_key LIKE 'work:%%'", trimmed)
}

func (s *Service) Query(ctx context.Context, in QueryTenantSQLInput) (*QueryTenantSQLOutput, error) {
	reg := s.registry
	if reg == nil {
		reg = tenantsql.DefaultRegistry()
	}

	effectiveSQL, hasScopedView, err := s.EnforceScopeSQL(ctx, in.SQL)
	if err != nil {
		return nil, s.wrapErrorWithAvailableViews(reg, err)
	}

	rows, meta, err := tenantsql.Execute(ctx, s.db, reg, effectiveSQL)
	if err != nil {
		return nil, s.wrapErrorWithAvailableViews(reg, err)
	}

	if hasScopedView && !plugin.AllowsPersonalScope(ctx) && len(rows) > 0 {
		filtered := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			if rawScope, exists := r["scope_key"]; exists {
				scopeStr := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawScope)))
				if !strings.HasPrefix(scopeStr, "work:") {
					continue
				}
			}
			filtered = append(filtered, r)
		}
		rows = filtered
		meta.RowCount = len(rows)
	}

	if rows == nil {
		rows = []map[string]any{}
	}

	return &QueryTenantSQLOutput{
		Rows:      rows,
		RowCount:  meta.RowCount,
		Truncated: meta.Truncated,
	}, nil
}

func (s *Service) wrapErrorWithAvailableViews(reg *tenantsql.Registry, err error) error {
	views := reg.List()
	viewNames := make([]string, len(views))
	for i, v := range views {
		viewNames[i] = v.Name
	}
	availMsg := fmt.Sprintf("available views: %s", strings.Join(viewNames, ", "))
	return fmt.Errorf("%w (%s)", err, availMsg)
}
