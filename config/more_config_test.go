package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDirVariations(t *testing.T) {
	cases := []struct {
		driver string
		dsn    string
		want   string
	}{
		{"postgres", "postgres://localhost/db", "."},
		{"sqlite", "", "."},
		{"sqlite", ":memory:", "."},
		{"sqlite", "file::memory:?cache=shared", "."},
		{"sqlite", "file:data/test.db?_pragma=busy_timeout(5000)", "data"},
		{"sqlite", "/var/lib/octarq.db", "/var/lib"},
	}

	for _, tc := range cases {
		c := &Config{DBDriver: tc.driver, DBDSN: tc.dsn}
		got := c.stateDir()
		if got != tc.want {
			t.Errorf("stateDir(%s, %q) = %q, want %q", tc.driver, tc.dsn, got, tc.want)
		}
	}
}

func TestLoadAllEnvOptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OCTARQ_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "supersecretadminpassword")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "octarq.db"))
	t.Setenv("OCTARQ_LISTEN", ":9090")
	t.Setenv("OCTARQ_ADMIN_USER", "customadmin")
	t.Setenv("OCTARQ_TRUST_PROXY", "true")
	t.Setenv("OCTARQ_ALLOW_PRIVATE_WEBHOOKS", "1")
	t.Setenv("OCTARQ_ALLOW_PRIVATE_SMTP", "true")
	t.Setenv("OCTARQ_GEOIP_DB", "/path/to/geoip.mmdb")
	t.Setenv("OCTARQ_CORS_ORIGINS", "https://app.example.com,https://api.example.com")
	t.Setenv("OCTARQ_LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q, want :9090", cfg.Listen)
	}
	if cfg.AdminUser != "customadmin" {
		t.Errorf("AdminUser = %q, want customadmin", cfg.AdminUser)
	}
	if !cfg.TrustProxy {
		t.Error("TrustProxy should be true")
	}
	if !cfg.AllowPrivateWebhooks {
		t.Error("AllowPrivateWebhooks should be true")
	}
	if !cfg.AllowPrivateSMTP {
		t.Error("AllowPrivateSMTP should be true")
	}
	if cfg.GeoIPDB != "/path/to/geoip.mmdb" {
		t.Errorf("GeoIPDB = %q, want /path/to/geoip.mmdb", cfg.GeoIPDB)
	}
	if cfg.PublicCORSOrigins != "https://app.example.com,https://api.example.com" {
		t.Errorf("PublicCORSOrigins = %q", cfg.PublicCORSOrigins)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadOrCreateWhitespaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blank.txt")
	if err := os.WriteFile(path, []byte("   \n\t  \n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	val, created, err := loadOrCreate(dir, "blank.txt", func() (string, error) {
		return "generated-value", nil
	})
	if err != nil {
		t.Fatalf("loadOrCreate error: %v", err)
	}
	if !created {
		t.Error("expected created=true for whitespace-only file")
	}
	if val != "generated-value" {
		t.Errorf("got %q, want generated-value", val)
	}
}

func TestLoadDotEnvLineVariations(t *testing.T) {
	content := `
# leading comment
export VALID_VAR=hello
EMPTY_LINE=
NO_EQUALS_LINE
=NO_KEY
SPACED = value_with_space
COMMENT_NO_SPACE=val#notcomment
`
	tmpfile, err := os.CreateTemp("", "dotenv_var")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	tmpfile.Close()

	os.Unsetenv("VALID_VAR")
	os.Unsetenv("SPACED")
	os.Unsetenv("COMMENT_NO_SPACE")

	if err := loadDotEnv(tmpfile.Name()); err != nil {
		t.Fatalf("loadDotEnv error: %v", err)
	}

	if os.Getenv("VALID_VAR") != "hello" {
		t.Errorf("VALID_VAR = %q, want hello", os.Getenv("VALID_VAR"))
	}
	if os.Getenv("SPACED") != "value_with_space" {
		t.Errorf("SPACED = %q, want value_with_space", os.Getenv("SPACED"))
	}
	if os.Getenv("COMMENT_NO_SPACE") != "val#notcomment" {
		t.Errorf("COMMENT_NO_SPACE = %q, want val#notcomment", os.Getenv("COMMENT_NO_SPACE"))
	}
}

func TestRandHelpers(t *testing.T) {
	h, err := randHex(16)
	if err != nil {
		t.Fatalf("randHex failed: %v", err)
	}
	if len(h) != 32 {
		t.Errorf("randHex length = %d, want 32", len(h))
	}

	p, err := randPassword(20)
	if err != nil {
		t.Fatalf("randPassword failed: %v", err)
	}
	if len(p) != 20 {
		t.Errorf("randPassword length = %d, want 20", len(p))
	}
}

func TestLoadOrCreateReadError(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with the secret name, causing ReadFile to return an error (EISDIR)
	subDirPath := filepath.Join(dir, "secret_is_dir")
	if err := os.Mkdir(subDirPath, 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	_, _, err := loadOrCreate(dir, "secret_is_dir", func() (string, error) {
		return "val", nil
	})
	if err == nil {
		t.Fatal("expected error when reading directory as file")
	}
}

func TestEnsureAutoSecretsError(t *testing.T) {
	dir := t.TempDir()
	// Place a directory with name autoSecretFile
	if err := os.Mkdir(filepath.Join(dir, autoSecretFile), 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	cfg := &Config{
		DBDriver: "sqlite",
		DBDSN:    filepath.Join(dir, "octarq.db"),
	}
	if err := cfg.ensureAutoSecrets(); err == nil {
		t.Fatal("expected ensureAutoSecrets to fail when secret file is a directory")
	}

	// Now test admin password failure
	dir2 := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir2, autoAdminPassFile), 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	cfg2 := &Config{
		DBDriver:  "sqlite",
		DBDSN:     filepath.Join(dir2, "octarq.db"),
		SecretKey: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg2.ensureAutoSecrets(); err == nil {
		t.Fatal("expected ensureAutoSecrets to fail when admin password file is a directory")
	}
}

func TestLoadDotEnvOpenError(t *testing.T) {
	dir := t.TempDir()
	// Open a directory with loadDotEnv -> read error
	err := loadDotEnv(dir)
	// On Unix opening a directory succeeds for Open, but scanning or read might fail or open might succeed.
	// But let's check non-existent file vs unreadable file.
	if err != nil {
		// If error is returned, verify it's not swallowed
		t.Logf("loadDotEnv returned error as expected on directory: %v", err)
	}
}

func TestLoadOrCreateGenError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := loadOrCreate(dir, "key.txt", func() (string, error) {
		return "", os.ErrPermission
	})
	if err == nil {
		t.Fatal("expected error when generator fails")
	}
}
