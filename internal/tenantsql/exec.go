package tenantsql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// DefaultMaxBytes is the default maximum byte size of returned query results (1MB).
const DefaultMaxBytes = 1024 * 1024

// ExecMeta carries execution metadata returned by Execute.
type ExecMeta struct {
	RowCount  int           `json:"rowCount"`
	Truncated bool          `json:"truncated"`
	Duration  time.Duration `json:"duration"`
	Columns   []string      `json:"columns"`
}

// ExecOption configures execution parameters for Execute.
type ExecOption func(*execOptions)

type execOptions struct {
	maxRows  int
	maxBytes int
}

// WithMaxRows overrides the default maximum row limit (plugin.MaxRows = 200).
func WithMaxRows(maxRows int) ExecOption {
	return func(opts *execOptions) {
		if maxRows > 0 {
			opts.maxRows = maxRows
		}
	}
}

// WithMaxBytes overrides the default maximum result size in bytes (1MB).
func WithMaxBytes(maxBytes int) ExecOption {
	return func(opts *execOptions) {
		if maxBytes > 0 {
			opts.maxBytes = maxBytes
		}
	}
}

// Execute validates and executes a tenant SQL query in a fail-closed pipeline:
//  1. Validate caller orgID from context (reject if 0)
//  2. Check database dialect (reject if not sqlite or postgres)
//  3. Validate SQL against Parser and Registry whitelist
//  4. Execute within a single transaction: create TEMP VIEWs, query, rollback
//  5. Enforce MaxRows and MaxBytes limits
//  6. Redact sensitive columns
//  7. Record synchronous AuditLog entry
func Execute(ctx context.Context, db *gorm.DB, reg *Registry, querySQL string, opts ...ExecOption) ([]map[string]any, ExecMeta, error) {
	var meta ExecMeta

	// 1. OrgID from context
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, meta, errors.New("unauthorized: missing workspace context")
	}

	if db == nil {
		return nil, meta, errors.New("database handle is nil")
	}

	// 2. Database Dialect check
	dial := db.Name()
	if dial != "sqlite" && dial != "postgres" {
		return nil, meta, fmt.Errorf("dialect not supported: %s", dial)
	}

	// Resolve registry
	if reg == nil {
		reg = DefaultRegistry()
	}

	// Configure execution options
	eOpts := &execOptions{
		maxRows:  plugin.MaxRows,
		maxBytes: DefaultMaxBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(eOpts)
		}
	}

	// 3. Parser validation
	registeredViews := reg.List()
	allowedViewNames := make([]string, len(registeredViews))
	for i, v := range registeredViews {
		allowedViewNames[i] = v.Name
	}

	parser := NewParser(WithAllowedViews(allowedViewNames))
	if err := parser.Validate(querySQL); err != nil {
		return nil, meta, err
	}

	referencedViewNames, err := ExtractReferencedViews(querySQL)
	if err != nil {
		return nil, meta, err
	}

	referencedViews := make([]plugin.TenantView, 0, len(referencedViewNames))
	sensitiveSet := make(map[string]bool)

	for _, name := range referencedViewNames {
		view, ok := reg.Lookup(name)
		if !ok {
			return nil, meta, fmt.Errorf("unknown view %q", name)
		}
		referencedViews = append(referencedViews, view)
		for _, sens := range view.Sensitive {
			sensitiveSet[strings.ToLower(strings.TrimSpace(sens))] = true
		}
	}

	// 4. Materialization + Query + Rollback in single transaction
	start := time.Now()

	// In PostgreSQL, CREATE TEMP VIEW requires catalog writes in pg_temp and fails in ReadOnly transactions.
	// Safety is guaranteed by AST validation (only SELECT allowed) and mandatory defer tx.Rollback().
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, meta, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	// Materialize referenced views
	for _, view := range referencedViews {
		viewDef := view.Definition(orgID)
		createSQL := fmt.Sprintf("CREATE TEMP VIEW %s AS %s", view.Name, viewDef)
		if err := tx.Exec(createSQL).Error; err != nil {
			return nil, meta, fmt.Errorf("failed to materialize view %q: %w", view.Name, err)
		}
	}

	// Execute SELECT query
	rows, err := tx.Raw(querySQL).Rows()
	if err != nil {
		return nil, meta, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, meta, fmt.Errorf("failed to read columns: %w", err)
	}
	meta.Columns = cols

	// 5. Scan rows with limits
	var result []map[string]any
	totalBytes := 0
	rowCount := 0

	values := make([]any, len(cols))
	valuePtrs := make([]any, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, meta, err
		}

		if rowCount >= eOpts.maxRows {
			meta.Truncated = true
			break
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, meta, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}

		// 6. Sensitive column redaction
		plugin.RedactRow(cols, row)
		for col := range row {
			if sensitiveSet[strings.ToLower(col)] {
				row[col] = plugin.RedactedValue
			}
		}

		rowBytes, err := json.Marshal(row)
		if err == nil {
			totalBytes += len(rowBytes)
			if totalBytes > eOpts.maxBytes {
				return nil, meta, fmt.Errorf("query result exceeded maximum allowed size of %d bytes", eOpts.maxBytes)
			}
		}

		result = append(result, row)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, meta, fmt.Errorf("row iteration error: %w", err)
	}

	meta.RowCount = len(result)
	meta.Duration = time.Since(start)

	// 7. Audit log (synchronous write on root DB)
	actorID := auth.UserIDFromContext(ctx)
	metaMap := map[string]any{
		"sql":         querySQL,
		"rows":        meta.RowCount,
		"duration_ms": meta.Duration.Milliseconds(),
	}
	metaJSON, _ := json.Marshal(metaMap)

	_ = db.Create(&models.AuditLog{
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     "tenant_sql.query",
		TargetType: "tenant_sql",
		Meta:       string(metaJSON),
		CreatedAt:  time.Now(),
	}).Error

	return result, meta, nil
}
