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
	// ErrNilDB is returned when a nil *gorm.DB is passed to TenantDB constructors.
	ErrNilDB = errors.New("tenantsql: database handle cannot be nil")

	// ErrZeroOrgID is returned when an invalid (zero) tenant org ID is provided.
	ErrZeroOrgID = errors.New("tenantsql: tenant org ID must be non-zero")

	// ErrMissingTenantContext is returned when context does not contain an authenticated tenant OrgID.
	ErrMissingTenantContext = errors.New("tenantsql: missing tenant context in request")

	// ErrTenantMismatch is returned when an entity's tenant ID contradicts the bound TenantDB scope.
	ErrTenantMismatch = errors.New("tenantsql: entity tenant ID does not match bound tenant scope")
)

// DefaultTenantColumn is the standard column used for tenant isolation in Octarq (legacy owner_id).
const DefaultTenantColumn = "owner_id"

// TenantScoped is an interface that models can optionally implement to explicitly specify
// their tenant isolation column (e.g. "org_id" vs "owner_id").
type TenantScoped interface {
	TenantColumn() string
}

// TenantDBOption configures a TenantDB instance.
type TenantDBOption func(*tenantDBOptions)

type tenantDBOptions struct {
	defaultColumn string
	registry      *Registry
}

// WithTenantColumn overrides the default tenant column (default: "owner_id").
func WithTenantColumn(col string) TenantDBOption {
	return func(opts *tenantDBOptions) {
		if trimmed := strings.TrimSpace(col); trimmed != "" {
			opts.defaultColumn = trimmed
		}
	}
}

// WithTenantRegistry sets the view registry for TenantDB SQL execution.
func WithTenantRegistry(reg *Registry) TenantDBOption {
	return func(opts *tenantDBOptions) {
		opts.registry = reg
	}
}

// TenantDB provides a type-safe, fail-closed database handle strictly scoped to a specific tenant (OrgID).
// Every query, mutation, and transaction operation executed through TenantDB is guaranteed to include
// the tenant scoping condition, eliminating the risk of accidental cross-tenant data leaks.
type TenantDB struct {
	db            *gorm.DB
	orgID         uint
	defaultColumn string
	registry      *Registry
	ctx           context.Context
}

// NewTenantDB constructs a new TenantDB scoped to the specified orgID.
// It fails closed if db is nil or orgID is 0.
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

// TenantDBFromContext extracts the tenant OrgID from ctx and constructs a TenantDB.
// If ctx does not contain an authenticated OrgID (OrgID == 0), it returns ErrMissingTenantContext.
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

// OrgID returns the tenant org ID bound to this handle.
func (t *TenantDB) OrgID() uint {
	return t.orgID
}

// DefaultColumn returns the default column name used for tenant filtering.
func (t *TenantDB) DefaultColumn() string {
	return t.defaultColumn
}

// WithContext returns a new TenantDB copy bound to the specified context.
func (t *TenantDB) WithContext(ctx context.Context) *TenantDB {
	cp := *t
	cp.ctx = ctx
	if t.db != nil {
		cp.db = t.db.WithContext(ctx)
	}
	return &cp
}

// WithColumn returns a new TenantDB copy with a different default tenant column (e.g. "org_id").
func (t *TenantDB) WithColumn(col string) *TenantDB {
	trimmed := strings.TrimSpace(col)
	if trimmed == "" {
		trimmed = DefaultTenantColumn
	}
	cp := *t
	cp.defaultColumn = trimmed
	return &cp
}

// ResolveColumn determines the appropriate tenant column name for a given model.
// 1. If model implements TenantScoped, uses model.TenantColumn().
// 2. If model struct contains a field with gorm:"column:org_id" or field named OrgID without owner_id column, uses "org_id".
// 3. If model struct contains a field with gorm:"column:owner_id" or field named OwnerID, uses "owner_id".
// 4. Defaults to t.defaultColumn ("owner_id").
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

