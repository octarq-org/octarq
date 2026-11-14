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

// ErrNotFound is returned for a record that does not exist in the caller's
// workspace. A record belonging to another workspace reaches this same error on
// purpose: the tenant predicate hides it, so the answer cannot confirm that the
// id exists somewhere else.
var ErrNotFound = errors.New("ontology: record not found")

// ErrNoTenantScope is returned when a request arrives without a workspace. The
// caller is anonymous or the transport carries no org, and a generated table has
// no meaning without one.
var ErrNoTenantScope = errors.New("ontology: no workspace in this request")

// ErrNoUserScope is returned when a user-scoped entity is reached over a
// transport that carries no user: stdio MCP stamps a workspace and no identity.
// Reading the whole workspace instead would hand one member another's rows.
var ErrNoUserScope = errors.New("ontology: this transport carries no user")

// ErrNoDatabase is returned when the composition gave the plugin no database at
// all. It is not an authentication problem, so it is not reported as one.
var ErrNoDatabase = errors.New("ontology: the host provided no database")

// ErrNoRole is returned when the transport carries no role at all, which is the
// stdio composition: it stamps a workspace but no identity. It is refused like
// any other missing grant, and told apart from a role that is simply too low so
// the caller learns which of the two it can fix.
var ErrNoRole = errors.New("ontology: this transport carries no role")

// ErrUnknownSort is returned when a caller sorts by something that is not a
// declared field.
var ErrUnknownSort = errors.New("ontology: unknown sort field")

// maxPageSize caps a list page. It is a platform budget, not part of the
// declaration, and both surfaces refuse a caller that asks for more.
const maxPageSize = 200

// defaultPageSize is the page a caller gets when it does not ask.
const defaultPageSize = 50

// store is the only code that writes to a generated table. Every method takes a
// tenant-scoped handle, so the org predicate is the platform's, never a value
// assembled here.
type store struct{}

// listParams is a parsed, already-validated list request.
type listParams struct {
	limit  int
	offset int
	sort   []string
}

// list returns one page plus the total the workspace holds.
func (store) list(ctx context.Context, tdb *plugin.TenantDB, e *entity, p listParams) ([]map[string]any, int64, error) {
	// Count on its own handle: reusing one handle across Count and Find makes
	// GORM carry the count's SELECT list into the row query.
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

// get reads one record. ErrNotFound covers both "absent" and "another tenant's".
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

// create inserts one record and returns the stored values.
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
	// The scope predicates TableScopedBy attaches apply to SELECT, UPDATE and
	// DELETE; GORM drops the WHERE clause when it builds an INSERT. The columns
	// are therefore written here, from the handle and the request context, and
	// never from the request body — the reserved-key check upstream is what
	// guarantees the latter.
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

// update applies a patch and returns the stored values. A patch that matches no
// row in the caller's workspace is ErrNotFound.
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
		// The row exists but nothing changed, or it is not ours. Read it back to
		// tell those apart without leaking which one it was.
		return (store{}).get(ctx, tdb, e, id)
	}
	return (store{}).get(ctx, tdb, e, id)
}

// delete removes one record. Delete returns no error for a miss, so the count is
// the answer.
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

// tenantScope is the one place the isolation predicates are attached, and the
// one place a user id is read. Always called with the entity's own table, so a
// handler cannot point it at another table. The user id it returns is the same
// value it bound, so the row an insert writes and the predicate a read applies
// cannot disagree.
func tenantScope(ctx context.Context, tdb *plugin.TenantDB, e *entity) (*gorm.DB, uint, error) {
	if tdb == nil || tdb.OrgID() == 0 {
		return nil, 0, ErrNoTenantScope
	}
	// A feature the workspace switched off is not a permissions problem and not
	// an absent record: the page, its menu entry and its tools are gone, and so
	// is the data behind them. Every operation reaches this line, so no transport
	// can be the one that forgot.
	if e.featureOff(tdb.OrgID()) {
		return nil, 0, ErrFeatureOff
	}
	if core.ScopeOf(e.spec) != core.ScopeUser {
		scope, err := tdb.TableScoped(e.table, core.ColumnOrgID)
		return scope, 0, err
	}
	// A user-scoped row belongs to the caller. The value comes from the request
	// context, never from the body: a caller that could name the owner could read
	// another member's rows.
	userID := plugin.UserIDFromContext(ctx)
	if userID == 0 {
		// stdio MCP stamps a workspace and no identity, and an unauthenticated
		// path has neither. Refuse rather than fall back to the whole workspace.
		return nil, 0, ErrNoUserScope
	}
	scope, err := tdb.TableScopedBy(e.table, core.ColumnOrgID, plugin.Scope{
		Column: core.ColumnUserID,
		Value:  userID,
	})
	return scope, userID, err
}

// orderClause renders the declared sort. Every name was checked against the
// declaration before it got here, so nothing caller-supplied reaches the SQL.
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

// parseSort splits a comma-separated sort expression and refuses any name the
// declaration does not contain. An unknown column would otherwise be an ORDER BY
// the database resolves, which is how a sort parameter becomes injection.
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
