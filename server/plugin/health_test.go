package plugin

import (
	"context"
	"testing"
)

type mockHealthProvider struct {
	name   string
	status HealthStatus
	msg    string
	cat    string
	unit   string
}

func (m *mockHealthProvider) Name() string {
	return m.name
}

func (m *mockHealthProvider) Check(ctx context.Context) HealthResult {
	return HealthResult{
		Status:  m.status,
		Message: m.msg,
		Metrics: map[string]interface{}{"val": 42},
	}
}

func (m *mockHealthProvider) Metadata() HealthMeta {
	return HealthMeta{
		Category: m.cat,
		Unit:     m.unit,
	}
}

func TestHealthStatusAndMetaConstants(t *testing.T) {
	if HealthOK != "ok" || HealthWarn != "warn" || HealthError != "error" {
		t.Errorf("unexpected health status values: %v, %v, %v", HealthOK, HealthWarn, HealthError)
	}

	if CategoryRuntime != "runtime" || CategoryDatabase != "database" ||
		CategoryStorage != "storage" || CategoryCustom != "custom" {
		t.Errorf("unexpected category constants")
	}

	if UnitBytes != "bytes" || UnitCount != "count" || UnitMs != "ms" || UnitPercent != "percent" {
		t.Errorf("unexpected unit constants")
	}
}

func TestHealthProviderRegistrationAndLookup(t *testing.T) {
	reg := NewRegistry()
	p1 := &mockHealthProvider{name: "db", status: HealthOK, cat: CategoryDatabase, unit: UnitCount}
	p2 := &mockHealthProvider{name: "disk", status: HealthWarn, cat: CategoryStorage, unit: UnitBytes}

	reg.Provide(ServiceHealthProvider, HealthProvider(p1))
	reg.Provide(ServiceHealthProvider, HealthProvider(p2))

	if err := reg.Err(); err != nil {
		t.Fatalf("unexpected registry error: %v", err)
	}

	hps := reg.HealthProviders()
	if len(hps) != 2 {
		t.Fatalf("expected 2 health providers, got %d", len(hps))
	}
	if hps[0].Name() != "db" || hps[1].Name() != "disk" {
		t.Errorf("unexpected provider names: %s, %s", hps[0].Name(), hps[1].Name())
	}

	// Test lookup
	if hp, ok := LookupServiceAs[HealthProvider](reg.Lookup, HealthProviderServiceName("db")); !ok || hp.Name() != "db" {
		t.Errorf("failed to lookup db provider by name")
	}

	all, ok := LookupServiceAs[[]HealthProvider](reg.Lookup, ServiceHealthProvidersAll)
	if !ok || len(all) != 2 {
		t.Errorf("failed to lookup all health providers: %v, len=%d", ok, len(all))
	}

	// Test GetHealthProviders with Context
	ctx := &Context{Lookup: reg.Lookup}
	ctxProviders := GetHealthProviders(ctx)
	if len(ctxProviders) != 2 {
		t.Errorf("GetHealthProviders returned %d, want 2", len(ctxProviders))
	}

	// Nil context
	if nilRes := GetHealthProviders(nil); nilRes != nil {
		t.Errorf("GetHealthProviders(nil) = %v, want nil", nilRes)
	}
}

func TestHealthProviderDuplicates(t *testing.T) {
	reg := NewRegistry()
	p1 := &mockHealthProvider{name: "dup", status: HealthOK}
	p2 := &mockHealthProvider{name: "dup", status: HealthError}

	reg.Provide(ServiceHealthProvider, HealthProvider(p1))
	reg.Provide(ServiceHealthProvider, HealthProvider(p2))

	if err := reg.Err(); err == nil {
		t.Errorf("expected error on duplicate health provider name, got nil")
	}

	reg2 := NewRegistry()
	reg2.Provide(HealthProviderServiceName("p"), HealthProvider(&mockHealthProvider{name: "p"}))
	reg2.Provide(HealthProviderServiceName("p"), HealthProvider(&mockHealthProvider{name: "p"}))
	if err := reg2.Err(); err == nil {
		t.Errorf("expected error on duplicate health provider service name, got nil")
	}
}

func TestHealthProviderSliceRegistration(t *testing.T) {
	reg := NewRegistry()
	slice := []HealthProvider{
		&mockHealthProvider{name: "a", status: HealthOK},
		&mockHealthProvider{name: "b", status: HealthWarn},
	}
	reg.Provide(ServiceHealthProvider, slice)
	if err := reg.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.HealthProviders()) != 2 {
		t.Errorf("expected 2 health providers, got %d", len(reg.HealthProviders()))
	}

	// Duplicate in slice
	regDup := NewRegistry()
	dupSlice := []HealthProvider{
		&mockHealthProvider{name: "dup"},
		&mockHealthProvider{name: "dup"},
	}
	regDup.Provide(ServiceHealthProvider, dupSlice)
	if err := regDup.Err(); err == nil {
		t.Errorf("expected error on duplicate in slice")
	}
}

func TestGetHealthProvidersSingleFallback(t *testing.T) {
	reg := NewRegistry()
	p := &mockHealthProvider{name: "single", status: HealthOK}
	reg.svcs[ServiceHealthProvider] = HealthProvider(p)
	ctx := &Context{Lookup: reg.Lookup}
	res := GetHealthProviders(ctx)
	if len(res) != 1 || res[0].Name() != "single" {
		t.Errorf("expected 1 provider from fallback, got %v", res)
	}

	emptyCtx := &Context{Lookup: func(name string) (any, bool) { return nil, false }}
	if resEmpty := GetHealthProviders(emptyCtx); resEmpty != nil {
		t.Errorf("expected nil for empty lookup, got %v", resEmpty)
	}
}
