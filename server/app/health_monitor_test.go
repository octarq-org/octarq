package app

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/monitor"
	"github.com/octarq-org/octarq/server/plugin"
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
	for i := 0; i < 250; i++ {
		if c := a.HealthCollector(); c != nil {
			collector = c
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if collector == nil {
		t.Fatal("HealthCollector() returned nil after app Run")
	}

	// Verify that runtime, database, disk, and plugin-check are all present.
	//
	// The plugin registers its health provider while the app boots, so the
	// collector can exist a moment before the provider shows up in the report
	// it builds. Poll until every expected provider is there instead of
	// racing the boot: on a loaded machine the collector was ready before
	// the plugin had mounted, and the assertion failed on a report that was
	// merely early.
	expectedProviders := []string{"runtime", "database", "disk", "plugin-check"}

	var report monitor.HealthReport
	deadline := time.Now().Add(10 * time.Second)
	for {
		report = collector.Collect(context.Background())
		if hasAllProviders(report, expectedProviders) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if report.Overall != plugin.HealthOK && report.Overall != plugin.HealthWarn && report.Overall != plugin.HealthError {
		t.Errorf("Overall = %q, want ok, warn or error", report.Overall)
	}

	for _, name := range expectedProviders {
		if !hasProvider(report, name) {
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

// hasProvider reports whether the report carries a provider with the name.
func hasProvider(report monitor.HealthReport, name string) bool {
	for _, p := range report.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

// hasAllProviders reports whether every expected provider is present.
func hasAllProviders(report monitor.HealthReport, names []string) bool {
	for _, name := range names {
		if !hasProvider(report, name) {
			return false
		}
	}
	return true
}
