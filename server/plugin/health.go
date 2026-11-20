package plugin

import (
	"context"
)

type HealthStatus string

const (
	HealthOK    HealthStatus = "ok"
	HealthWarn  HealthStatus = "warn"
	HealthError HealthStatus = "error"
)

type HealthProvider interface {
	Name() string
	Check(ctx context.Context) HealthResult
	Metadata() HealthMeta
}

type HealthResult struct {
	Status  HealthStatus           `json:"status"`
	Message string                 `json:"message,omitempty"`
	Metrics map[string]interface{} `json:"metrics,omitempty"`
}

type HealthMeta struct {
	Category string `json:"category,omitempty"`
	Unit     string `json:"unit,omitempty"`
}

const (
	CategoryRuntime  = "runtime"
	CategoryDatabase = "database"
	CategoryStorage  = "storage"
	CategoryCustom   = "custom"
)

const (
	UnitBytes   = "bytes"
	UnitCount   = "count"
	UnitMs      = "ms"
	UnitPercent = "percent"
)

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
