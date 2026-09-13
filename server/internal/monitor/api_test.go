package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

func TestAPI_Handler(t *testing.T) {
	// 1. Overall OK -> 200
	pOK := &mockProvider{
		name:     "runtime",
		category: "runtime",
		unit:     "bytes",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			return plugin.HealthResult{
				Status:  plugin.HealthOK,
				Message: "ok",
				Metrics: map[string]interface{}{"goroutines": 10},
			}
		},
	}
	cOK := NewCollector(DefaultInterval, pOK)
	handlerOK := Handler(cOK)

	req := httptest.NewRequest(http.MethodGet, "/api/monitor/health", nil)
	rec := httptest.NewRecorder()
	handlerOK.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	var repOK HealthReport
	if err := json.Unmarshal(rec.Body.Bytes(), &repOK); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if repOK.Overall != plugin.HealthOK {
		t.Errorf("Overall = %q, want %q", repOK.Overall, plugin.HealthOK)
	}
	if len(repOK.Providers) != 1 || repOK.Providers[0].Name != "runtime" {
		t.Errorf("unexpected providers in report: %v", repOK.Providers)
	}

	// 2. Overall Warn -> 200
	pWarn := &mockProvider{
		name:     "database",
		category: "database",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			return plugin.HealthResult{
				Status:  plugin.HealthWarn,
				Message: "2 slow queries",
				Metrics: map[string]interface{}{"slow_queries": 2},
			}
		},
	}
	cWarn := NewCollector(DefaultInterval, pOK, pWarn)
	handlerWarn := Handler(cWarn)

	recWarn := httptest.NewRecorder()
	handlerWarn.ServeHTTP(recWarn, req)

	if recWarn.Code != http.StatusOK {
		t.Errorf("got status %d for warn, want %d", recWarn.Code, http.StatusOK)
	}
	var repWarn HealthReport
	_ = json.Unmarshal(recWarn.Body.Bytes(), &repWarn)
	if repWarn.Overall != plugin.HealthWarn {
		t.Errorf("Overall = %q, want %q", repWarn.Overall, plugin.HealthWarn)
	}

	// 3. Overall Error -> 503
	pErr := &mockProvider{
		name:     "storage",
		category: "storage",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			return plugin.HealthResult{
				Status:  plugin.HealthError,
				Message: "disk full",
			}
		},
	}
	cErr := NewCollector(DefaultInterval, pOK, pErr)
	handlerErr := Handler(cErr)

	recErr := httptest.NewRecorder()
	handlerErr.ServeHTTP(recErr, req)

	if recErr.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d for error, want %d", recErr.Code, http.StatusServiceUnavailable)
	}
	var repErr HealthReport
	_ = json.Unmarshal(recErr.Body.Bytes(), &repErr)
	if repErr.Overall != plugin.HealthError {
		t.Errorf("Overall = %q, want %q", repErr.Overall, plugin.HealthError)
	}
}

func TestAPI_RegisterHTTP(t *testing.T) {
	mux := http.NewServeMux()
	p := &mockProvider{name: "p", category: "custom"}
	c := NewCollector(DefaultInterval, p)

	RegisterHTTP(mux, c)

	req := httptest.NewRequest(http.MethodGet, "/api/monitor/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("RegisterHTTP route returned %d, want 200", rec.Code)
	}
}

func TestAPI_RegisterRoutesHuma(t *testing.T) {
	mux := http.NewServeMux()
	humaConfig := huma.DefaultConfig("Test API", "1.0.0")
	api := humago.New(mux, humaConfig)

	pOK := &mockProvider{name: "p_ok", category: "runtime"}
	c := NewCollector(DefaultInterval, pOK)

	RegisterRoutes(api, c)

	// Test 200 OK path
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Huma route returned %d, want 200", rec.Code)
	}

	var rep HealthReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if rep.Overall != plugin.HealthOK {
		t.Errorf("Overall = %q, want %q", rep.Overall, plugin.HealthOK)
	}

	// Test 503 Error path
	pErr := &mockProvider{
		name: "p_err",
		checkFn: func(ctx context.Context) plugin.HealthResult {
			return plugin.HealthResult{Status: plugin.HealthError, Message: "fatal"}
		},
	}
	c.RegisterProvider(pErr)
	c.Collect(context.Background())

	rec503 := httptest.NewRecorder()
	mux.ServeHTTP(rec503, req)

	if rec503.Code != http.StatusServiceUnavailable {
		t.Errorf("Huma route with error returned %d, want 503", rec503.Code)
	}

	// HealthInput.Resolve
	input := &HealthInput{}
	if errs := input.Resolve(nil); errs != nil {
		t.Errorf("HealthInput.Resolve returned errs: %v", errs)
	}
}
