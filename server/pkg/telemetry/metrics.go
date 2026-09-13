package telemetry

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics bundles all pre-defined application metric instruments.
type Metrics struct {
	// HTTP Server
	HTTPRequestsTotal     metric.Int64Counter
	HTTPRequestDuration   metric.Float64Histogram
	HTTPRequestsInFlight  metric.Int64UpDownCounter
	HTTPResponseSizeBytes metric.Int64Histogram

	// Database
	DBQueryDuration metric.Float64Histogram

	// Cache
	CacheHitsTotal       metric.Int64Counter
	CacheMissesTotal     metric.Int64Counter
	CacheOperationsTotal metric.Int64Counter

	// Queue / Background Jobs
	QueueTasksTotal    metric.Int64Counter
	QueueTaskDuration  metric.Float64Histogram
	QueueTasksInFlight metric.Int64UpDownCounter

	// Webhooks & Events
	WebhookDeliveriesTotal  metric.Int64Counter
	WebhookDeliveryDuration metric.Float64Histogram

	// Business / Domain
	LinksClicksTotal    metric.Int64Counter
	LinksCreatedTotal   metric.Int64Counter
	EmailsReceivedTotal metric.Int64Counter
	EmailsSentTotal     metric.Int64Counter
}

// NewMetrics initializes all metric instruments using the provided Meter.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{}

	var err error

	// 1. HTTP Server Instruments
	if m.HTTPRequestsTotal, err = meter.Int64Counter("http_requests_total",
		metric.WithDescription("Total number of HTTP requests processed"),
		metric.WithUnit("{request}"),
	); err != nil {
		return nil, err
	}

	if m.HTTPRequestDuration, err = meter.Float64Histogram("http_request_duration_seconds",
		metric.WithDescription("HTTP request execution duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	); err != nil {
		return nil, err
	}

	if m.HTTPRequestsInFlight, err = meter.Int64UpDownCounter("http_requests_in_flight",
		metric.WithDescription("Current number of active HTTP requests"),
		metric.WithUnit("{request}"),
	); err != nil {
		return nil, err
	}

	if m.HTTPResponseSizeBytes, err = meter.Int64Histogram("http_response_size_bytes",
		metric.WithDescription("HTTP response size in bytes"),
		metric.WithUnit("By"),
	); err != nil {
		return nil, err
	}

	// 2. Database Instruments
	if m.DBQueryDuration, err = meter.Float64Histogram("db_query_duration_seconds",
		metric.WithDescription("Database operation latency in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2),
	); err != nil {
		return nil, err
	}

	// 3. Cache Instruments
	if m.CacheHitsTotal, err = meter.Int64Counter("cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
		metric.WithUnit("{hit}"),
	); err != nil {
		return nil, err
	}

	if m.CacheMissesTotal, err = meter.Int64Counter("cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
		metric.WithUnit("{miss}"),
	); err != nil {
		return nil, err
	}

	if m.CacheOperationsTotal, err = meter.Int64Counter("cache_operations_total",
		metric.WithDescription("Total number of cache operations performed"),
		metric.WithUnit("{operation}"),
	); err != nil {
		return nil, err
	}

	// 4. Queue / Tasks Instruments
	if m.QueueTasksTotal, err = meter.Int64Counter("queue_tasks_total",
		metric.WithDescription("Total background tasks processed"),
		metric.WithUnit("{task}"),
	); err != nil {
		return nil, err
	}

	if m.QueueTaskDuration, err = meter.Float64Histogram("queue_task_duration_seconds",
		metric.WithDescription("Duration of background task execution"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30, 60),
	); err != nil {
		return nil, err
	}

	if m.QueueTasksInFlight, err = meter.Int64UpDownCounter("queue_tasks_in_flight",
		metric.WithDescription("Current number of active background tasks in progress"),
		metric.WithUnit("{task}"),
	); err != nil {
		return nil, err
	}

	// 5. Webhooks & Events Instruments
	if m.WebhookDeliveriesTotal, err = meter.Int64Counter("webhook_deliveries_total",
		metric.WithDescription("Total outbound webhook delivery attempts"),
		metric.WithUnit("{delivery}"),
	); err != nil {
		return nil, err
	}

	if m.WebhookDeliveryDuration, err = meter.Float64Histogram("webhook_delivery_duration_seconds",
		metric.WithDescription("Outbound webhook delivery latency in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10),
	); err != nil {
		return nil, err
	}

	// 6. Business Domain Instruments
	if m.LinksClicksTotal, err = meter.Int64Counter("links_clicks_total",
		metric.WithDescription("Total short-link redirection clicks handled"),
		metric.WithUnit("{click}"),
	); err != nil {
		return nil, err
	}

	if m.LinksCreatedTotal, err = meter.Int64Counter("links_created_total",
		metric.WithDescription("Total short links created"),
		metric.WithUnit("{link}"),
	); err != nil {
		return nil, err
	}

	if m.EmailsReceivedTotal, err = meter.Int64Counter("emails_received_total",
		metric.WithDescription("Total inbound emails processed"),
		metric.WithUnit("{email}"),
	); err != nil {
		return nil, err
	}

	if m.EmailsSentTotal, err = meter.Int64Counter("emails_sent_total",
		metric.WithDescription("Total outbound emails dispatched"),
		metric.WithUnit("{email}"),
	); err != nil {
		return nil, err
	}

	return m, nil
}

// NewNoopMetrics returns an empty Metrics struct that does nothing when called.
func NewNoopMetrics() *Metrics {
	return &Metrics{}
}

// RecordHTTPRequest records server request metrics.
func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, route string, status int, duration time.Duration, bytes int64) {
	if m == nil || m.HTTPRequestsTotal == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.route", route),
		attribute.Int("http.status_code", status),
		attribute.String("http.status_group", strconv.Itoa(status/100)+"xx"),
	}
	opt := metric.WithAttributes(attrs...)
	m.HTTPRequestsTotal.Add(ctx, 1, opt)
	m.HTTPRequestDuration.Record(ctx, duration.Seconds(), opt)
	if bytes > 0 && m.HTTPResponseSizeBytes != nil {
		m.HTTPResponseSizeBytes.Record(ctx, bytes, opt)
	}
}

