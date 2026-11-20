package tenantsql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

const DefaultMaxBytes = 1024 * 1024

const DefaultMaxRows = 200

type ExecMeta struct {
	RowCount  int           `json:"rowCount"`
	Truncated bool          `json:"truncated"`
	Duration  time.Duration `json:"duration"`
	Columns   []string      `json:"columns"`
}

type ExecOption func(*execOptions)

type execOptions struct {
	maxRows  int
	maxBytes int
}

func WithMaxRows(maxRows int) ExecOption {
	return func(opts *execOptions) {
		if maxRows > 0 {
			opts.maxRows = maxRows
		}
	}
}

func WithMaxBytes(maxBytes int) ExecOption {
	return func(opts *execOptions) {
		if maxBytes > 0 {
			opts.maxBytes = maxBytes
		}
	}
}

func RedactRow(columns []string, row map[string]any) {
	for _, c := range columns {
		if SensitiveColumns[strings.ToLower(c)] {
			if _, ok := row[c]; ok {
				row[c] = RedactedValue
			}
		}
	}
}

func Execute(ctx context.Context, db *gorm.DB, reg *Registry, querySQL string, opts ...ExecOption) ([]map[string]any, ExecMeta, error) {
	var meta ExecMeta

	orgID := OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, meta, errors.New("unauthorized: missing workspace context")
	}

	if db == nil {
		return nil, meta, errors.New("database handle is nil")
	}

	dial := db.Name()
	if dial != "sqlite" && dial != "postgres" {
		return nil, meta, fmt.Errorf("dialect not supported: %s", dial)
	}

	if reg == nil {
		reg = DefaultRegistry()
	}

	eOpts := &execOptions{
		maxRows:  DefaultMaxRows,
		maxBytes: DefaultMaxBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(eOpts)
		}
	}

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

	referencedViews := make([]TenantView, 0, len(referencedViewNames))
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

	start := time.Now()

	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, meta, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	allowPersonal := AllowsPersonalScope(ctx)
	for _, view := range referencedViews {
		quotedViewName, err := quoteIdent(view.Name)
		if err != nil {
			return nil, meta, fmt.Errorf("invalid view name %q: %w", view.Name, err)
		}
		viewDef := view.Definition(orgID)
		wrappedSQL, err := wrapScopedRedactedViewSQL(view, viewDef, allowPersonal)
		if err != nil {
			return nil, meta, fmt.Errorf("failed to wrap view %q: %w", view.Name, err)
		}
		createSQL := fmt.Sprintf("CREATE TEMP VIEW %s AS %s", quotedViewName, wrappedSQL)
		if err := tx.Exec(createSQL).Error; err != nil {
			return nil, meta, fmt.Errorf("failed to materialize view %q: %w", view.Name, err)
		}
	}

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

		RedactRow(cols, row)
		for col := range row {
			if sensitiveSet[strings.ToLower(col)] {
				row[col] = RedactedValue
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

	actorID := UserIDFromContext(ctx)
	metaMap := map[string]any{
		"sql":         querySQL,
		"rows":        meta.RowCount,
		"duration_ms": meta.Duration.Milliseconds(),
	}
	metaJSON, err := json.Marshal(metaMap)
	if err != nil {
		log.Printf("[tenantsql] failed to marshal audit log metadata: %v", err)
	}

	if err := db.Create(&models.AuditLog{
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     "tenant_sql.query",
		TargetType: "tenant_sql",
		Meta:       string(metaJSON),
		CreatedAt:  time.Now(),
	}).Error; err != nil {
		log.Printf("[tenantsql] warning: failed to write query audit log: %v", err)
	}

	return result, meta, nil
}
