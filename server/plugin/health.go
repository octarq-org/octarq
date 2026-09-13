package plugin

import (
	"context"
)

// HealthStatus represents the health evaluation state of a provider or overall system.
type HealthStatus string

const (
	HealthOK    HealthStatus = "ok"
	HealthWarn  HealthStatus = "warn"
	HealthError HealthStatus = "error"
)

// HealthProvider is the SPI contract for plugins and core modules to contribute
// health check evaluation and diagnostic metrics.
type HealthProvider interface {
	Name() string
	Check(ctx context.Context) HealthResult
	Metadata() HealthMeta
}

// HealthResult carries the outcome of an individual health check.
type HealthResult struct {
	Status  HealthStatus           `json:"status"`
	Message string                 `json:"message,omitempty"`
	Metrics map[string]interface{} `json:"metrics,omitempty"` // Custom metric key-value pairs
}

// HealthMeta describes categorization and display metadata for a HealthProvider.
type HealthMeta struct {
	Category string `json:"category,omitempty"` // "runtime" | "database" | "storage" | "custom"
	Unit     string `json:"unit,omitempty"`     // "bytes" | "count" | "ms" | "percent"
}

// Predefined HealthMeta categories.
const (
	CategoryRuntime  = "runtime"
	CategoryDatabase = "database"
	CategoryStorage  = "storage"
	CategoryCustom   = "custom"
)

// Predefined HealthMeta units.
const (
	UnitBytes   = "bytes"
	UnitCount   = "count"
	UnitMs      = "ms"
	UnitPercent = "percent"
)

// GetHealthProviders resolves all registered HealthProvider instances from the context.
func GetHealthProviders(ctx *Context) []HealthProvider {
	if ctx == nil {
		return nil
	}
	if all, ok := LookupAs[[]HealthProvider](ctx, ServiceHealthProvidersAll); ok && len(all) > 0 {
		return all
	}
	if p, ok := LookupAs[HealthProvider](ctx, ServiceHealthProvider); ok {
		return []HealthProvider{p}
	}
	return nil
}
