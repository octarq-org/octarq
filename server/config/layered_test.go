package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePrecedence(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "octarq.yaml")
	yamlBody := "listen: \":9090\"\ndb:\n  driver: sqlite\n  dsn: yaml.db\nlog:\n  level: debug\n"
	if err := os.WriteFile(yamlPath, []byte(yamlBody), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OCTARQ_LISTEN", ":7070")
	t.Setenv("OCTARQ_DB_DSN", "env.db")

	values, err := Resolve(LoadOptions{
		DotEnvPath: "",
		YAMLPath:   yamlPath,
		Overrides:  map[string]string{"OCTARQ_DB_DSN": "flag.db"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if values["OCTARQ_LISTEN"] != ":7070" {
		t.Errorf("env must beat YAML, got %q", values["OCTARQ_LISTEN"])
	}
	if values["OCTARQ_DB_DSN"] != "flag.db" {
		t.Errorf("override must beat env, got %q", values["OCTARQ_DB_DSN"])
	}
	if values["OCTARQ_LOG_LEVEL"] != "debug" {
		t.Errorf("YAML must supply unset keys, got %q", values["OCTARQ_LOG_LEVEL"])
	}
}

func TestResolveBadYAMLFailsClosed(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badPath, []byte(":\t: bad: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(LoadOptions{DotEnvPath: "", YAMLPath: badPath}); err == nil {
		t.Error("malformed YAML must fail")
	}
	if _, err := Resolve(LoadOptions{DotEnvPath: "", YAMLPath: filepath.Join(dir, "missing.yaml")}); err == nil {
		t.Error("missing YAML must fail")
	}
}

func TestLoadWithOptionsMatchesLoad(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "layered.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "layered-test-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "layered-test-admin-pass")

	fromEnv, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fromLayered, err := LoadWithOptions(LoadOptions{DotEnvPath: ""})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if fromEnv.Listen != fromLayered.Listen || fromEnv.DBDSN != fromLayered.DBDSN {
		t.Errorf("layered load diverged: %+v vs %+v", fromEnv, fromLayered)
	}
}

func TestLoadWithOptionsValidation(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "oracle")
	t.Setenv("OCTARQ_SECRET_KEY", "layered-test-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "layered-test-admin-pass")
	if _, err := LoadWithOptions(LoadOptions{DotEnvPath: ""}); err == nil {
		t.Error("bad driver must fail closed")
	}
}

func TestLoadWithOptionsDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	body := "OCTARQ_LISTEN=:7071\nOCTARQ_SECRET_KEY=dotenv-secret-12345678901234\nOCTARQ_ADMIN_PASSWORD=dotenv-admin-pass\n"
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("OCTARQ_LISTEN")
	t.Cleanup(func() { os.Unsetenv("OCTARQ_LISTEN") })
	cfg, err := LoadWithOptions(LoadOptions{DotEnvPath: envPath})
	if err != nil {
		t.Fatalf("dotenv load: %v", err)
	}
	if cfg.Listen != ":7071" {
		t.Errorf("listen = %q, want :7071", cfg.Listen)
	}
}

func TestBuildFromValuesBranches(t *testing.T) {
	base := map[string]string{
		"OCTARQ_DB_DRIVER":      "sqlite",
		"OCTARQ_DB_DSN":         filepath.Join(t.TempDir(), "branches.db"),
		"OCTARQ_SECRET_KEY":     "branch-test-secret-1234567890",
		"OCTARQ_ADMIN_PASSWORD": "branch-test-admin-pass",
	}
	if _, err := buildFromValues(base); err != nil {
		t.Fatalf("valid values: %v", err)
	}

	badLevel := map[string]string{}
	for k, v := range base {
		badLevel[k] = v
	}
	badLevel["OCTARQ_LOG_LEVEL"] = "verbose"
	if _, err := buildFromValues(badLevel); err == nil {
		t.Error("bad log level must fail")
	}

	provisioned := map[string]string{}
	for k, v := range base {
		provisioned[k] = v
	}
	provisioned["OCTARQ_DB_DRIVER"] = "postgres"
	provisioned["OCTARQ_SECRET_KEY"] = "short"
	if _, err := buildFromValues(provisioned); err == nil {
		t.Error("short secret on provisioned infra must fail")
	}

	missingSecret := map[string]string{}
	for k, v := range base {
		missingSecret[k] = v
	}
	delete(missingSecret, "OCTARQ_SECRET_KEY")
	missingSecret["OCTARQ_DB_DSN"] = filepath.Join(t.TempDir(), "auto.db")
	if _, err := buildFromValues(missingSecret); err != nil {
		t.Errorf("zero-config boot must auto-generate secrets, got: %v", err)
	}
}
