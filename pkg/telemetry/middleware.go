package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wrote {
		rw.status = code
		rw.wrote = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wrote {
		rw.status = http.StatusOK
		rw.wrote = true
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += int64(n)
	return n, err
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// HTTPMiddleware creates a middleware that instruments incoming HTTP requests with tracing & metrics.
func HTTPMiddleware(tracerName string) func(http.Handler) http.Handler {
	if tracerName == "" {
		tracerName = "github.com/octarq-org/octarq/http"
	}
	tr := Tracer(tracerName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
			ctx, span := tr.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.URLQuery(r.URL.RawQuery),
					semconv.UserAgentOriginal(r.UserAgent()),
					attribute.String("http.host", r.Host),
				),
			)
			defer span.End()

			// Expose trace ID on response header for client-side debugging
			if sc := span.SpanContext(); sc.IsValid() {
				w.Header().Set("X-Trace-Id", sc.TraceID().String())
			}

			// In-flight gauge
			tel := Global()
			if tel.Metrics != nil && tel.Metrics.HTTPRequestsInFlight != nil {
				tel.Metrics.HTTPRequestsInFlight.Add(ctx, 1)
				defer tel.Metrics.HTTPRequestsInFlight.Add(ctx, -1)
			}

			rw := &responseWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
			}

			r = r.WithContext(ctx)
			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			// Record span status & attributes
			span.SetAttributes(
				semconv.HTTPResponseStatusCode(rw.status),
				attribute.Int64("http.response_content_length", rw.bytes),
			)

			if rw.status >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", rw.status))
			} else {
				span.SetStatus(codes.Ok, "")
			}

			// Record server metrics
			if tel.Metrics != nil {
				tel.Metrics.RecordHTTPRequest(ctx, r.Method, r.URL.Path, rw.status, duration, rw.bytes)
			}
		})
	}
}

// TraceLogHandler returns a slog.Handler middleware that enriches log records with active trace_id and span_id.
type TraceLogHandler struct {
	slog.Handler
}

func (h *TraceLogHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		r.AddAttrs(
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *TraceLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceLogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *TraceLogHandler) WithGroup(name string) slog.Handler {
	return &TraceLogHandler{Handler: h.Handler.WithGroup(name)}
}
