// Package config loads runtime configuration from environment variables.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for led.
type Config struct {
	Listen    string // e.g. ":8080"
	AdminHost string // host that serves the dashboard; empty = serve dashboard on any non-link host

	DBDriver string // "sqlite" | "postgres"
	DBDSN    string // sqlite: file path; postgres: connection string

	// SecretKey seeds both the session-cookie HMAC and the AES-GCM used to
	// encrypt provider credentials at rest. Must be stable across restarts.
	SecretKey string

	AdminUser     string
	AdminPassword string

	GeoIPDB string // optional path to a MaxMind GeoLite2-City.mmdb

	// BaseURL is the public-facing URL used to build OAuth callback URIs,
	// e.g. "https://app.example.com". Leave empty to disable OAuth login.
	BaseURL string

	// MCPOrgID scopes the `led mcp` server's convenience tools to one tenant's
	// data. Defaults to 1 (the single-operator self-hosted case). On a
	// multi-tenant deployment, run one MCP process per tenant with this set.
	MCPOrgID uint
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

		GeoIPDB:  env("LED_GEOIP_DB", ""),
		BaseURL:  env("LED_BASE_URL", ""),
		MCPOrgID: uint(envInt("LED_MCP_ORG_ID", 1)),
	}
	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" {
		return nil, fmt.Errorf("LED_DB_DRIVER must be sqlite or postgres, got %q", c.DBDriver)
	}
	if c.SecretKey == "" {
		return nil, fmt.Errorf("LED_SECRET_KEY is required (used for sessions and credential encryption)")
	}
	if c.AdminPassword == "" {
		return nil, fmt.Errorf("LED_ADMIN_PASSWORD is required")
	}
	return c, nil
}
