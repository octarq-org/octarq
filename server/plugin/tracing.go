package plugin

import (
	"context"

	"github.com/octarq-org/octarq/server/pkg/telemetry"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan starts an OpenTelemetry span for a plugin operation.
func StartSpan(ctx context.Context, pluginName, opName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return telemetry.StartSpan(ctx, "github.com/octarq-org/octarq/server/plugin/"+pluginName, opName, opts...)
}
