// Package config loads runtime configuration from environment variables.
package config

import (
	"bufio"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
)

// MinSecretKeyLen is the minimum acceptable length for OCTARQ_SECRET_KEY. It is
// the KEK for AES-GCM credential encryption and the HMAC key for session
// cookies; a short key is brute-forceable. Exported so the startup readiness
// report can name the same threshold Load enforces instead of restating it.
const MinSecretKeyLen = 16

// DefaultAppName is the fallback product name shown in the UI when the
// `app_name` runtime setting (Settings → General) is empty. Downstream builds
// (e.g. the commercial octarq-pro) override it at compile time via
// -ldflags="-X github.com/octarq-org/octarq/config.DefaultAppName=…".
var DefaultAppName = "octarq"

// Config holds all runtime configuration for octarq.
type Config struct {
	Listen string // e.g. ":8080"

	DBDriver string // "sqlite" | "postgres"
	DBDSN    string // sqlite: file path; postgres: connection string

	// SecretKey seeds both the session-cookie HMAC and the AES-GCM used to
	// encrypt provider credentials at rest. Must be stable across restarts.
	// With envelope encryption it is the KEK that wraps the data key (DEK).
	SecretKey string

	AdminUser     string
	AdminPassword string

	// TrustProxy controls whether proxy-supplied headers are honoured:
	// X-Forwarded-For / X-Real-IP when determining the client IP (rate limiting
	// and abuse throttling), and X-Forwarded-Proto when deciding whether the
	// request reached us over TLS (the session cookie's Secure attribute — see
	// origin.Secure). Only enable when octarq sits behind a reverse
	// proxy that sets these headers itself; otherwise clients spoof them to
	// evade per-IP limits or to claim a plaintext request was HTTPS. Set via
	// OCTARQ_TRUST_PROXY=true|1. Off by default.
	TrustProxy bool

	// AllowPrivateWebhooks lets outbound webhook / notification delivery reach
	// private, loopback, or link-local addresses. Off by default so a tenant
	// can't point a webhook at internal services or cloud metadata (SSRF); a
	// self-hoster running their own receiver on the same box/LAN opts in via
	// OCTARQ_ALLOW_PRIVATE_WEBHOOKS=true|1. Never relaxes the link-preview client.
	AllowPrivateWebhooks bool

	// AllowPrivateSMTP lets outbound SMTP mail delivery reach private, loopback,
	// or link-local addresses. Off by default so a tenant cannot point SMTP at
	// internal services; a self-hoster running their own postfix/mailhog on the
	// same box/LAN opts in via OCTARQ_ALLOW_PRIVATE_SMTP=true|1.
	AllowPrivateSMTP bool

	GeoIPDB string // optional path to a MaxMind GeoLite2-City.mmdb

	// Absolute URLs (password-reset and email-verification links, invite links,
	// OAuth redirect_uri) are NOT configured here. They are derived from the
	// request that asks for them, validated against the hostnames this instance
	// has registered — see origin.

	// RedisURL configures the optional Redis connection (e.g. "redis://localhost:6379").
	// If empty, Redis-based features will be disabled or fall back to DB/in-memory.
	RedisURL string

	// PublicCORSOrigins is the bootstrap allowlist of exact origins (e.g.
	// "https://octarq.org") allowed to read public GET API endpoints
	// cross-origin. It is only the startup fallback: once the runtime
	// `public_cors_origins` setting is set, that wins. An empty value disables
	// CORS entirely (today's behaviour). Never "*" — and even the configured
	// origins never get credentials.
	PublicCORSOrigins string

	// LogLevel is the slog severity threshold for the process-wide default
	// logger: "debug", "info", "warn", or "error". Set via OCTARQ_LOG_LEVEL.
	// An unrecognised value is a fatal startup error (like OCTARQ_DB_DRIVER),
	// never a silent fallback — a typo that downgrades to info would quietly
	// silence ops debugging. Empty means "info".
	LogLevel string
}

// validLogLevels is the vocabulary OCTARQ_LOG_LEVEL accepts. Keeping the list
// here makes the config package the single owner of what "a log level is".
var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// slogsByLevel maps the OCTARQ_LOG_LEVEL vocabulary to slog severity levels so
// the process logger and the config value can never disagree.
var slogsByLevel = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// normalizeLogLevel trims and case-folds an OCTARQ_LOG_LEVEL value; an empty
// (set-but-blank) value behaves like an unset one and resolves to the default
// "info". Shared by Load and LogLevel so the two can never disagree.
func normalizeLogLevel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "info"
	}
	return v
}

// LogLevel resolves OCTARQ_LOG_LEVEL to the slog severity threshold for the
// process default logger. It is read eagerly (before config.Load) because the
// logger is a process-wide singleton configured in main; config.Load validates
// the same value again for any code that reads Config.LogLevel. An unknown
// value is an error that names the variable — never a silent downgrade to a
// quieter level that would swallow the very logs the operator is reaching for.
func LogLevel() (slog.Level, error) {
	l, ok := slogsByLevel[normalizeLogLevel(env("OCTARQ_LOG_LEVEL", "info"))]
	if !ok {
		return slog.LevelInfo, fmt.Errorf("OCTARQ_LOG_LEVEL must be debug, info, warn or error, got %q", normalizeLogLevel(env("OCTARQ_LOG_LEVEL", "info")))
	}
	return l, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// loadDotEnv reads KEY=VALUE pairs from a .env file (if present) into the
// process environment. Existing environment variables always win, so explicit
// env overrides the file. Missing file is not an error. Supports blank lines,
// "#" comments (whole-line and trailing on unquoted values), an optional
// "export " prefix, and single/double quoted values.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		trimmed := strings.TrimSpace(val)
		switch {
		case strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) && len(trimmed) >= 2:
			val = trimmed[1 : len(trimmed)-1]
		case strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'") && len(trimmed) >= 2:
			val = trimmed[1 : len(trimmed)-1]
		default:
			// Strip a trailing inline comment: a "#" at the start of the value
			// or preceded by whitespace begins a comment.
			for i := 0; i < len(val); i++ {
				if val[i] == '#' && (i == 0 || val[i-1] == ' ' || val[i-1] == '\t') {
					val = val[:i]
					break
				}
			}
			val = strings.TrimSpace(val)
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}

