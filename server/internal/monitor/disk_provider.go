package monitor

import (
	"context"
	"fmt"
	"os"

	"github.com/octarq-org/octarq/server/plugin"
)

type DiskProvider struct {
	dataDir           string
	warnThresholdPct  float64
	errorThresholdPct float64
	statFunc          func(path string) (total, free, avail uint64, err error)
}

type DiskProviderOption func(*DiskProvider)

func WithDiskThresholds(warnPct, errorPct float64) DiskProviderOption {
	return func(p *DiskProvider) {
		if warnPct > 0 {
			p.warnThresholdPct = warnPct
		}
		if errorPct > 0 {
			p.errorThresholdPct = errorPct
		}
	}
}

func WithDiskStatFunc(fn func(path string) (total, free, avail uint64, err error)) DiskProviderOption {
	return func(p *DiskProvider) {
		p.statFunc = fn
	}
}

func NewDiskProvider(dataDir string, opts ...DiskProviderOption) *DiskProvider {
	if dataDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dataDir = cwd
		} else {
			dataDir = "."
		}
	}

	p := &DiskProvider{
		dataDir:           dataDir,
		warnThresholdPct:  85.0,
		errorThresholdPct: 95.0,
		statFunc:          getDiskUsage,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

func (p *DiskProvider) Name() string {
	return "disk"
}

func (p *DiskProvider) Metadata() plugin.HealthMeta {
	return plugin.HealthMeta{
		Category: plugin.CategoryStorage,
		Unit:     plugin.UnitBytes,
	}
}

func (p *DiskProvider) Check(ctx context.Context) plugin.HealthResult {
	statFn := p.statFunc
	if statFn == nil {
		statFn = getDiskUsage
	}

	total, free, avail, err := statFn(p.dataDir)
	if err != nil {
		return plugin.HealthResult{
			Status:  plugin.HealthError,
			Message: fmt.Sprintf("disk check failed: %v", err),
			Metrics: map[string]interface{}{
				"path": p.dataDir,
			},
		}
	}

	var used uint64
	var usagePercent float64
	if total > 0 {
		if total >= free {
			used = total - free
		}
		usagePercent = float64(used) / float64(total) * 100.0
	}

	metrics := map[string]interface{}{
		"total_bytes":     total,
		"free_bytes":      free,
		"available_bytes": avail,
		"used_bytes":      used,
		"usage_percent":   usagePercent,
		"path":            p.dataDir,
	}

	if usagePercent >= p.errorThresholdPct {
		return plugin.HealthResult{
			Status:  plugin.HealthError,
			Message: fmt.Sprintf("disk space critically low: %.1f%% used", usagePercent),
			Metrics: metrics,
		}
	}

	if usagePercent >= p.warnThresholdPct {
		return plugin.HealthResult{
			Status:  plugin.HealthWarn,
			Message: fmt.Sprintf("disk space warning: %.1f%% used", usagePercent),
			Metrics: metrics,
		}
	}

	return plugin.HealthResult{
		Status:  plugin.HealthOK,
		Message: "ok",
		Metrics: metrics,
	}
}
