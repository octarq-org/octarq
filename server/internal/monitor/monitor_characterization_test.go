package monitor_test

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/monitor"
	"github.com/octarq-org/octarq/server/plugin"
)

type charMockProvider struct {
	name   string
	status plugin.HealthStatus
}

func (m *charMockProvider) Name() string { return m.name }
func (m *charMockProvider) Metadata() plugin.HealthMeta {
	return plugin.HealthMeta{Category: "system", Unit: "count"}
}
func (m *charMockProvider) Check(ctx context.Context) plugin.HealthResult {
	return plugin.HealthResult{
		Status:  m.status,
		Message: "mock detail",
		Metrics: map[string]interface{}{"val": 1},
	}
}

func TestMonitorCharacterizationAggregation(t *testing.T) {
	c := monitor.NewCollector(time.Minute)
	ctx := context.Background()

	// 1. All OK -> Overall OK
	c.RegisterProvider(&charMockProvider{name: "db", status: plugin.HealthOK})
	c.RegisterProvider(&charMockProvider{name: "disk", status: plugin.HealthOK})
	c.Collect(ctx)
	rep := c.LatestReport(ctx)
	if rep.Overall != plugin.HealthOK {
		t.Fatalf("expected HealthOK, got %v", rep.Overall)
	}

	// 2. One Error -> Overall Error
	c.RegisterProvider(&charMockProvider{name: "net", status: plugin.HealthError})
	c.Collect(ctx)
	rep = c.LatestReport(ctx)
	if rep.Overall != plugin.HealthError {
		t.Fatalf("expected HealthError, got %v", rep.Overall)
	}
}