// Load reads configuration from the environment, applying sane defaults.
func Load() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, fmt.Errorf("loading .env: %w", err)
	}
	c := &Config{
		Listen:        env("OCTARQ_LISTEN", ":8080"),
		DBDriver:      env("OCTARQ_DB_DRIVER", "sqlite"),
		DBDSN:         env("OCTARQ_DB_DSN", "octarq.db"),
		SecretKey:     env("OCTARQ_SECRET_KEY", ""),
		AdminUser:     env("OCTARQ_ADMIN_USER", "admin"),
		AdminPassword: env("OCTARQ_ADMIN_PASSWORD", ""),

		TrustProxy: strings.EqualFold(strings.TrimSpace(env("OCTARQ_TRUST_PROXY", "")), "true") || strings.TrimSpace(env("OCTARQ_TRUST_PROXY", "")) == "1",

		AllowPrivateWebhooks: strings.EqualFold(strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "")), "true") || strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "")) == "1",

		AllowPrivateSMTP: strings.EqualFold(strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_SMTP", "")), "true") || strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_SMTP", "")) == "1",

		GeoIPDB:  env("OCTARQ_GEOIP_DB", ""),
		RedisURL: env("OCTARQ_REDIS_URL", ""),

		PublicCORSOrigins: env("OCTARQ_CORS_ORIGINS", ""),

		LogLevel: normalizeLogLevel(env("OCTARQ_LOG_LEVEL", "info")),
	}
	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" {
		return nil, fmt.Errorf("OCTARQ_DB_DRIVER must be sqlite or postgres, got %q", c.DBDriver)
	}
	if c.LogLevel != "info" && !validLogLevels[c.LogLevel] {
		return nil, fmt.Errorf("OCTARQ_LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel)
	}
	// Zero-config boot: when the secret key and/or admin password are absent,
	// generate and persist them next to the database so `docker run` needs no
	// .env. Env-supplied values still win and are never written to disk.
	if err := c.ensureAutoSecrets(); err != nil {
		return nil, err
	}
	if c.SecretKey == "" {
		return nil, fmt.Errorf("OCTARQ_SECRET_KEY is required (used for sessions and credential encryption)")
	}

	if c.AdminPassword == "" {
		return nil, fmt.Errorf("OCTARQ_ADMIN_PASSWORD is required")
	}
	// A weak secret key undermines both credential encryption and cookie
	// integrity. Hard-fail on a provisioned deployment; warn otherwise so the
	// documented local dev key (OCTARQ_SECRET_KEY=dev) keeps working.
	if len(c.SecretKey) < MinSecretKeyLen {
		if c.Provisioned() {
			return nil, fmt.Errorf("OCTARQ_SECRET_KEY must be at least %d bytes when octarq is pointed at provisioned infrastructure (%s)", MinSecretKeyLen, c.provisionedBecause())
		}
		log.Printf("WARNING: OCTARQ_SECRET_KEY is only %d bytes; use at least %d bytes (e.g. `openssl rand -hex 32`) before production", len(c.SecretKey), MinSecretKeyLen)
	}
	return c, nil
}

// Provisioned reports whether this instance is pointed at infrastructure
// somebody stood up on purpose: an external Postgres, or an external Redis.
//
// It replaces the old IsProduction, which keyed on an https OCTARQ_BASE_URL or
// a set OCTARQ_ADMIN_HOST — both gone, now that absolute URLs come from the
// request (origin) and the dashboard host comes from the domains
// table. It is the SINGLE strictness predicate in this package: the secret-key
// floor is the only rule that uses it, and a second notion of "is this
// production" must not be introduced beside it. Note that full strictness
// enforcement consists of two halves: environment signals (this function)
// plus registered domains (at startup, requiring DB access via
// app.enforceSecretKeyFloor). Changing one must stay in sync with the other.
//
// It is deliberately not an env var — adding one would just move the decision
// back onto the operator, and an operator who would set it correctly is an
// operator who would already have set a long key.
//
// The trade-off is honest: a deployment on the default sqlite file with no
// Redis is treated as development and only gets a warning at config load time
// (unless a domain is registered, in which case app.enforceSecretKeyFloor
// enforces the secret-key floor at boot time), where an https OCTARQ_BASE_URL
// used to make it fatal. Nobody reaches Postgres or Redis by accident, though,
// whereas a laptop reaches sqlite by default — so the signal no longer fires
// on the setup it must not break.
func (c *Config) Provisioned() bool {
	return c.DBDriver == "postgres" || strings.TrimSpace(c.RedisURL) != ""
}

// provisionedBecause names the signal that made Provisioned true, so the
// startup error tells the operator which knob put them in the strict path.
func (c *Config) provisionedBecause() string {
	if c.DBDriver == "postgres" {
		return "OCTARQ_DB_DRIVER=postgres"
	}
	return "OCTARQ_REDIS_URL is set"
}
