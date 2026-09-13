package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

// DefaultInterval is the default polling interval for the health collector (30 seconds).
const DefaultInterval = 30 * time.Second

// ProviderHealthReport represents the health check report for an individual provider.
type ProviderHealthReport struct {
	Name     string                 `json:"name"`
	Status   plugin.HealthStatus    `json:"status"`
	Message  string                 `json:"message,omitempty"`
	Metrics  map[string]interface{} `json:"metrics,omitempty"`
	Category string                 `json:"category,omitempty"`
	Unit     string                 `json:"unit,omitempty"`
}

// HealthReport is the aggregated health check report containing overall system health,
// all provider results, and the evaluation timestamp.
type HealthReport struct {
	Overall   plugin.HealthStatus    `json:"overall"`
	Providers []ProviderHealthReport `json:"providers"`
	CheckedAt string                 `json:"checked_at"`
}

// Collector periodically polls registered HealthProviders and aggregates their diagnostics.
type Collector struct {
	mu           sync.RWMutex
	interval     time.Duration
	providers    []plugin.HealthProvider
	source       func() []plugin.HealthProvider
	latestReport HealthReport
	stopCh       chan struct{}
	running      bool
	timeout      time.Duration
}

// NewCollector constructs a new health Collector with the given interval and initial providers.
func NewCollector(interval time.Duration, providers ...plugin.HealthProvider) *Collector {
	if interval <= 0 {
		interval = DefaultInterval
	}
	c := &Collector{
		interval:  interval,
		providers: make([]plugin.HealthProvider, 0, len(providers)),
		stopCh:    make(chan struct{}),
		timeout:   10 * time.Second,
	}
	c.RegisterProviders(providers...)
	return c
}

// RegisterProvider adds a HealthProvider to the collector.
func (c *Collector) RegisterProvider(p plugin.HealthProvider) {
	if p == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.providers {
		if existing.Name() == p.Name() {
			return
		}
	}
	c.providers = append(c.providers, p)
}

// RegisterProviders registers multiple HealthProvider instances.
func (c *Collector) RegisterProviders(providers ...plugin.HealthProvider) {
	for _, p := range providers {
		c.RegisterProvider(p)
	}
}

// SetProviderSource sets a dynamic function to discover additional HealthProviders
// (e.g. from the plugin registry).
func (c *Collector) SetProviderSource(source func() []plugin.HealthProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.source = source
}

// SetTimeout configures the timeout for executing provider checks.
func (c *Collector) SetTimeout(t time.Duration) {
	if t > 0 {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.timeout = t
	}
}

// Start begins periodic polling of health providers in a background goroutine.
func (c *Collector) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopCh = make(chan struct{})
	interval := c.interval
	c.mu.Unlock()

	// Perform initial collection immediately
	_ = c.Collect(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-c.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.Collect(ctx)
			}
		}
	}()
}

// Stop terminates the background collection loop.
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	c.running = false
	close(c.stopCh)
}

// Collect executes checks across all providers and stores the aggregated result.
func (c *Collector) Collect(ctx context.Context) HealthReport {
	c.mu.RLock()
	var allProviders []plugin.HealthProvider
	seen := make(map[string]bool)

	for _, p := range c.providers {
		if p != nil && !seen[p.Name()] {
			seen[p.Name()] = true
			allProviders = append(allProviders, p)
		}
	}
	if c.source != nil {
		for _, p := range c.source() {
			if p != nil && !seen[p.Name()] {
				seen[p.Name()] = true
				allProviders = append(allProviders, p)
			}
		}
	}
	timeout := c.timeout
	c.mu.RUnlock()

	checkCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	results := make([]ProviderHealthReport, len(allProviders))
	var wg sync.WaitGroup

	for i, p := range allProviders {
		wg.Add(1)
		go func(idx int, hp plugin.HealthProvider) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = ProviderHealthReport{
						Name:     hp.Name(),
						Status:   plugin.HealthError,
						Message:  fmt.Sprintf("health check panicked: %v", r),
						Category: hp.Metadata().Category,
						Unit:     hp.Metadata().Unit,
					}
				}
			}()
			res := hp.Check(checkCtx)
			meta := hp.Metadata()
			results[idx] = ProviderHealthReport{
				Name:     hp.Name(),
				Status:   res.Status,
				Message:  res.Message,
				Metrics:  res.Metrics,
				Category: meta.Category,
				Unit:     meta.Unit,
			}
		}(i, p)
	}

	wg.Wait()

	// Derive overall status
	overall := plugin.HealthOK
	for _, r := range results {
		if r.Status == plugin.HealthError {
			overall = plugin.HealthError
			break
		} else if r.Status == plugin.HealthWarn && overall != plugin.HealthError {
			overall = plugin.HealthWarn
		}
	}

	report := HealthReport{
		Overall:   overall,
		Providers: results,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}

	c.mu.Lock()
	c.latestReport = report
	c.mu.Unlock()

	return report
}

// LatestReport returns the most recently collected HealthReport.
// If no report has been collected yet, it triggers an immediate synchronous collection.
func (c *Collector) LatestReport(ctx context.Context) HealthReport {
	c.mu.RLock()
	report := c.latestReport
	c.mu.RUnlock()

	if report.CheckedAt == "" {
		return c.Collect(ctx)
	}
	return report
}
