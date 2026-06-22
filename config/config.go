// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
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

	// InboundToken guards POST /api/email/inbound (the Cloudflare Email Worker
	// webhook). Requests must present it via the X-Led-Token header.
	InboundToken string

	// CatchAll, when true, auto-creates a mailbox the first time mail arrives
	// for an unknown address on a managed domain.
	CatchAll bool

	GeoIPDB string // optional path to a MaxMind GeoLite2-City.mmdb

	// Outbound SMTP relay (sending). Optional.
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	BaseURL string // public base URL, used to build short link / QR URLs
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// Load reads configuration from the environment, applying sane defaults.
func Load() (*Config, error) {
	c := &Config{
		Listen:        env("LED_LISTEN", ":8080"),
		AdminHost:     env("LED_ADMIN_HOST", ""),
		DBDriver:      env("LED_DB_DRIVER", "sqlite"),
		DBDSN:         env("LED_DB_DSN", "led.db"),
		SecretKey:     env("LED_SECRET_KEY", ""),
		AdminUser:     env("LED_ADMIN_USER", "admin"),
		AdminPassword: env("LED_ADMIN_PASSWORD", ""),
		InboundToken:  env("LED_INBOUND_TOKEN", ""),
		CatchAll:      env("LED_CATCH_ALL", "true") == "true",
		GeoIPDB:       env("LED_GEOIP_DB", ""),
		SMTPHost:      env("LED_SMTP_HOST", ""),
		SMTPPort:      env("LED_SMTP_PORT", "587"),
		SMTPUser:      env("LED_SMTP_USER", ""),
		SMTPPass:      env("LED_SMTP_PASS", ""),
		SMTPFrom:      env("LED_SMTP_FROM", ""),
		BaseURL:       strings.TrimRight(env("LED_BASE_URL", ""), "/"),
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