// Scoped returns a *gorm.DB query handle with the tenant WHERE condition applied.
// If a model is provided, it configures .Model(model[0]) and resolves the tenant column for that model.
func (t *TenantDB) Scoped(model ...any) *gorm.DB {
	tx := t.db
	if len(model) > 0 && model[0] != nil {
		m := model[0]
		col := t.ResolveColumn(m)
		return tx.Model(m).Where(fmt.Sprintf("%s = ?", col), t.orgID)
	}
	return tx.Where(fmt.Sprintf("%s = ?", t.defaultColumn), t.orgID)
}

// Model sets the model and applies the tenant scope for that model.
func (t *TenantDB) Model(value any) *gorm.DB {
	return t.Scoped(value)
}

// Table sets the table name and applies the tenant scope.
func (t *TenantDB) Table(name string, args ...any) *gorm.DB {
	tx := t.db.Table(name, args...)
	return tx.Where(fmt.Sprintf("%s = ?", t.defaultColumn), t.orgID)
}

// Where appends a query condition to the tenant-scoped query.
func (t *TenantDB) Where(query any, args ...any) *gorm.DB {
	return t.Scoped().Where(query, args...)
}

// Select specifies fields that you want when querying, creating, updating.
func (t *TenantDB) Select(query any, args ...any) *gorm.DB {
	return t.Scoped().Select(query, args...)
}

// Order specifies order when retrieving records from database.
func (t *TenantDB) Order(value any) *gorm.DB {
	return t.Scoped().Order(value)
}

// Limit specifies the number of records to be retrieved.
func (t *TenantDB) Limit(limit int) *gorm.DB {
	return t.Scoped().Limit(limit)
}

// Offset specifies the number of records to skip before starting to return the records.
func (t *TenantDB) Offset(offset int) *gorm.DB {
	return t.Scoped().Offset(offset)
}

// Joins specifies Joins conditions.
func (t *TenantDB) Joins(query string, args ...any) *gorm.DB {
	return t.Scoped().Joins(query, args...)
}

// Preload preloads associations with given conditions.
func (t *TenantDB) Preload(query string, args ...any) *gorm.DB {
	return t.Scoped().Preload(query, args...)
}

// Omit specifies fields that you want to ignore when creating, updating and querying.
func (t *TenantDB) Omit(columns ...string) *gorm.DB {
	return t.Scoped().Omit(columns...)
}

// Distinct specifies distinct query.
func (t *TenantDB) Distinct(args ...any) *gorm.DB {
	return t.Scoped().Distinct(args...)
}

// Clauses adds clauses to the query.
func (t *TenantDB) Clauses(conds ...clause.Expression) *gorm.DB {
	return t.Scoped().Clauses(conds...)
}

// Find executes a tenant-scoped Find query into dest.
func (t *TenantDB) Find(dest any, conds ...any) *gorm.DB {
	return t.Scoped(dest).Find(dest, conds...)
}

// First executes a tenant-scoped First query into dest.
func (t *TenantDB) First(dest any, conds ...any) *gorm.DB {
	return t.Scoped(dest).First(dest, conds...)
}

// Take executes a tenant-scoped Take query into dest.
func (t *TenantDB) Take(dest any, conds ...any) *gorm.DB {
	return t.Scoped(dest).Take(dest, conds...)
}

// Count returns the count of records matching the tenant scope and optional conditions.
func (t *TenantDB) Count(count *int64) *gorm.DB {
	return t.Scoped().Count(count)
}

// Pluck plucks a single column from the tenant-scoped records.
func (t *TenantDB) Pluck(column string, dest any) *gorm.DB {
	return t.Scoped().Pluck(column, dest)
}

// Create inserts one or more records, automatically enforcing that the tenant ID
// matches the bound orgID. If the entity has an OrgID or OwnerID field with a conflicting
// non-zero value, Create fails immediately with ErrTenantMismatch to prevent cross-tenant writes.
func (t *TenantDB) Create(value any) *gorm.DB {
	if err := t.enforceTenantScope(value); err != nil {
		tx := t.db.Session(&gorm.Session{})
		tx.AddError(err)
		return tx
	}
	return t.db.Create(value)
}

