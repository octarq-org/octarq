package tenantsql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNilDB = errors.New("tenantsql: database handle cannot be nil")

	ErrZeroOrgID = errors.New("tenantsql: tenant org ID must be non-zero")

	ErrMissingTenantContext = errors.New("tenantsql: missing tenant context in request")

	ErrTenantMismatch = errors.New("tenantsql: entity tenant ID does not match bound tenant scope")

	ErrInvalidTable = errors.New("tenantsql: invalid table name")

	ErrInvalidTenantColumn = errors.New("tenantsql: invalid tenant column")

	ErrInvalidScopeColumn = errors.New("tenantsql: invalid scope column")

	ErrEmptyScopeValue = errors.New("tenantsql: scope value must not be zero")
)

const DefaultTenantColumn = "owner_id"

type TenantScoped interface {
	TenantColumn() string
}

type TenantDBOption func(*tenantDBOptions)

type tenantDBOptions struct {
	defaultColumn string
	registry      *Registry
}

func WithTenantColumn(col string) TenantDBOption {
	return func(opts *tenantDBOptions) {
		if trimmed := strings.TrimSpace(col); trimmed != "" {
			opts.defaultColumn = trimmed
		}
	}
}

func WithTenantRegistry(reg *Registry) TenantDBOption {
	return func(opts *tenantDBOptions) {
		opts.registry = reg
	}
}

type TenantDB struct {
	db            *gorm.DB
	orgID         uint
	defaultColumn string
	registry      *Registry
	ctx           context.Context
}

func NewTenantDB(db *gorm.DB, orgID uint, opts ...TenantDBOption) (*TenantDB, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	if orgID == 0 {
		return nil, ErrZeroOrgID
	}

	o := &tenantDBOptions{
		defaultColumn: DefaultTenantColumn,
		registry:      DefaultRegistry(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}

	return &TenantDB{
		db:            db,
		orgID:         orgID,
		defaultColumn: o.defaultColumn,
		registry:      o.registry,
		ctx:           db.Statement.Context,
	}, nil
}

func TenantDBFromContext(ctx context.Context, db *gorm.DB, opts ...TenantDBOption) (*TenantDB, error) {
	if ctx == nil {
		return nil, ErrMissingTenantContext
	}
	orgID := OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, ErrMissingTenantContext
	}
	tdb, err := NewTenantDB(db, orgID, opts...)
	if err != nil {
		return nil, err
	}
	tdb.ctx = ctx
	tdb.db = tdb.db.WithContext(ctx)
	return tdb, nil
}

func (t *TenantDB) OrgID() uint {
	return t.orgID
}

func (t *TenantDB) DefaultColumn() string {
	return t.defaultColumn
}

func (t *TenantDB) WithContext(ctx context.Context) *TenantDB {
	cp := *t
	cp.ctx = ctx
	if t.db != nil {
		cp.db = t.db.WithContext(ctx)
	}
	return &cp
}

func (t *TenantDB) WithColumn(col string) *TenantDB {
	trimmed := strings.TrimSpace(col)
	if trimmed == "" {
		trimmed = DefaultTenantColumn
	}
	cp := *t
	cp.defaultColumn = trimmed
	return &cp
}

func (t *TenantDB) ResolveColumn(model any) string {
	if model == nil {
		return t.defaultColumn
	}
	if ts, ok := model.(TenantScoped); ok {
		if col := ts.TenantColumn(); col != "" {
			return col
		}
	}
	return inspectModelColumn(model, t.defaultColumn)
}

func inspectModelColumn(model any, fallback string) string {
	val := reflect.ValueOf(model)
	for val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return fallback
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Slice {
		elemType := val.Type().Elem()
		for elemType.Kind() == reflect.Pointer {
			elemType = elemType.Elem()
		}
		if elemType.Kind() == reflect.Struct {
			return inspectStructColumn(elemType, fallback)
		}
		return fallback
	}
	if val.Kind() == reflect.Struct {
		return inspectStructColumn(val.Type(), fallback)
	}
	return fallback
}

func inspectStructColumn(st reflect.Type, fallback string) string {
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		tag := f.Tag.Get("gorm")
		if strings.Contains(tag, "column:owner_id") {
			return "owner_id"
		}
		if strings.Contains(tag, "column:org_id") {
			return "org_id"
		}
	}
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		tag := f.Tag.Get("gorm")
		if strings.Contains(tag, "-") {
			continue
		}
		if f.Name == "OwnerID" {
			return "owner_id"
		}
		if f.Name == "OrgID" && !strings.Contains(tag, "column:") {
			return "org_id"
		}
	}
	return fallback
}

func (t *TenantDB) Scoped(model ...any) *gorm.DB {
	tx := t.db
	if len(model) > 0 && model[0] != nil {
		m := model[0]
		col := t.ResolveColumn(m)
		return tx.Model(m).Where(fmt.Sprintf("%s = ?", col), t.orgID)
	}
	return tx.Where(fmt.Sprintf("%s = ?", t.defaultColumn), t.orgID)
}

func (t *TenantDB) Model(value any) *gorm.DB {
	return t.Scoped(value)
}

func (t *TenantDB) Table(name string, args ...any) *gorm.DB {
	tx := t.db.Table(name, args...)
	return tx.Where(fmt.Sprintf("%s = ?", t.defaultColumn), t.orgID)
}

