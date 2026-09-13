package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan starts a span using the named tracer.
func StartSpan(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tr := Tracer(tracerName)
	return tr.Start(ctx, spanName, opts...)
}

// SpanFromContext returns the current span from context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceID returns the hex trace ID for the current active span, or empty if none.
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// SpanID returns the hex span ID for the current active span, or empty if none.
func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// InjectHTTP injects the trace context from ctx into HTTP request headers using the global propagator.
func InjectHTTP(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

// ExtractHTTP extracts trace context from HTTP request headers and returns a new context.
func ExtractHTTP(ctx context.Context, header http.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(header))
}

// InjectMap injects trace context into a string map (useful for async tasks / message headers).
func InjectMap(ctx context.Context, carrier map[string]string) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(carrier))
}

// ExtractMap extracts trace context from a string map.
func ExtractMap(ctx context.Context, carrier map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
}

// RecordError records an error on the span and sets the span status to Error.
func RecordError(span trace.Span, err error, opts ...trace.EventOption) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err, opts...)
	span.SetStatus(codes.Error, err.Error())
}

// SetOK marks the span status as OK.
func SetOK(span trace.Span) {
	if span != nil {
		span.SetStatus(codes.Ok, "")
	}
}

// AddEvent adds an event to the span.
func AddEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	if span != nil {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}
