package monitor

import (
	"context"
	"runtime"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// RuntimeProvider monitors process runtime metrics including goroutines, memory, GC, and Go version.
type RuntimeProvider struct {
	startTime time.Time
}

// NewRuntimeProvider creates a new RuntimeProvider.
func NewRuntimeProvider() *RuntimeProvider {
	return &RuntimeProvider{
		startTime: time.Now(),
	}
}

// Name returns the identifier of the runtime provider.
func (p *RuntimeProvider) Name() string {
	return "runtime"
}

// Metadata returns the classification and unit metadata for the runtime provider.
func (p *RuntimeProvider) Metadata() plugin.HealthMeta {
	return plugin.HealthMeta{
		Category: plugin.CategoryRuntime,
		Unit:     plugin.UnitBytes,
	}
}

// Check evaluates the runtime health and returns current runtime metrics.
func (p *RuntimeProvider) Check(ctx context.Context) plugin.HealthResult {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := int64(time.Since(p.startTime).Seconds())
	if p.startTime.IsZero() {
		uptime = 0
	}

	metrics := map[string]interface{}{
		"goroutines":        runtime.NumGoroutine(),
		"heap_alloc_bytes":  m.HeapAlloc,
		"heap_sys_bytes":    m.HeapSys,
		"heap_idle_bytes":   m.HeapIdle,
		"heap_inuse_bytes":  m.HeapInuse,
		"alloc_bytes":       m.Alloc,
		"total_alloc_bytes": m.TotalAlloc,
		"sys_bytes":         m.Sys,
		"num_gc":            m.NumGC,
		"gc_pause_total_ns": m.PauseTotalNs,
		"gc_pause_total_ms": float64(m.PauseTotalNs) / 1e6,
		"go_version":        runtime.Version(),
		"num_cpu":           runtime.NumCPU(),
		"uptime_seconds":    uptime,
	}

	return plugin.HealthResult{
		Status:  plugin.HealthOK,
		Message: "ok",
		Metrics: metrics,
	}
}
