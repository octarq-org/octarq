package ontology

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("ontology: record not found")

var ErrNoTenantScope = errors.New("ontology: no workspace in this request")

var ErrNoUserScope = errors.New("ontology: this transport carries no user")

var ErrNoDatabase = errors.New("ontology: the host provided no database")

var ErrNoRole = errors.New("ontology: this transport carries no role")

var ErrUnknownSort = errors.New("ontology: unknown sort field")

const maxPageSize = 200

const defaultPageSize = 50

type store struct{}

type listParams struct {
	limit  int
	offset int
	sort   []string
}

func (store) list(ctx context.Context, tdb *plugin.TenantDB, e *entity, p listParams) ([]map[string]any, int64, error) {
	countQ, _, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := countQ.WithContext(ctx).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("ontology: count %s: %w", e.table, err)
	}

	scope, _, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return nil, 0, err
	}
	rows := []map[string]any{}
	err = scope.WithContext(ctx).
		Order(orderClause(p.sort)).
		Limit(p.limit).
		Offset(p.offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("ontology: list %s: %w", e.table, err)
	}
	return rows, total, nil
}

func (store) get(ctx context.Context, tdb *plugin.TenantDB, e *entity, id string) (map[string]any, error) {
	scope, _, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	if err := scope.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ontology: read %s: %w", e.table, err)
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}

func (store) create(ctx context.Context, tdb *plugin.TenantDB, e *entity, values map[string]any) (map[string]any, error) {
	scope, userID, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now().UTC()

	row := make(map[string]any, len(values)+5)
	for k, v := range values {
		row[k] = v
	}
	row[core.ColumnID] = id
	row[core.ColumnOrgID] = tdb.OrgID()
	if core.ScopeOf(e.spec) == core.ScopeUser {
		row[core.ColumnUserID] = userID
	}
	row[core.ColumnCreatedAt] = now
	row[core.ColumnUpdatedAt] = now

	if err := scope.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("ontology: create %s: %w", e.table, err)
	}
	return (store{}).get(ctx, tdb, e, id)
}

func (store) update(ctx context.Context, tdb *plugin.TenantDB, e *entity, id string, patch map[string]any) (map[string]any, error) {
	if len(patch) == 0 {
		return (store{}).get(ctx, tdb, e, id)
	}
	scope, _, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return nil, err
	}
	values := make(map[string]any, len(patch)+1)
	for k, v := range patch {
		values[k] = v
	}
	values[core.ColumnUpdatedAt] = time.Now().UTC()

	res := scope.WithContext(ctx).Where("id = ?", id).Updates(values)
	if res.Error != nil {
		return nil, fmt.Errorf("ontology: update %s: %w", e.table, res.Error)
	}
	if res.RowsAffected == 0 {
		return (store{}).get(ctx, tdb, e, id)
	}
	return (store{}).get(ctx, tdb, e, id)
}

func (store) delete(ctx context.Context, tdb *plugin.TenantDB, e *entity, id string) error {
	scope, _, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return err
	}
	res := scope.WithContext(ctx).Where("id = ?", id).Delete(nil)
	if res.Error != nil {
		return fmt.Errorf("ontology: delete %s: %w", e.table, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func tenantScope(ctx context.Context, tdb *plugin.TenantDB, e *entity) (*gorm.DB, uint, error) {
	if tdb == nil || tdb.OrgID() == 0 {
		return nil, 0, ErrNoTenantScope
	}
	if e.featureOff(tdb.OrgID()) {
		return nil, 0, ErrFeatureOff
	}
	if core.ScopeOf(e.spec) != core.ScopeUser {
		scope, err := tdb.TableScoped(e.table, core.ColumnOrgID)
		return scope, 0, err
	}
	userID := plugin.UserIDFromContext(ctx)
	if userID == 0 {
		return nil, 0, ErrNoUserScope
	}
	scope, err := tdb.TableScopedBy(e.table, core.ColumnOrgID, plugin.Scope{
		Column: core.ColumnUserID,
		Value:  userID,
	})
	return scope, userID, err
}

func orderClause(sort []string) string {
	if len(sort) == 0 {
		return core.ColumnCreatedAt + " DESC"
	}
	parts := make([]string, 0, len(sort))
	for _, s := range sort {
		desc := strings.HasPrefix(s, "-")
		name := strings.TrimPrefix(s, "-")
		if desc {
			parts = append(parts, name+" DESC")
			continue
		}
		parts = append(parts, name+" ASC")
	}
	return strings.Join(parts, ", ")
}

func parseSort(e *entity, raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !e.sortable(strings.TrimPrefix(name, "-")) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownSort, name)
		}
		out = append(out, name)
	}
	return out, nil
}
