package ontology

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm/clause"
)

// findLimit caps a Find. A caller uses it to answer "is this row already there",
// not to page a table: the REST surface owns paging.
const findLimit = 100

// recordWriter is the seam a module writes through. It holds the plugin so every
// call reaches the same declaration set, store and tenant handle the REST
// surface uses.
type recordWriter struct{ p *Plugin }

var _ core.RecordWriter = recordWriter{}

// entity resolves a slug against what the boot applied. A slug nobody declared
// is refused instead of being turned into a table name.
func (w recordWriter) entity(slug string) (*entity, error) {
	for _, e := range w.p.entities {
		if e.spec.Slug == slug {
			return e, nil
		}
	}
	return nil, fmt.Errorf("ontology: no entity is declared as %q", slug)
}

// tenant returns the handle for one workspace. The host builds it, so the org
// predicate is the platform's and never assembled from a caller value.
func (w recordWriter) tenant(orgID uint) (*plugin.TenantDB, error) {
	if w.p.host == nil {
		return nil, ErrNoTenantScope
	}
	tdb := w.p.host.TenantDB(orgID)
	if tdb == nil {
		return nil, ErrNoTenantScope
	}
	return tdb, nil
}

// Create stores one row. The values pass through the same decoder the REST
// create uses, so a reserved key, an undeclared field or a wrong type is refused
// before an INSERT is built.
func (w recordWriter) Create(ctx context.Context, orgID uint, slug string, values map[string]any) (map[string]any, error) {
	e, err := w.entity(slug)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("ontology: create %s: %w", slug, err)
	}
	checked, err := core.DecodeValues(e.spec, raw)
	if err != nil {
		return nil, err
	}
	tdb, err := w.tenant(orgID)
	if err != nil {
		return nil, err
	}
	row, err := (store{}).create(ctx, tdb, e, checked)
	if err != nil {
		return nil, err
	}
	return core.FromRow(e.spec, row), nil
}

// Update applies a patch. Omitted fields keep their value, and a row of another
// workspace is ErrNotFound.
func (w recordWriter) Update(ctx context.Context, orgID uint, slug, id string, patch map[string]any) (map[string]any, error) {
	e, err := w.entity(slug)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("ontology: update %s: %w", slug, err)
	}
	checked, err := core.DecodePatch(e.spec, raw)
	if err != nil {
		return nil, err
	}
	tdb, err := w.tenant(orgID)
	if err != nil {
		return nil, err
	}
	row, err := (store{}).update(ctx, tdb, e, id, checked)
	if err != nil {
		return nil, err
	}
	return core.FromRow(e.spec, row), nil
}

// Get reads one row of one workspace.
func (w recordWriter) Get(ctx context.Context, orgID uint, slug, id string) (map[string]any, error) {
	e, err := w.entity(slug)
	if err != nil {
		return nil, err
	}
	tdb, err := w.tenant(orgID)
	if err != nil {
		return nil, err
	}
	row, err := (store{}).get(ctx, tdb, e, id)
	if err != nil {
		return nil, err
	}
	return core.FromRow(e.spec, row), nil
}

// Find returns the rows whose declared field equals value, newest first. The
// field is checked against the declaration, so a caller cannot name a column the
// declaration does not have.
func (w recordWriter) Find(ctx context.Context, orgID uint, slug, field string, value any) ([]map[string]any, error) {
	e, err := w.entity(slug)
	if err != nil {
		return nil, err
	}
	if field != core.ColumnID && !e.declares(field) {
		return nil, fmt.Errorf("ontology: find %s: %q is not a declared field", slug, field)
	}
	tdb, err := w.tenant(orgID)
	if err != nil {
		return nil, err
	}
	scope, _, err := tenantScope(ctx, tdb, e)
	if err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	err = scope.WithContext(ctx).
		Where(clause.Eq{Column: field, Value: value}).
		Order(orderClause(nil)).
		Limit(findLimit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("ontology: find %s: %w", e.table, err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, core.FromRow(e.spec, row))
	}
	return out, nil
}
