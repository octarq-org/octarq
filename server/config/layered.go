// Package config resolves operator configuration from layered sources with
// one precedence order: built-in defaults < YAML file < dotenv file <
// process environment < explicit overrides (CLI flags). The koanf registry
// owns the merge; Config construction and every Fail-Closed validation stay
// in one builder so all entry points share identical strictness.
package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Layered keys (koanf paths). YAML files use these dotted paths; environment
// variables and CLI overrides use the mapped OCTARQ_* / OTEL_* names.
var layeredKeys = []struct {
	Env  string
	Path string
}{
	{"OCTARQ_LISTEN", "listen"},
	{"OCTARQ_DB_DRIVER", "db.driver"},
	{"OCTARQ_DB_DSN", "db.dsn"},
	{"OCTARQ_SECRET_KEY", "secret.key"},
	{"OCTARQ_ADMIN_USER", "admin.user"},
	{"OCTARQ_ADMIN_PASSWORD", "admin.password"},
	{"OCTARQ_STORAGE_DIR", "storage.dir"},
	{"OCTARQ_TRUST_PROXY", "trust.proxy"},
	{"OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "webhooks.allow_private"},
	{"OCTARQ_ALLOW_PRIVATE_SMTP", "smtp.allow_private"},
	{"OCTARQ_GEOIP_DB", "geoip.db"},
	{"OCTARQ_REDIS_URL", "redis.url"},
	{"OCTARQ_CORS_ORIGINS", "cors.origins"},
	{"OCTARQ_DEV_WEB_PROXY", "dev.web_proxy"},
	{"OCTARQ_LOG_LEVEL", "log.level"},
	{"OCTARQ_OTEL_ENABLED", "otel.enabled"},
	{"OTEL_EXPORTER_OTLP_ENDPOINT", "otel.endpoint"},
	{"OTEL_SERVICE_NAME", "otel.service_name"},
	{"OTEL_TRACES_EXPORTER", "otel.traces_exporter"},
	{"OTEL_METRICS_EXPORTER", "otel.metrics_exporter"},
	{"OTEL_TRACES_SAMPLER", "otel.traces_sampler"},
	{"OTEL_EXPORTER_OTLP_INSECURE", "otel.insecure"},
	{"OTEL_EXPORTER_OTLP_HEADERS", "otel.headers"},
}

// LoadOptions selects the configuration sources for one load.
type LoadOptions struct {
	// DotEnvPath fills unset process variables ("" skips, ".env" is standard).
	DotEnvPath string
	// YAMLPath layers a YAML file under env ("" skips).
	YAMLPath string
	// Overrides wins over everything (CLI flags). Keys are OCTARQ_* env names.
	Overrides map[string]string
}

// Resolve merges all sources into a flat env-keyed map following the
// precedence: defaults < YAML < dotenv < environment < overrides.
func Resolve(opts LoadOptions) (map[string]string, error) {
	k := koanf.New(".")

	if opts.YAMLPath != "" {
		if err := k.Load(file.Provider(opts.YAMLPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load YAML config %q: %w", opts.YAMLPath, err)
		}
	}

	if opts.DotEnvPath != "" {
		// Preserve historical dotenv semantics (fill only unset process
		// variables) by loading through the same routine Load uses.
		if err := loadDotEnv(opts.DotEnvPath); err != nil {
			return nil, fmt.Errorf("loading %s: %w", opts.DotEnvPath, err)
		}
	}

	values := make(map[string]string, len(layeredKeys))
	for _, key := range layeredKeys {
		if opts.YAMLPath != "" && k.Exists(key.Path) {
			values[key.Env] = k.String(key.Path)
		}
		if v, ok := os.LookupEnv(key.Env); ok {
			values[key.Env] = v
		}
		if opts.Overrides != nil {
			if v, ok := opts.Overrides[key.Env]; ok {
				values[key.Env] = v
			}
		}
	}
	return values, nil
}

// LoadWithOptions builds the Config from layered sources with the same
// validation and Fail-Closed rules as Load.
func LoadWithOptions(opts LoadOptions) (*Config, error) {
	values, err := Resolve(opts)
	if err != nil {
		return nil, err
	}
	return buildFromValues(values)
}

func valueOf(values map[string]string, key, def string) string {
	if v, ok := values[key]; ok {
		return v
	}
	return def
}

func buildFromValues(values map[string]string) (*Config, error) {
	get := func(key, def string) string { return valueOf(values, key, def) }
	c := &Config{
		Listen:        get("OCTARQ_LISTEN", ":8080"),
		DBDriver:      get("OCTARQ_DB_DRIVER", "sqlite"),
		DBDSN:         get("OCTARQ_DB_DSN", "octarq.db"),
		SecretKey:     get("OCTARQ_SECRET_KEY", ""),
		AdminUser:     get("OCTARQ_ADMIN_USER", "admin"),
		AdminPassword: get("OCTARQ_ADMIN_PASSWORD", ""),
		StorageDir:    get("OCTARQ_STORAGE_DIR", "./data/storage"),

		TrustProxy: strings.EqualFold(strings.TrimSpace(get("OCTARQ_TRUST_PROXY", "")), "true") || strings.TrimSpace(get("OCTARQ_TRUST_PROXY", "")) == "1",

		AllowPrivateWebhooks: strings.EqualFold(strings.TrimSpace(get("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "")), "true") || strings.TrimSpace(get("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "")) == "1",

		AllowPrivateSMTP: strings.EqualFold(strings.TrimSpace(get("OCTARQ_ALLOW_PRIVATE_SMTP", "")), "true") || strings.TrimSpace(get("OCTARQ_ALLOW_PRIVATE_SMTP", "")) == "1",

		GeoIPDB:  get("OCTARQ_GEOIP_DB", ""),
		RedisURL: get("OCTARQ_REDIS_URL", ""),

		PublicCORSOrigins: get("OCTARQ_CORS_ORIGINS", ""),

		DevWebProxy: get("OCTARQ_DEV_WEB_PROXY", ""),

		LogLevel: normalizeLogLevel(get("OCTARQ_LOG_LEVEL", "info")),

		OTelEnabled:         strings.EqualFold(strings.TrimSpace(get("OCTARQ_OTEL_ENABLED", "")), "true") || strings.TrimSpace(get("OCTARQ_OTEL_ENABLED", "")) == "1",
		OTelEndpoint:        get("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelServiceName:     get("OTEL_SERVICE_NAME", DefaultAppName),
		OTelTracesExporter:  get("OTEL_TRACES_EXPORTER", "otlp"),
		OTelMetricsExporter: get("OTEL_METRICS_EXPORTER", "prometheus"),
		OTelSampler:         get("OTEL_TRACES_SAMPLER", "parentbased_always_on"),
		OTelInsecure:        strings.EqualFold(strings.TrimSpace(get("OTEL_EXPORTER_OTLP_INSECURE", "")), "true") || strings.TrimSpace(get("OTEL_EXPORTER_OTLP_INSECURE", "")) == "1",
		OTelHeaders:         get("OTEL_EXPORTER_OTLP_HEADERS", ""),
	}
	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" && c.DBDriver != "mysql" {
		return nil, fmt.Errorf("OCTARQ_DB_DRIVER must be sqlite, postgres, or mysql, got %q", c.DBDriver)
	}
	if c.LogLevel != "info" && !validLogLevels[c.LogLevel] {
		return nil, fmt.Errorf("OCTARQ_LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel)
	}
	if err := c.ensureAutoSecrets(); err != nil {
		return nil, err
	}
	if c.SecretKey == "" {
		return nil, fmt.Errorf("OCTARQ_SECRET_KEY is required (used for sessions and credential encryption)")
	}

	if c.AdminPassword == "" {
		return nil, fmt.Errorf("OCTARQ_ADMIN_PASSWORD is required")
	}
	if len(c.SecretKey) < MinSecretKeyLen {
		if c.Provisioned() {
			return nil, fmt.Errorf("OCTARQ_SECRET_KEY must be at least %d bytes when octarq is pointed at provisioned infrastructure (%s)", MinSecretKeyLen, c.provisionedBecause())
		}
		log.Printf("WARNING: OCTARQ_SECRET_KEY is only %d bytes; use at least %d bytes (e.g. `openssl rand -hex 32`) before production", len(c.SecretKey), MinSecretKeyLen)
	}
	return c, nil
}
