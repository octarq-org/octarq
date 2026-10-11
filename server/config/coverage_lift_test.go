package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoverageLift_EnvDefault(t *testing.T) {
	_ = os.Unsetenv("TEST_LIFT_NONEXISTENT_KEY")
	val := env("TEST_LIFT_NONEXISTENT_KEY", "default-val")
	if val != "default-val" {
		t.Fatalf("expected default-val, got %q", val)
	}
}

func TestCoverageLift_LoadDotEnvDirectoryError(t *testing.T) {
	// Opening a directory returns an error on os.Open
	err := loadDotEnv(".")
	if err == nil {
		t.Fatal("expected error when loading directory as dotenv, got nil")
	}
}

func TestCoverageLift_ProvisionedBecauseMySQL(t *testing.T) {
	c := &Config{DBDriver: "mysql"}
	if got := c.provisionedBecause(); got != "OCTARQ_DB_DRIVER=mysql" {
		t.Fatalf("expected OCTARQ_DB_DRIVER=mysql, got %q", got)
	}
}

func TestCoverageLift_ResolveAndLoadWithOptionsDotEnvError(t *testing.T) {
	opts := LoadOptions{DotEnvPath: "."}
	if _, err := Resolve(opts); err == nil {
		t.Fatal("expected error from Resolve with invalid dotenv, got nil")
	}
	if _, err := LoadWithOptions(opts); err == nil {
		t.Fatal("expected error from LoadWithOptions with invalid dotenv, got nil")
	}
}

func TestCoverageLift_BuildFromValuesValidation(t *testing.T) {
	// 1. Short secret key with MySQL provisioned infrastructure triggers line 166 and provisionedBecause
	valsShortSecretMySQL := map[string]string{
		"OCTARQ_DB_DRIVER":      "mysql",
		"OCTARQ_DB_DSN":         "mysql://localhost/octarq",
		"OCTARQ_SECRET_KEY":     "short",
		"OCTARQ_ADMIN_PASSWORD": "adminpassword123",
	}
	if _, err := buildFromValues(valsShortSecretMySQL); err == nil {
		t.Fatal("expected error for short secret key on provisioned mysql, got nil")
	}

	// 2. Failed auto secrets due to invalid directory triggers line 155-156
	valsBadDir := map[string]string{
		"OCTARQ_DB_DRIVER": "sqlite",
		"OCTARQ_DB_DSN":    "/dev/null/impossible/octarq.db",
	}
	if _, err := buildFromValues(valsBadDir); err == nil {
		t.Fatal("expected error for impossible sqlite state directory, got nil")
	}
}

func TestCoverageLift_LoadOrCreateErrors(t *testing.T) {
	// 1. MkdirAll error
	_, _, err := loadOrCreate("/dev/null/impossible", "key", func() (string, error) {
		return "secret", nil
	})
	if err == nil {
		t.Fatal("expected error from loadOrCreate when mkdir fails, got nil")
	}

	// 2. WriteFile error in read-only directory
	roDir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(roDir, 0o500); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(roDir, 0o700)
	})
	_, _, err = loadOrCreate(roDir, "key", func() (string, error) {
		return "secret", nil
	})
	if err == nil {
		t.Fatal("expected error from loadOrCreate when write fails, got nil")
	}
}