// RecordCacheHit records a cache hit.
func (m *Metrics) RecordCacheHit(ctx context.Context, layer string) {
	if m == nil || m.CacheHitsTotal == nil {
		return
	}
	m.CacheHitsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("layer", layer)))
}

// RecordCacheMiss records a cache miss.
func (m *Metrics) RecordCacheMiss(ctx context.Context, layer string) {
	if m == nil || m.CacheMissesTotal == nil {
		return
	}
	m.CacheMissesTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("layer", layer)))
}

// RecordQueueTask records a completed background task.
func (m *Metrics) RecordQueueTask(ctx context.Context, taskType, status string, duration time.Duration) {
	if m == nil || m.QueueTasksTotal == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("task.type", taskType),
		attribute.String("status", status),
	)
	m.QueueTasksTotal.Add(ctx, 1, attrs)
	m.QueueTaskDuration.Record(ctx, duration.Seconds(), attrs)
}

// RecordWebhookDelivery records a webhook delivery attempt.
func (m *Metrics) RecordWebhookDelivery(ctx context.Context, event string, status int, duration time.Duration) {
	if m == nil || m.WebhookDeliveriesTotal == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("event", event),
		attribute.Int("http.status_code", status),
	)
	m.WebhookDeliveriesTotal.Add(ctx, 1, attrs)
	m.WebhookDeliveryDuration.Record(ctx, duration.Seconds(), attrs)
}

// RegisterDBStatsMetrics registers observable gauges for database connection pool statistics.
func RegisterDBStatsMetrics(meter metric.Meter, sqlDB *sql.DB) error {
	if sqlDB == nil || meter == nil {
		return nil
	}

	_, err := meter.Int64ObservableGauge("db_connections_open",
		metric.WithDescription("Number of open connections to the database"),
		metric.WithUnit("{connection}"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			stats := sqlDB.Stats()
			obs.Observe(int64(stats.OpenConnections))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("db_connections_in_use",
		metric.WithDescription("Number of in-use connections to the database"),
		metric.WithUnit("{connection}"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			stats := sqlDB.Stats()
			obs.Observe(int64(stats.InUse))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge("db_connections_idle",
		metric.WithDescription("Number of idle connections in the database pool"),
		metric.WithUnit("{connection}"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			stats := sqlDB.Stats()
			obs.Observe(int64(stats.Idle))
			return nil
		}),
	)
	return err
}
