// Package config loads runtime configuration from environment variables.
package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// minSecretKeyLen is the minimum acceptable length for LED_SECRET_KEY. It is
// the KEK for AES-GCM credential encryption and the HMAC key for session
// cookies; a short key is brute-forceable.
const minSecretKeyLen = 16

// DefaultAppName is the fallback product name shown in the UI when the
// `app_name` runtime setting (Settings → General) is empty. Downstream builds
// (e.g. the commercial led-pro) override it at compile time via
// -ldflags="-X github.com/Jungley8/led/config.DefaultAppName=…".
var DefaultAppName = "led"

// Config holds all runtime configuration for led.
type Config struct {
	Listen    string // e.g. ":8080"
	AdminHost string // host that serves the dashboard; empty = serve dashboard on any non-link host

	DBDriver string // "sqlite" | "postgres"
	DBDSN    string // sqlite: file path; postgres: connection string

	// SecretKey seeds both the session-cookie HMAC and the AES-GCM used to
	// encrypt provider credentials at rest. Must be stable across restarts.
	// With envelope encryption it is the KEK that wraps the data key (DEK).
	SecretKey string

	// OldSecretKeys are previous LED_SECRET_KEY values, kept only during a key
	// rotation. They let the binary unwrap the DEK (and read not-yet-migrated
	// legacy ciphertext / sessions) that were written under the old key. On
	// startup the DEK is automatically re-wrapped under the current SecretKey, so
	// these can be dropped on the next restart. Sourced from LED_SECRET_KEY_OLD
	// (comma-separated).
	OldSecretKeys []string

	AdminUser     string
	AdminPassword string

	// SecureCookies adds the Secure attribute to the session cookie (HTTPS-only).
	// Auto-enabled when the deployment looks production (BaseURL is https or
	// AdminHost is set); force with LED_SECURE_COOKIES=true|false. Off by default
	// for plain-http localhost dev, where a Secure cookie would never be sent.
	SecureCookies bool

	// TrustProxy controls whether X-Forwarded-For / X-Real-IP are honoured when
	// determining the client IP (for rate limiting and abuse throttling). Only
	// enable when led sits behind a trusted reverse proxy that sets these
	// headers; otherwise clients can spoof them to evade per-IP limits. Set via
	// LED_TRUST_PROXY=true|1. Off by default.
	TrustProxy bool

	GeoIPDB string // optional path to a MaxMind GeoLite2-City.mmdb

	// BaseURL is the public-facing URL used to build OAuth callback URIs,
	// e.g. "https://app.example.com". Leave empty to disable OAuth login.
	BaseURL string

	// MCPOrgID scopes the `led mcp` server's convenience tools to one tenant's
	// data. Defaults to 1 (the single-operator self-hosted case). On a
	// multi-tenant deployment, run one MCP process per tenant with this set.
	MCPOrgID uint

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

// envInt reads an integer env var, falling back to def on absence or parse error.
func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
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
		Listen:        env("LED_LISTEN", ":8080"),
		AdminHost:     env("LED_ADMIN_HOST", ""),
		DBDriver:      env("LED_DB_DRIVER", "sqlite"),
		DBDSN:         env("LED_DB_DSN", "led.db"),
		SecretKey:     env("LED_SECRET_KEY", ""),
		AdminUser:     env("LED_ADMIN_USER", "admin"),
		AdminPassword: env("LED_ADMIN_PASSWORD", ""),

		TrustProxy: strings.EqualFold(strings.TrimSpace(env("LED_TRUST_PROXY", "")), "true") || strings.TrimSpace(env("LED_TRUST_PROXY", "")) == "1",

		GeoIPDB:  env("LED_GEOIP_DB", ""),
		BaseURL:  env("LED_BASE_URL", ""),
		MCPOrgID: uint(envInt("LED_MCP_ORG_ID", 1)),
		RedisURL: env("LED_REDIS_URL", ""),
	}
	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" {
		return nil, fmt.Errorf("LED_DB_DRIVER must be sqlite or postgres, got %q", c.DBDriver)
	}
	if c.SecretKey == "" {
		return nil, fmt.Errorf("LED_SECRET_KEY is required (used for sessions and credential encryption)")
	}
	// Optional rotation fallbacks: previous master keys, newest-first.
	for _, k := range strings.Split(env("LED_SECRET_KEY_OLD", ""), ",") {
		if k = strings.TrimSpace(k); k != "" {
			c.OldSecretKeys = append(c.OldSecretKeys, k)
		}
	}

	// Secure cookies: auto-on when prod-looking, overridable by env.
	c.SecureCookies = strings.HasPrefix(strings.ToLower(c.BaseURL), "https://") || c.AdminHost != ""
	if v, ok := os.LookupEnv("LED_SECURE_COOKIES"); ok {
		c.SecureCookies = strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	}
	if c.AdminPassword == "" {
		return nil, fmt.Errorf("LED_ADMIN_PASSWORD is required")
	}
	// A weak secret key undermines both credential encryption and cookie
	// integrity. Hard-fail in production-looking setups; warn otherwise so the
	// documented local dev key (LED_SECRET_KEY=dev) keeps working.
	if len(c.SecretKey) < minSecretKeyLen {
		if c.SecureCookies {
			return nil, fmt.Errorf("LED_SECRET_KEY must be at least %d bytes in production", minSecretKeyLen)
		}
		log.Printf("WARNING: LED_SECRET_KEY is only %d bytes; use at least %d bytes (e.g. `openssl rand -hex 32`) before production", len(c.SecretKey), minSecretKeyLen)
	}
	return c, nil
}
