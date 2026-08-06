package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestLoadAutoGeneratesSecrets checks the zero-config path: with neither the
// secret key nor the admin password set, Load generates and persists both next
// to the SQLite database instead of failing.
func TestLoadAutoGeneratesSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OCTARQ_SECRET_KEY", "")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "octarq.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with no secrets: %v", err)
	}
	if len(cfg.SecretKey) < MinSecretKeyLen {
		t.Errorf("auto SecretKey too short: %d bytes", len(cfg.SecretKey))
	}
	if cfg.AdminPassword == "" {
		t.Error("auto AdminPassword is empty")
	}
	for _, name := range []string{autoSecretFile, autoAdminPassFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s persisted: %v", name, err)
		}
	}
}

// TestLoadAutoSecretsStableAcrossRestart checks that a second Load reuses the
// persisted secret and password (the KEK and login must not rotate on reboot).
func TestLoadAutoSecretsStableAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OCTARQ_SECRET_KEY", "")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "octarq.db"))

	first, err := Load()
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	second, err := Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if first.SecretKey != second.SecretKey {
		t.Error("SecretKey rotated across restart")
	}
	if first.AdminPassword != second.AdminPassword {
		t.Error("AdminPassword rotated across restart")
	}
}

// TestLoadEnvSecretsWin checks env-supplied values are used verbatim and never
// written to disk.
func TestLoadEnvSecretsWin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OCTARQ_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "hunter2")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "octarq.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminPassword != "hunter2" {
		t.Errorf("AdminPassword = %q, want env value", cfg.AdminPassword)
	}
	if _, err := os.Stat(filepath.Join(dir, autoSecretFile)); !os.IsNotExist(err) {
		t.Error("secret file should not be written when env supplies the key")
	}
}

func TestLoadRejectsBadDriver(t *testing.T) {
	t.Setenv("OCTARQ_SECRET_KEY", "s")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "pw")
	t.Setenv("OCTARQ_DB_DRIVER", "mysql")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("OCTARQ_SECRET_KEY", "s")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "pw")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("Listen default = %q want :8080", cfg.Listen)
	}
	if cfg.AdminUser != "admin" {
		t.Errorf("AdminUser default = %q want admin", cfg.AdminUser)
	}
	if cfg.DBDSN != "octarq.db" {
		t.Errorf("DBDSN default = %q want octarq.db", cfg.DBDSN)
	}
}

// TestLoadBaseURLValidation covers every combination of the three interesting
// OCTARQ_BASE_URL inputs (absent / malformed / absolute) against the two
// severity modes Load distinguishes.
//
// The mode is pinned with OCTARQ_SECURE_COOKIES because that is the real
// production signal (Config.IsProduction) — the same one that decides the
// secret-key floor. Setting it explicitly also keeps the cases independent: an
// https:// base URL would otherwise flip the mode to production by itself.
//
// The empty-in-production case is the one that matters: a self-hosted instance
// with no base URL boots fine and mails out unopenable relative links.
func TestLoadBaseURLValidation(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		prod    bool
		wantErr bool
	}{
		{name: "absent in production is fatal", baseURL: "", prod: true, wantErr: true},
		{name: "absent in development is tolerated", baseURL: "", prod: false, wantErr: false},
		{name: "malformed in production is fatal", baseURL: "app.example.com", prod: true, wantErr: true},
		{name: "malformed in development is fatal too", baseURL: "app.example.com", prod: false, wantErr: true},
		{name: "absolute in production is accepted", baseURL: "https://app.example.com", prod: true, wantErr: false},
		{name: "absolute in development is accepted", baseURL: "http://localhost:8080", prod: false, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// Long enough to clear the secret-key floor, so a production case
			// can only fail for the reason under test.
			t.Setenv("OCTARQ_SECRET_KEY", "0123456789abcdef0123456789abcdef")
			t.Setenv("OCTARQ_ADMIN_PASSWORD", "pw")
			t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
			t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "octarq.db"))
			t.Setenv("OCTARQ_BASE_URL", tc.baseURL)
			t.Setenv("OCTARQ_SECURE_COOKIES", strconv.FormatBool(tc.prod))

			cfg, err := Load()
			switch {
			case tc.wantErr && err == nil:
				t.Fatalf("Load(base=%q, production=%v) succeeded; expected a refusal to start", tc.baseURL, tc.prod)
			case !tc.wantErr && err != nil:
				t.Fatalf("Load(base=%q, production=%v) failed: %v", tc.baseURL, tc.prod, err)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "OCTARQ_BASE_URL") {
					t.Errorf("error does not name the offending variable: %v", err)
				}
				return
			}
			if cfg.IsProduction() != tc.prod {
				t.Fatalf("IsProduction() = %v, want %v — the production signal under test was not applied", cfg.IsProduction(), tc.prod)
			}
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	content := `
# A comment line
export KEY1=value1
KEY2="value2"
KEY3='value3'
KEY4=value4 # another comment
`
	tmpfile, err := os.CreateTemp("", "dotenv")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	tmpfile.Close()

	os.Unsetenv("KEY1")
	os.Unsetenv("KEY2")
	os.Unsetenv("KEY3")
	os.Unsetenv("KEY4")

	if err := loadDotEnv(tmpfile.Name()); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	if os.Getenv("KEY1") != "value1" {
		t.Errorf("KEY1 = %q, want value1", os.Getenv("KEY1"))
	}
	if os.Getenv("KEY2") != "value2" {
		t.Errorf("KEY2 = %q, want value2", os.Getenv("KEY2"))
	}
	if os.Getenv("KEY3") != "value3" {
		t.Errorf("KEY3 = %q, want value3", os.Getenv("KEY3"))
	}
	if os.Getenv("KEY4") != "value4" {
		t.Errorf("KEY4 = %q, want value4", os.Getenv("KEY4"))
	}

	if err := loadDotEnv("nonexistent_dotenv_file"); err != nil {
		t.Errorf("loadDotEnv on missing file returned error: %v", err)
	}
}
