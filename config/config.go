// Package config loads runtime configuration from environment variables.
package config

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
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
	Listen    string // e.g. ":8080"
	AdminHost string // host that serves the dashboard; empty = serve dashboard on any non-link host

	DBDriver string // "sqlite" | "postgres"
	DBDSN    string // sqlite: file path; postgres: connection string

	// SecretKey seeds both the session-cookie HMAC and the AES-GCM used to
	// encrypt provider credentials at rest. Must be stable across restarts.
	// With envelope encryption it is the KEK that wraps the data key (DEK).
	SecretKey string

	AdminUser     string
	AdminPassword string

	// SecureCookies adds the Secure attribute to the session cookie (HTTPS-only).
	// Auto-enabled when the deployment looks production (BaseURL is https or
	// AdminHost is set); force with OCTARQ_SECURE_COOKIES=true|false. Off by default
	// for plain-http localhost dev, where a Secure cookie would never be sent.
	SecureCookies bool

	// TrustProxy controls whether X-Forwarded-For / X-Real-IP are honoured when
	// determining the client IP (for rate limiting and abuse throttling). Only
	// enable when octarq sits behind a trusted reverse proxy that sets these
	// headers; otherwise clients can spoof them to evade per-IP limits. Set via
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

	// BaseURL is the public-facing URL every absolute link is built from —
	// password-reset and email-verification links, OAuth callback URIs and
	// outbound webhook addresses — e.g. "https://app.example.com". Required in
	// production (see validateBaseURL); empty in development degrades those
	// links to relative paths and disables OAuth login.
	BaseURL string

	// RedisURL configures the optional Redis connection (e.g. "redis://localhost:6379").
	// If empty, Redis-based features will be disabled or fall back to DB/in-memory.
	RedisURL string
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
		AdminHost:     env("OCTARQ_ADMIN_HOST", ""),
		DBDriver:      env("OCTARQ_DB_DRIVER", "sqlite"),
		DBDSN:         env("OCTARQ_DB_DSN", "octarq.db"),
		SecretKey:     env("OCTARQ_SECRET_KEY", ""),
		AdminUser:     env("OCTARQ_ADMIN_USER", "admin"),
		AdminPassword: env("OCTARQ_ADMIN_PASSWORD", ""),

		TrustProxy: strings.EqualFold(strings.TrimSpace(env("OCTARQ_TRUST_PROXY", "")), "true") || strings.TrimSpace(env("OCTARQ_TRUST_PROXY", "")) == "1",

		AllowPrivateWebhooks: strings.EqualFold(strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "")), "true") || strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "")) == "1",

		AllowPrivateSMTP: strings.EqualFold(strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_SMTP", "")), "true") || strings.TrimSpace(env("OCTARQ_ALLOW_PRIVATE_SMTP", "")) == "1",

		GeoIPDB:  env("OCTARQ_GEOIP_DB", ""),
		BaseURL:  env("OCTARQ_BASE_URL", ""),
		RedisURL: env("OCTARQ_REDIS_URL", ""),
	}
	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" {
		return nil, fmt.Errorf("OCTARQ_DB_DRIVER must be sqlite or postgres, got %q", c.DBDriver)
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

	// Secure cookies: auto-on when prod-looking, overridable by env.
	c.SecureCookies = strings.HasPrefix(strings.ToLower(c.BaseURL), "https://") || c.AdminHost != ""
	if v, ok := os.LookupEnv("OCTARQ_SECURE_COOKIES"); ok {
		c.SecureCookies = strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	}
	if c.AdminPassword == "" {
		return nil, fmt.Errorf("OCTARQ_ADMIN_PASSWORD is required")
	}
	// A weak secret key undermines both credential encryption and cookie
	// integrity. Hard-fail in production-looking setups; warn otherwise so the
	// documented local dev key (OCTARQ_SECRET_KEY=dev) keeps working.
	if len(c.SecretKey) < MinSecretKeyLen {
		if c.IsProduction() {
			return nil, fmt.Errorf("OCTARQ_SECRET_KEY must be at least %d bytes in production", MinSecretKeyLen)
		}
		log.Printf("WARNING: OCTARQ_SECRET_KEY is only %d bytes; use at least %d bytes (e.g. `openssl rand -hex 32`) before production", len(c.SecretKey), MinSecretKeyLen)
	}
	if err := c.validateBaseURL(); err != nil {
		return nil, err
	}
	return c, nil
}

// IsProduction reports whether this deployment looks production-facing.
//
// It is deliberately the SAME signal that decides Secure cookies (see the
// SecureCookies field): an https:// BaseURL or a configured OCTARQ_ADMIN_HOST,
// with OCTARQ_SECURE_COOKIES as the explicit override. Every "strict in
// production, lenient in development" rule in Load keys on this one predicate —
// the secret-key length floor and the BaseURL requirement both — so an operator
// only ever has to reason about one notion of "is this production". Do not
// introduce a second one.
//
// It is only meaningful after Load has computed SecureCookies.
func (c *Config) IsProduction() bool { return c.SecureCookies }

// validateBaseURL checks OCTARQ_BASE_URL, which is the only source of absolute
// URLs in the product: password-reset and email-verification links
// (internal/api/recovery.go), OAuth callback URIs, and outbound webhook
// addresses are all built by prefixing it. Left empty, those come out as bare
// paths like "/admin/reset?token=…" — unopenable from a mail client — while the
// instance boots and logs nothing.
//
// A malformed value is always fatal: it cannot produce a working link anywhere,
// so there is no mode in which tolerating it helps. An empty value follows the
// same production/development split as the secret-key floor (see IsProduction):
// fatal for a production-looking deployment, a warning for local development,
// where relative links are merely inconvenient and the operator is the only user.
func (c *Config) validateBaseURL() error {
	raw := strings.TrimSpace(c.BaseURL)
	if raw == "" {
		if c.IsProduction() {
			return fmt.Errorf("OCTARQ_BASE_URL is required in production: it is the only source of absolute URLs — password-reset and email-verification links, OAuth callback URIs and outbound webhook addresses are all built from it, and without it they are emitted as relative paths that cannot be opened from a mail client. Set OCTARQ_BASE_URL=https://your.domain")
		}
		log.Printf("WARNING: OCTARQ_BASE_URL is not set; password-reset and email-verification links will be relative paths (unopenable from a mail client) and OAuth callbacks and webhook addresses cannot be built. Set OCTARQ_BASE_URL=http://localhost:8080 for local use")
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("OCTARQ_BASE_URL %q is not a parsable URL: %w — expected an absolute URL such as https://your.domain", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("OCTARQ_BASE_URL %q must be an absolute URL with a scheme and a host, such as https://your.domain — it is prefixed onto password-reset links, OAuth callbacks and webhook addresses", raw)
	}
	return nil
}
