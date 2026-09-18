package clitop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/ipc"
)

func statusForTest() *ipc.StatusReply {
	return &ipc.StatusReply{Version: "v1.2.3", UptimeSecs: 3723, PID: 4242, Goroutines: 17, MemAlloc: 8 << 20}
}

func metricsForTest() *ipc.MetricsReply {
	return &ipc.MetricsReply{
		Overall:   "healthy",
		Providers: []ipc.ProviderMetric{{Name: "db", Status: "healthy"}},
	}
}

func TestRenderPlainUnreachable(t *testing.T) {
	out := RenderPlain(Snapshot{Err: "dial unix: no such file"})
	if !strings.Contains(out, "daemon unreachable") {
		t.Errorf("plain snapshot must say unreachable, got:\n%s", out)
	}
}

func TestRenderPlainVitals(t *testing.T) {
	out := RenderPlain(Snapshot{
		Status:  statusForTest(),
		Metrics: metricsForTest(),
	})
	for _, want := range []string{"v1.2.3", "goroutines", "mem", "overall: healthy", "db", "healthy"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain snapshot missing %q:\n%s", want, out)
		}
	}
}

func TestRunTopPlainWithoutTTY(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	if code := RunTop(context.Background(), &buf, filepath.Join(dir, "nobody.sock")); code != 1 {
		t.Errorf("unreachable daemon exit = %d, want 1", code)
	}
	if !strings.Contains(buf.String(), "daemon unreachable") {
		t.Errorf("plain fallback must report unreachable, got:\n%s", buf.String())
	}
}

func TestValidateSetup(t *testing.T) {
	resolved, err := ValidateSetup(map[string]string{
		"OCTARQ_ADMIN_USER":     "ops",
		"OCTARQ_ADMIN_PASSWORD": "long-admin-password",
		"OCTARQ_DB_DRIVER":      "sqlite",
		"OCTARQ_DB_DSN":         filepath.Join(t.TempDir(), "setup.db"),
		"OCTARQ_LISTEN":         ":8080",
		"OCTARQ_SECRET_KEY":     "",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if resolved["OCTARQ_ADMIN_USER"] != "ops" {
		t.Errorf("admin user = %q", resolved["OCTARQ_ADMIN_USER"])
	}
	if len(resolved["OCTARQ_SECRET_KEY"]) != 64 {
		t.Errorf("generated secret len = %d, want 64", len(resolved["OCTARQ_SECRET_KEY"]))
	}

	if _, err := ValidateSetup(map[string]string{
		"OCTARQ_ADMIN_PASSWORD": "x",
		"OCTARQ_DB_DRIVER":      "oracle",
		"OCTARQ_SECRET_KEY":     "long-enough-secret-key!!",
	}); err == nil {
		t.Error("bad driver must fail validation")
	}

	if _, err := ValidateSetup(map[string]string{
		"OCTARQ_ADMIN_PASSWORD": "",
		"OCTARQ_DB_DRIVER":      "sqlite",
		"OCTARQ_DB_DSN":         filepath.Join(t.TempDir(), "s.db"),
		"OCTARQ_SECRET_KEY":     "long-enough-secret-key!!",
	}); err != nil {
		t.Errorf("empty admin password auto-generates via zero-config boot, got: %v", err)
	}
}

func TestWriteEnvFileRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OCTARQ_LISTEN=:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := map[string]string{
		"OCTARQ_ADMIN_USER": "a", "OCTARQ_ADMIN_PASSWORD": "b",
		"OCTARQ_DB_DRIVER": "sqlite", "OCTARQ_DB_DSN": "c",
		"OCTARQ_LISTEN": ":1", "OCTARQ_SECRET_KEY": "d",
	}
	if err := WriteEnvFile(path, resolved, false); err == nil {
		t.Error("overwrite without force must fail")
	}
	if err := WriteEnvFile(path, resolved, true); err != nil {
		t.Errorf("overwrite with force: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("env perm = %o, want 600", info.Mode().Perm())
	}
}

func TestRunSetupRequiresTTY(t *testing.T) {
	var buf strings.Builder
	if code := RunSetup(context.Background(), &buf, filepath.Join(t.TempDir(), ".env"), false); code != 1 {
		t.Errorf("non-TTY setup exit = %d, want 1", code)
	}
}