type Scope struct {
	Column string
	Value  any
}

func (t *TenantDB) TableScoped(table, tenantColumn string) (*gorm.DB, error) {
	return t.TableScopedBy(table, tenantColumn)
}

func (t *TenantDB) TableScopedBy(table, tenantColumn string, extra ...Scope) (*gorm.DB, error) {
	if t == nil || t.db == nil {
		return nil, ErrNilDB
	}
	table = strings.TrimSpace(table)
	tenantColumn = strings.TrimSpace(tenantColumn)
	if !validIdentPattern.MatchString(table) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidTable, table)
	}
	if tenantColumn == "" || !validIdentPattern.MatchString(tenantColumn) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidTenantColumn, tenantColumn)
	}

	query := t.db.Table(table).Where(clause.Eq{
		Column: clause.Column{Name: tenantColumn},
		Value:  t.orgID,
	})
	for _, s := range extra {
		column := strings.TrimSpace(s.Column)
		if column == "" || !validIdentPattern.MatchString(column) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidScopeColumn, s.Column)
		}
		if isZeroScopeValue(s.Value) {
			return nil, fmt.Errorf("%w: %q", ErrEmptyScopeValue, column)
		}
		query = query.Where(clause.Eq{
			Column: clause.Column{Name: column},
			Value:  s.Value,
		})
	}
	return query, nil
}

func isZeroScopeValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.IsZero()
	default:
		return false
	}
}

func (t *TenantDB) Where(query any, args ...any) *gorm.DB {
	return t.Scoped().Where(query, args...)
}

func (t *TenantDB) Select(query any, args ...any) *gorm.DB {
	return t.Scoped().Select(query, args...)
}

func (t *TenantDB) Order(value any) *gorm.DB {
	return t.Scoped().Order(value)
}

func (t *TenantDB) Limit(limit int) *gorm.DB {
	return t.Scoped().Limit(limit)
}

func (t *TenantDB) Offset(offset int) *gorm.DB {
	return t.Scoped().Offset(offset)
}

func (t *TenantDB) Joins(query string, args ...any) *gorm.DB {
	return t.Scoped().Joins(query, args...)
}

func (t *TenantDB) Preload(query string, args ...any) *gorm.DB {
	return t.Scoped().Preload(query, args...)
}

func (t *TenantDB) Omit(columns ...string) *gorm.DB {
	return t.Scoped().Omit(columns...)
}

func (t *TenantDB) Distinct(args ...any) *gorm.DB {
	return t.Scoped().Distinct(args...)
}

func (t *TenantDB) Clauses(conds ...clause.Expression) *gorm.DB {
	return t.Scoped().Clauses(conds...)
}

func (t *TenantDB) Find(dest any, conds ...any) *gorm.DB {
	return t.Scoped(dest).Find(dest, conds...)
}

func (t *TenantDB) First(dest any, conds ...any) *gorm.DB {
	return t.Scoped(dest).First(dest, conds...)
}

func (t *TenantDB) Take(dest any, conds ...any) *gorm.DB {
	return t.Scoped(dest).Take(dest, conds...)
}

func (t *TenantDB) Count(count *int64) *gorm.DB {
	return t.Scoped().Count(count)
}

func (t *TenantDB) Pluck(column string, dest any) *gorm.DB {
	return t.Scoped().Pluck(column, dest)
}

func (t *TenantDB) Create(value any) *gorm.DB {
	if err := t.enforceTenantScope(value); err != nil {
		tx := t.db.Session(&gorm.Session{})
		tx.AddError(err)
		return tx
	}
	return t.db.Create(value)
}

func (t *TenantDB) Save(value any) *gorm.DB {
	if err := t.enforceTenantScope(value); err != nil {
		tx := t.db.Session(&gorm.Session{})
		tx.AddError(err)
		return tx
	}
	col := t.ResolveColumn(value)
	return t.db.Where(fmt.Sprintf("%s = ?", col), t.orgID).Save(value)
}

func (t *TenantDB) Delete(value any, conds ...any) *gorm.DB {
	return t.Scoped(value).Delete(value, conds...)
}

func (t *TenantDB) Update(column string, value any) *gorm.DB {
	return t.Scoped().Update(column, value)
}

func (t *TenantDB) Updates(values any) *gorm.DB {
	if err := t.enforceTenantScope(values); err != nil {
		tx := t.db.Session(&gorm.Session{})
		tx.AddError(err)
		return tx
	}
	return t.Scoped(values).Updates(values)
}

func (t *TenantDB) Transaction(fc func(tx *TenantDB) error) error {
	return t.db.Transaction(func(gormTx *gorm.DB) error {
		txTDB := &TenantDB{
			db:            gormTx,
			orgID:         t.orgID,
			defaultColumn: t.defaultColumn,
			registry:      t.registry,
			ctx:           t.ctx,
		}
		return fc(txTDB)
	})
}

func (t *TenantDB) ExecuteSQL(querySQL string, opts ...ExecOption) ([]map[string]any, ExecMeta, error) {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = WithOrgID(ctx, t.orgID)
	return Execute(ctx, t.db, t.registry, querySQL, opts...)
}
