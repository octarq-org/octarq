# OpenTelemetry Observability in Octarq

Octarq provides first-class, out-of-the-box OpenTelemetry integration for distributed tracing and metrics monitoring.

## Features

- **Distributed Tracing**:
  - Full W3C Trace Context (`traceparent`, `tracestate`) and W3C Baggage propagation.
  - End-to-end HTTP request tracing (server spans, request/response headers, status codes, route paths).
  - Outbound HTTP request tracing (`safehttp`, webhook delivery, external APIs).
  - Database operation spans (GORM queries, transactions, inserts, updates, deletes).
  - Redis cache command tracing (`redisotel`).
  - Background task tracing (queue enqueue to execution linking).
  - Webhook delivery attempt tracing.
  - Structured logging correlation: `slog` automatically stamps `trace_id` and `span_id`.

- **Metrics Monitoring**:
  - HTTP server metrics (`http_requests_total`, `http_request_duration_seconds`, `http_requests_in_flight`, `http_response_size_bytes`).
  - Database connection pool gauges (`db_connections_open`, `db_connections_in_use`, `db_connections_idle`).
  - Cache hits & misses (`cache_hits_total`, `cache_misses_total`, `cache_operations_total`).
  - Background queue metrics (`queue_tasks_total`, `queue_task_duration_seconds`, `queue_tasks_in_flight`).
  - Webhook delivery metrics (`webhook_deliveries_total`, `webhook_delivery_duration_seconds`).
  - Business metrics (`links_clicks_total`, `links_created_total`, `emails_received_total`, `emails_sent_total`).
  - Dual metrics endpoint (`/metrics`): Prometheus scraper format by default, with JSON expvar format fallback via `?format=json`.

## Quickstart with Docker Compose

Run the pre-configured observability stack (Octarq + OpenTelemetry Collector + Jaeger + Prometheus):

```bash
docker compose -f deploy/docker-compose.otel.yml up -d
```

- **Octarq Dashboard & API**: [http://localhost:8080](http://localhost:8080)
- **Jaeger Tracing UI**: [http://localhost:16686](http://localhost:16686)
- **Prometheus Metrics UI**: [http://localhost:9090](http://localhost:9090)
- **Metrics Endpoint**: [http://localhost:8080/metrics](http://localhost:8080/metrics)

## Configuration Reference

All standard OpenTelemetry environment variables are supported:

| Environment Variable | Default | Description |
|---|---|---|
| `OCTARQ_OTEL_ENABLED` | `false` | Enable OpenTelemetry SDK (must be explicitly set to `true`) |
| `OTEL_SERVICE_NAME` | `octarq` (or `octarq-pro`) | Logical name of the service |
| `OTEL_SERVICE_VERSION` | `dev` / build version | Version of the service |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | Target OTLP collector endpoint (e.g. `http://localhost:4318` or `localhost:4317`) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | OTLP protocol (`grpc`, `http/protobuf`, `http/json`) |
| `OTEL_EXPORTER_OTLP_INSECURE` | `false` | Disable TLS for OTLP export |
| `OTEL_EXPORTER_OTLP_HEADERS` | `""` | Key-value pairs for headers (e.g. `Authorization=Bearer xxx`) |
| `OTEL_TRACES_EXPORTER` | `otlp` | Traces exporter (`otlp`, `stdout`, `none`) |
| `OTEL_METRICS_EXPORTER` | `prometheus` | Metrics exporter (`prometheus`, `otlp`, `stdout`, `none`) |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` | Sampler (`always_on`, `always_off`, `traceidratio`, `parentbased_always_on`, `parentbased_traceidratio`) |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` | Sampling ratio (0.0 - 1.0) for `traceidratio` |
| `OTEL_RESOURCE_ATTRIBUTES` | `""` | Custom attributes (e.g. `environment=production,region=us-east-1`) |

## Plugin Developer Guide

Plugins can start spans and record metrics using `plugin.Context` or the helper functions:

```go
func (p *MyPlugin) MyOperation(ctx context.Context) error {
    ctx, span := plugin.StartSpan(ctx, "myplugin", "operation.name")
    defer span.End()

    // ... business logic ...
    return nil
}
```