// Save updates or inserts a record while enforcing tenant scope.
func (t *TenantDB) Save(value any) *gorm.DB {
	if err := t.enforceTenantScope(value); err != nil {
		tx := t.db.Session(&gorm.Session{})
		tx.AddError(err)
		return tx
	}
	col := t.ResolveColumn(value)
	return t.db.Where(fmt.Sprintf("%s = ?", col), t.orgID).Save(value)
}

// Delete deletes records matching the value and conditions, strictly bounded by the tenant scope.
func (t *TenantDB) Delete(value any, conds ...any) *gorm.DB {
	return t.Scoped(value).Delete(value, conds...)
}

// Update updates a single column on records matching the tenant scope.
func (t *TenantDB) Update(column string, value any) *gorm.DB {
	return t.Scoped().Update(column, value)
}

// Updates updates multiple columns on records matching the tenant scope.
func (t *TenantDB) Updates(values any) *gorm.DB {
	if err := t.enforceTenantScope(values); err != nil {
		tx := t.db.Session(&gorm.Session{})
		tx.AddError(err)
		return tx
	}
	return t.Scoped(values).Updates(values)
}

func (t *TenantDB) enforceTenantScope(val any) error {
	if val == nil {
		return nil
	}
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			if err := t.enforceSingleEntityScope(elem); err != nil {
				return err
			}
		}
		return nil
	}

	if v.Kind() == reflect.Map {
		iter := v.MapRange()
		for iter.Next() {
			k := strings.ToLower(fmt.Sprintf("%v", iter.Key().Interface()))
			if k == "org_id" || k == "owner_id" || k == "orgid" || k == "ownerid" {
				valInt := toUint(iter.Value().Interface())
				if valInt != 0 && valInt != t.orgID {
					return fmt.Errorf("%w: map key %s is %d, want %d", ErrTenantMismatch, iter.Key().Interface(), valInt, t.orgID)
				}
			}
		}
		return nil
	}

	return t.enforceSingleEntityScope(v)
}

func toUint(val any) uint {
	switch v := val.(type) {
	case uint:
		return v
	case uint8:
		return uint(v)
	case uint16:
		return uint(v)
	case uint32:
		return uint(v)
	case uint64:
		return uint(v)
	case int:
		return uint(v)
	case int8:
		return uint(v)
	case int16:
		return uint(v)
	case int32:
		return uint(v)
	case int64:
		return uint(v)
	default:
		return 0
	}
}

func (t *TenantDB) enforceSingleEntityScope(v reflect.Value) error {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	for _, fieldName := range []string{"OrgID", "OwnerID"} {
		f := v.FieldByName(fieldName)
		if f.IsValid() && f.CanSet() {
			switch f.Kind() {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				curVal := f.Uint()
				if curVal == 0 {
					f.SetUint(uint64(t.orgID))
				} else if curVal != uint64(t.orgID) {
					return fmt.Errorf("%w: field %s is %d, want %d", ErrTenantMismatch, fieldName, curVal, t.orgID)
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				curVal := f.Int()
				if curVal == 0 {
					f.SetInt(int64(t.orgID))
				} else if curVal != int64(t.orgID) {
					return fmt.Errorf("%w: field %s is %d, want %d", ErrTenantMismatch, fieldName, curVal, t.orgID)
				}
			}
		}
	}
	return nil
}

// Transaction executes the provided callback within a database transaction.
// The callback receives a *TenantDB instance that is scoped to the transaction
// and bound to the same tenant OrgID.
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

// ExecuteSQL executes a tenant SQL query through the fail-closed tenantsql pipeline
// for this tenant, creating temporary views and returning results.
func (t *TenantDB) ExecuteSQL(querySQL string, opts ...ExecOption) ([]map[string]any, ExecMeta, error) {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = WithOrgID(ctx, t.orgID)
	return Execute(ctx, t.db, t.registry, querySQL, opts...)
}

// UnscopedDangerous returns the underlying unscoped *gorm.DB handle.
// CAUTION: This bypasses all tenant isolation guarantees. Use only for system-level
// migrations or maintenance operations where cross-tenant access is explicitly intended.
func (t *TenantDB) UnscopedDangerous() *gorm.DB {
	return t.db
}
