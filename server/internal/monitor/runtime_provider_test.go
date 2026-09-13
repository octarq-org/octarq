package monitor

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestRuntimeProvider(t *testing.T) {
	p := NewRuntimeProvider()

	if p.Name() != "runtime" {
		t.Errorf("Name() = %q, want 'runtime'", p.Name())
	}

	meta := p.Metadata()
	if meta.Category != plugin.CategoryRuntime {
		t.Errorf("Metadata().Category = %q, want %q", meta.Category, plugin.CategoryRuntime)
	}
	if meta.Unit != plugin.UnitBytes {
		t.Errorf("Metadata().Unit = %q, want %q", meta.Unit, plugin.UnitBytes)
	}

	res := p.Check(context.Background())
	if res.Status != plugin.HealthOK {
		t.Errorf("Check().Status = %q, want %q", res.Status, plugin.HealthOK)
	}
	if res.Message != "ok" {
		t.Errorf("Check().Message = %q, want 'ok'", res.Message)
	}

	// Verify required metrics
	expectedMetrics := []string{
		"goroutines",
		"heap_alloc_bytes",
		"heap_sys_bytes",
		"heap_idle_bytes",
		"heap_inuse_bytes",
		"alloc_bytes",
		"total_alloc_bytes",
		"sys_bytes",
		"num_gc",
		"gc_pause_total_ns",
		"gc_pause_total_ms",
		"go_version",
		"num_cpu",
	}

	for _, key := range expectedMetrics {
		if _, ok := res.Metrics[key]; !ok {
			t.Errorf("missing metric %q in runtime check result", key)
		}
	}

	if goroutines, ok := res.Metrics["goroutines"].(int); !ok || goroutines <= 0 {
		t.Errorf("invalid goroutines metric: %v", res.Metrics["goroutines"])
	}
	if cpus, ok := res.Metrics["num_cpu"].(int); !ok || cpus <= 0 {
		t.Errorf("invalid num_cpu metric: %v", res.Metrics["num_cpu"])
	}
	if ver, ok := res.Metrics["go_version"].(string); !ok || ver == "" {
		t.Errorf("invalid go_version metric: %v", res.Metrics["go_version"])
	}
}
