package app

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/monitor"
	"github.com/octarq-org/octarq/plugin"
)

type healthTestPlugin struct {
	name string
}

func (p *healthTestPlugin) Name() string {
	return p.name
}

func (p *healthTestPlugin) Models() []any {
	return nil
}

func (p *healthTestPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	customProvider := &testHealthCustomProvider{name: "plugin-check"}
	ctx.Provide(plugin.ServiceHealthProvider, plugin.HealthProvider(customProvider))
}

type testHealthCustomProvider struct {
	name string
}

func (p *testHealthCustomProvider) Name() string {
	return p.name
}

func (p *testHealthCustomProvider) Metadata() plugin.HealthMeta {
	return plugin.HealthMeta{Category: plugin.CategoryCustom, Unit: plugin.UnitCount}
}

func (p *testHealthCustomProvider) Check(ctx context.Context) plugin.HealthResult {
	return plugin.HealthResult{
		Status:  plugin.HealthOK,
		Message: "plugin custom ok",
		Metrics: map[string]interface{}{"items": 99},
	}
}

func TestApp_HealthMonitorIntegration(t *testing.T) {
	a := bootApp(t)
	a.Use(&healthTestPlugin{name: "myhealth"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run app in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(ctx)
	}()

	// Wait briefly for app to initialize
	var collector *monitor.Collector
	for i := 0; i < 50; i++ {
		if c := a.HealthCollector(); c != nil {
			collector = c
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if collector == nil {
		t.Fatal("HealthCollector() returned nil after app Run")
	}

	report := collector.Collect(context.Background())
	if report.Overall != plugin.HealthOK && report.Overall != plugin.HealthWarn {
		t.Errorf("Overall = %q, want ok or warn", report.Overall)
	}

	// Verify that runtime, database, disk, and plugin-check are all present
	expectedProviders := map[string]bool{
		"runtime":      false,
		"database":     false,
		"disk":         false,
		"plugin-check": false,
	}

	for _, p := range report.Providers {
		if _, ok := expectedProviders[p.Name]; ok {
			expectedProviders[p.Name] = true
		}
	}

	for name, found := range expectedProviders {
		if !found {
			t.Errorf("expected provider %q not found in health report", name)
		}
	}

	// Cancel context to shut down cleanly
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("app Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("app did not shut down within timeout")
	}
}
