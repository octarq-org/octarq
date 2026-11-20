package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const MinSecretKeyLen = 16

const MinAdminPasswordLen = 12

var DefaultAppName = "octarq"

type Config struct {
	Listen string

	WebhookListen string

	DBDriver string
	DBDSN    string

	SecretKey string

	AdminUser     string
	AdminPassword string

	StorageDir string

	TrustProxy bool

	AllowPrivateWebhooks bool

	AllowPrivateSMTP bool

	GeoIPDB string

	RedisURL string

	PublicCORSOrigins string

	LogLevel string

	LoginMaxFails int

	LoginAccountMaxFails int

	LoginWindow time.Duration

	DevWebProxy string

	OTelEnabled         bool
	OTelEndpoint        string
	OTelServiceName     string
	OTelTracesExporter  string
	OTelMetricsExporter string
	OTelSampler         string
	OTelSampleRatio     float64
	OTelInsecure        bool
	OTelHeaders         string
	MetricsEnabled      bool
	MetricsToken        string

	BackupS3Endpoint    string
	BackupS3Bucket      string
	BackupS3AccessKey   string
	BackupS3SecretKey   string
	BackupS3Region      string
	BackupRetentionDays int
	BackupCron          string
}

const (
	DefaultLoginMaxFails        = 5
	DefaultLoginAccountMaxFails = 10
	DefaultLoginWindow          = 15 * time.Minute
)

func (c *Config) LoginBudgets() (maxFails, accountMaxFails int, window time.Duration) {
	maxFails, accountMaxFails, window = c.LoginMaxFails, c.LoginAccountMaxFails, c.LoginWindow
	if maxFails <= 0 {
		maxFails = DefaultLoginMaxFails
	}
	if accountMaxFails <= 0 {
		accountMaxFails = DefaultLoginAccountMaxFails
	}
	if window <= 0 {
		window = DefaultLoginWindow
	}
	return maxFails, accountMaxFails, window
}

func (c *Config) BackupS3Configured() bool {
	return strings.TrimSpace(c.BackupS3Bucket) != "" &&
		strings.TrimSpace(c.BackupS3AccessKey) != "" &&
		strings.TrimSpace(c.BackupS3SecretKey) != ""
}

func ValidateWebhookListen(listen, webhookListen string) error {
	webhookListen = strings.TrimSpace(webhookListen)
	listen = strings.TrimSpace(listen)
	if webhookListen == "" {
		return nil
	}
	if webhookListen == listen {
		return fmt.Errorf("webhook listen address %q cannot be identical to main listen address %q", webhookListen, listen)
	}
	_, p2, err2 := net.SplitHostPort(webhookListen)
	if err2 != nil {
		return fmt.Errorf("invalid webhook listen address %q: %w", webhookListen, err2)
	}
	if port, err := strconv.Atoi(p2); err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("invalid webhook listen port %q: must be between 0 and 65535", p2)
	}
	_, p1, err1 := net.SplitHostPort(listen)
	if err1 == nil && p1 == p2 && p1 != "0" {
		return fmt.Errorf("webhook listen port %s conflicts with main listen port %s", p2, p1)
	}
	return nil
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

var slogsByLevel = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

func normalizeLogLevel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "info"
	}
	return v
}

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

func LoadDotEnv(path string) error { return loadDotEnv(path) }

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

func Load() (*Config, error) {
	return LoadWithOptions(LoadOptions{DotEnvPath: ".env"})
}

func (c *Config) Provisioned() bool {
	return c.DBDriver == "postgres" || c.DBDriver == "mysql" || strings.TrimSpace(c.RedisURL) != ""
}

func (c *Config) provisionedBecause() string {
	if c.DBDriver == "postgres" {
		return "OCTARQ_DB_DRIVER=postgres"
	}
	if c.DBDriver == "mysql" {
		return "OCTARQ_DB_DRIVER=mysql"
	}
	return "OCTARQ_REDIS_URL is set"
}
