package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestDiskProvider_DefaultsAndRealStat(t *testing.T) {
	p := NewDiskProvider("")

	if p.Name() != "disk" {
		t.Errorf("Name() = %q, want 'disk'", p.Name())
	}

	meta := p.Metadata()
	if meta.Category != plugin.CategoryStorage {
		t.Errorf("Category = %q, want %q", meta.Category, plugin.CategoryStorage)
	}
	if meta.Unit != plugin.UnitBytes {
		t.Errorf("Unit = %q, want %q", meta.Unit, plugin.UnitBytes)
	}

	res := p.Check(context.Background())
	if res.Status != plugin.HealthOK && res.Status != plugin.HealthWarn {
		t.Errorf("unexpected status on real filesystem: %q (msg: %s)", res.Status, res.Message)
	}

	metrics := res.Metrics
	for _, k := range []string{"total_bytes", "free_bytes", "available_bytes", "used_bytes", "usage_percent", "path"} {
		if _, ok := metrics[k]; !ok {
			t.Errorf("missing metric %q", k)
		}
	}
}

func TestDiskProvider_ThresholdsAndMocks(t *testing.T) {
	// 1. Normal usage: 100 GB total, 50 GB free -> 50% used
	statOK := func(path string) (total, free, avail uint64, err error) {
		return 100 * 1e9, 50 * 1e9, 50 * 1e9, nil
	}
	pOK := NewDiskProvider("/tmp", WithDiskStatFunc(statOK))
	resOK := pOK.Check(context.Background())
	if resOK.Status != plugin.HealthOK {
		t.Errorf("expected HealthOK, got %q", resOK.Status)
	}
	if resOK.Message != "ok" {
		t.Errorf("expected 'ok', got %q", resOK.Message)
	}
	pct, ok := resOK.Metrics["usage_percent"].(float64)
	if !ok || pct != 50.0 {
		t.Errorf("expected 50%% usage, got %v", pct)
	}

	// 2. Warning usage: 100 GB total, 12 GB free -> 88% used (>= 85%)
	statWarn := func(path string) (total, free, avail uint64, err error) {
		return 100 * 1e9, 12 * 1e9, 12 * 1e9, nil
	}
	pWarn := NewDiskProvider("/data", WithDiskStatFunc(statWarn))
	resWarn := pWarn.Check(context.Background())
	if resWarn.Status != plugin.HealthWarn {
		t.Errorf("expected HealthWarn, got %q", resWarn.Status)
	}
	if !strings.Contains(resWarn.Message, "disk space warning") {
		t.Errorf("expected warning message, got %q", resWarn.Message)
	}

	// 3. Error usage: 100 GB total, 3 GB free -> 97% used (>= 95%)
	statErr := func(path string) (total, free, avail uint64, err error) {
		return 100 * 1e9, 3 * 1e9, 3 * 1e9, nil
	}
	pErr := NewDiskProvider("/data", WithDiskStatFunc(statErr))
	resErr := pErr.Check(context.Background())
	if resErr.Status != plugin.HealthError {
		t.Errorf("expected HealthError, got %q", resErr.Status)
	}
	if !strings.Contains(resErr.Message, "critically low") {
		t.Errorf("expected critical message, got %q", resErr.Message)
	}

	// 4. Custom thresholds via WithDiskThresholds
	pCustom := NewDiskProvider("/data",
		WithDiskThresholds(70.0, 90.0),
		WithDiskStatFunc(func(path string) (uint64, uint64, uint64, error) {
			return 100 * 1e9, 25 * 1e9, 25 * 1e9, nil // 75% used
		}),
	)
	resCustom := pCustom.Check(context.Background())
	if resCustom.Status != plugin.HealthWarn {
		t.Errorf("expected HealthWarn with custom threshold 70%%, got %q", resCustom.Status)
	}

	// 5. Stat function error
	statFail := func(path string) (total, free, avail uint64, err error) {
		return 0, 0, 0, errors.New("io error on mount")
	}
	pFail := NewDiskProvider("/unmounted", WithDiskStatFunc(statFail))
	resFail := pFail.Check(context.Background())
	if resFail.Status != plugin.HealthError {
		t.Errorf("expected HealthError on stat failure, got %q", resFail.Status)
	}
	if !strings.Contains(resFail.Message, "disk check failed") {
		t.Errorf("expected 'disk check failed' in message, got %q", resFail.Message)
	}

	// 6. Zero total bytes edge case
	statZero := func(path string) (total, free, avail uint64, err error) {
		return 0, 0, 0, nil
	}
	pZero := NewDiskProvider("/empty", WithDiskStatFunc(statZero))
	resZero := pZero.Check(context.Background())
	if resZero.Status != plugin.HealthOK {
		t.Errorf("expected HealthOK on zero total bytes, got %q", resZero.Status)
	}
}
