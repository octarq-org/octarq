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
