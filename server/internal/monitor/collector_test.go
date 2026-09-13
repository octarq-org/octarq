package monitor

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

type mockProvider struct {
	name     string
	category string
	unit     string
	checkFn  func(ctx context.Context) plugin.HealthResult
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Metadata() plugin.HealthMeta {
	return plugin.HealthMeta{
		Category: m.category,
		Unit:     m.unit,
	}
}

func (m *mockProvider) Check(ctx context.Context) plugin.HealthResult {
	if m.checkFn != nil {
		return m.checkFn(ctx)
	}
	return plugin.HealthResult{
		Status:  plugin.HealthOK,
		Message: "ok",
		Metrics: map[string]interface{}{"val": 1},
	}
}

func TestCollector_RegistrationAndOptions(t *testing.T) {
	c := NewCollector(0)
	if c.interval != DefaultInterval {
		t.Errorf("interval = %v, want %v", c.interval, DefaultInterval)
	}

	p1 := &mockProvider{name: "p1", category: "custom", unit: "count"}
	p2 := &mockProvider{name: "p2", category: "runtime", unit: "bytes"}

	c.RegisterProvider(nil) // nil safe
	c.RegisterProvider(p1)
	c.RegisterProvider(p1) // duplicate safe
	c.RegisterProviders(p2, nil)

	if len(c.providers) != 2 {
		t.Errorf("expected 2 providers registered, got %d", len(c.providers))
	}

	c.SetTimeout(2 * time.Second)
	if c.timeout != 2*time.Second {
		t.Errorf("timeout = %v, want 2s", c.timeout)
	}
	c.SetTimeout(0) // ignored if <= 0
	if c.timeout != 2*time.Second {
		t.Errorf("timeout changed on <=0 value: %v", c.timeout)
	}
}

func TestCollector_CollectAggregation(t *testing.T) {
	// Case 1: All OK
	p1 := &mockProvider{name: "p1", category: "runtime", unit: "bytes"}
	p2 := &mockProvider{name: "p2", category: "database", unit: "count"}
	c := NewCollector(10*time.Second, p1, p2)

	rep := c.Collect(context.Background())
	if rep.Overall != plugin.HealthOK {
		t.Errorf("expected Overall HealthOK, got %q", rep.Overall)
	}
	if len(rep.Providers) != 2 {
		t.Fatalf("expected 2 provider reports, got %d", len(rep.Providers))
	}
	if rep.CheckedAt == "" {
		t.Error("CheckedAt timestamp is empty")
	}

	// Case 2: One Warn -> Overall Warn
	pWarn := &mockProvider{
		name:     "warn_p",
		category: "custom",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			return plugin.HealthResult{Status: plugin.HealthWarn, Message: "degraded"}
		},
	}
	c.RegisterProvider(pWarn)
	repWarn := c.Collect(context.Background())
	if repWarn.Overall != plugin.HealthWarn {
		t.Errorf("expected Overall HealthWarn, got %q", repWarn.Overall)
	}

	// Case 3: One Error -> Overall Error
	pErr := &mockProvider{
		name:     "err_p",
		category: "storage",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			return plugin.HealthResult{Status: plugin.HealthError, Message: "fatal"}
		},
	}
	c.RegisterProvider(pErr)
	repErr := c.Collect(context.Background())
	if repErr.Overall != plugin.HealthError {
		t.Errorf("expected Overall HealthError, got %q", repErr.Overall)
	}
}

func TestCollector_DynamicProviderSource(t *testing.T) {
	p1 := &mockProvider{name: "static", category: "runtime"}
	pDynamic := &mockProvider{name: "plugin_dynamic", category: "custom"}

	c := NewCollector(10*time.Second, p1)
	c.SetProviderSource(func() []plugin.HealthProvider {
		return []plugin.HealthProvider{pDynamic, p1} // p1 is duplicate and should be deduplicated
	})

	rep := c.Collect(context.Background())
	if len(rep.Providers) != 2 {
		t.Errorf("expected 2 unique providers, got %d", len(rep.Providers))
	}
}

func TestCollector_PanicRecovery(t *testing.T) {
	pPanic := &mockProvider{
		name:     "panicky",
		category: "custom",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			panic("something went terribly wrong inside provider")
		},
	}

	c := NewCollector(10*time.Second, pPanic)
	rep := c.Collect(context.Background())

	if rep.Overall != plugin.HealthError {
		t.Errorf("expected Overall HealthError on panic, got %q", rep.Overall)
	}
	if len(rep.Providers) != 1 {
		t.Fatalf("expected 1 provider result, got %d", len(rep.Providers))
	}
	pRep := rep.Providers[0]
	if pRep.Status != plugin.HealthError {
		t.Errorf("expected provider status HealthError, got %q", pRep.Status)
	}
	if !strings.Contains(pRep.Message, "panicked") {
		t.Errorf("expected message to note panic, got %q", pRep.Message)
	}
}

func TestCollector_LatestReportAndLifecycle(t *testing.T) {
	var count atomic.Int32
	p := &mockProvider{
		name: "counter",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			count.Add(1)
			return plugin.HealthResult{Status: plugin.HealthOK}
		},
	}

	c := NewCollector(50*time.Millisecond, p)

	// Cold start LatestReport should trigger Collect
	rep1 := c.LatestReport(context.Background())
	if rep1.Overall != plugin.HealthOK || count.Load() != 1 {
		t.Errorf("cold start LatestReport failed, count = %d", count.Load())
	}

	// Subsequent LatestReport returns cached without incrementing count immediately
	rep2 := c.LatestReport(context.Background())
	if rep2.CheckedAt != rep1.CheckedAt || count.Load() != 1 {
		t.Errorf("LatestReport did not return cached report")
	}

	// Start background polling
	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	// Calling Start again should be a no-op
	c.Start(ctx)

	// Wait for background ticker to tick
	time.Sleep(120 * time.Millisecond)
	if count.Load() < 2 {
		t.Errorf("expected ticker to collect, got count = %d", count.Load())
	}

	// Stop collector
	c.Stop()
	// Stop again should be safe
	c.Stop()
	cancel()
}

func TestCollector_StartWithContextCancel(t *testing.T) {
	p := &mockProvider{name: "quick"}
	c := NewCollector(20*time.Millisecond, p)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	cancel() // cancel immediately
	time.Sleep(30 * time.Millisecond)
	c.Stop()
}
