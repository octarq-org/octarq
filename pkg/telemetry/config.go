package telemetry

import (
	"os"
	"strconv"
	"strings"
)

// Config encapsulates configuration for OpenTelemetry tracing and metrics.
type Config struct {
	Enabled            bool
	ServiceName        string
	ServiceVersion     string
	Environment        string
	Endpoint           string
	TracesEndpoint     string
	MetricsEndpoint    string
	Protocol           string // "grpc", "http/protobuf", "http/json"
	Insecure           bool
	Headers            map[string]string
	TracesExporter     string // "otlp", "stdout", "none"
	MetricsExporter    string // "otlp", "prometheus", "stdout", "none"
	Sampler            string // "always_on", "always_off", "traceidratio", "parentbased_always_on", "parentbased_traceidratio"
	SampleRatio        float64
	ResourceAttributes map[string]string
}

// ConfigFromEnv reads standard OpenTelemetry and Octarq-specific environment variables.
func ConfigFromEnv(defaultServiceName, defaultVersion string) Config {
	if defaultServiceName == "" {
		defaultServiceName = "octarq"
	}
	if defaultVersion == "" {
		defaultVersion = "dev"
	}

	cfg := Config{
		ServiceName:        envOr("OTEL_SERVICE_NAME", defaultServiceName),
		ServiceVersion:     envOr("OTEL_SERVICE_VERSION", defaultVersion),
		Environment:        envOr("OCTARQ_ENV", envOr("ENVIRONMENT", "production")),
		Endpoint:           strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		TracesEndpoint:     strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")),
		MetricsEndpoint:    strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")),
		Protocol:           strings.ToLower(strings.TrimSpace(envOr("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"))),
		Insecure:           parseBool(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"), false),
		Headers:            parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		TracesExporter:     strings.ToLower(strings.TrimSpace(envOr("OTEL_TRACES_EXPORTER", "otlp"))),
		MetricsExporter:    strings.ToLower(strings.TrimSpace(envOr("OTEL_METRICS_EXPORTER", "prometheus"))),
		Sampler:            strings.ToLower(strings.TrimSpace(envOr("OTEL_TRACES_SAMPLER", "parentbased_always_on"))),
		SampleRatio:        parseFloat(os.Getenv("OTEL_TRACES_SAMPLER_ARG"), 1.0),
		ResourceAttributes: parseResourceAttributes(os.Getenv("OTEL_RESOURCE_ATTRIBUTES")),
	}

	// OCTARQ_OTEL_ENABLED / OTEL_SDK_DISABLED handling
	if sdkDisabled := parseBool(os.Getenv("OTEL_SDK_DISABLED"), false); sdkDisabled {
		cfg.Enabled = false
		return cfg
	}

	if val, exists := os.LookupEnv("OCTARQ_OTEL_ENABLED"); exists {
		cfg.Enabled = parseBool(val, false)
	} else {
		// Enabled by default if an endpoint or explicit exporter is configured,
		// or if prometheus metrics exporter is active.
		cfg.Enabled = cfg.Endpoint != "" || cfg.TracesEndpoint != "" || cfg.MetricsEndpoint != "" ||
			cfg.TracesExporter == "stdout" || cfg.MetricsExporter == "prometheus"
	}

	return cfg
}

func envOr(key, def string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return def
}

func parseBool(val string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "true" || v == "1" || v == "yes" || v == "on" {
		return true
	}
	if v == "false" || v == "0" || v == "no" || v == "off" {
		return false
	}
	return def
}

func parseFloat(val string, def float64) float64 {
	if val == "" {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return def
	}
	return f
}

func parseHeaders(h string) map[string]string {
	res := make(map[string]string)
	if h == "" {
		return res
	}
	pairs := strings.Split(h, ",")
	for _, p := range pairs {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok && k != "" {
			res[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return res
}

func parseResourceAttributes(ra string) map[string]string {
	return parseHeaders(ra)
}
