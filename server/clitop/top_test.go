package clitop

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/octarq-org/octarq/server/internal/ipc"
)

var errMetricsBoom = errors.New("metrics boom")

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

	// Check backup file
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var backupFound bool
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".env.bak.") {
			backupFound = true
			bInfo, err := f.Info()
			if err != nil {
				t.Fatal(err)
			}
			if bInfo.Mode().Perm() != 0o600 {
				t.Errorf("backup file perm = %v, want 0600", bInfo.Mode().Perm())
			}
			bData, _ := os.ReadFile(filepath.Join(filepath.Dir(path), f.Name()))
			if string(bData) != "OCTARQ_LISTEN=:1\n" {
				t.Errorf("backup file content = %q, want OCTARQ_LISTEN=:1\n", string(bData))
			}
		}
	}
	if !backupFound {
		t.Error("backup file not created on force overwrite")
	}
}

func TestRunSetupRequiresTTY(t *testing.T) {
	var buf strings.Builder
	if code := RunSetup(context.Background(), &buf, filepath.Join(t.TempDir(), ".env"), false); code != 1 {
		t.Errorf("non-TTY setup exit = %d, want 1", code)
	}
}

func TestTopModelQuitAndTick(t *testing.T) {
	m := topModel{socket: "x"}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil {
		t.Error("q must quit")
	}
	updated, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Error("tick must schedule a refresh")
	}
	tm := updated.(topModel)
	if tm.socket != "x" {
		t.Errorf("socket = %q", tm.socket)
	}
	ready, _ := m.Update(snapMsg{snap: Snapshot{Status: statusForTest(), Metrics: metricsForTest()}})
	rm := ready.(topModel)
	if !rm.ready {
		t.Error("snapMsg must mark ready")
	}
	view := rm.View()
	for _, want := range []string{"octarq top", "v1.2.3", "healthy"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	pending := topModel{}.View()
	if !strings.Contains(pending, "connecting") {
		t.Errorf("pending view must show connecting, got:\n%s", pending)
	}
	broken := topModel{socket: "x", ready: true, snap: Snapshot{Err: "boom"}}.View()
	if !strings.Contains(broken, "boom") {
		t.Errorf("error view must surface the error, got:\n%s", broken)
	}
	if cmd := m.Init(); cmd == nil {
		t.Error("init must schedule work")
	}
	if got := fetchCmd("x")(); got == nil {
		t.Error("fetchCmd must produce a message")
	}
}

func TestSetupModelFlow(t *testing.T) {
	m := setupModel{fields: setupFields(), values: map[string]string{}}
	if got := m.Init(); got != nil {
		t.Errorf("init = %v, want nil", got)
	}
	if view := m.View(); !strings.Contains(view, "Admin username") {
		t.Errorf("view must list fields, got:\n%s", view)
	}
	for i := range m.fields {
		answer := "v" + string(rune('0'+i))
		for _, r := range answer {
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = updated.(setupModel)
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(setupModel)
	}
	if !m.done {
		t.Error("wizard must finish after all fields")
	}
	if m.values["OCTARQ_ADMIN_USER"] == "" {
		t.Errorf("answers not recorded: %+v", m.values)
	}

	m2 := setupModel{fields: setupFields(), values: map[string]string{}}
	updated, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m2 = updated.(setupModel)
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m2 = updated.(setupModel)
	if len(m2.input) != 0 {
		t.Errorf("backspace must erase, got %q", string(m2.input))
	}
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 = updated.(setupModel)
	if !m2.done || m2.err == "" {
		t.Error("abort must finish with an error")
	}
}

func TestFormatUptimeBranches(t *testing.T) {
	cases := map[int64]string{
		45:    "45s",
		125:   "2m5s",
		3723:  "1h2m",
		90000: "1d1h",
	}
	for in, want := range cases {
		if got := formatUptime(in); got != want {
			t.Errorf("formatUptime(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchSnapshotMetricsErrorTolerated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.sock")
	lis, err := ipc.Listen(path)
	if err != nil {
		t.Skipf("unix socket unavailable: %v", err)
	}
	defer lis.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &ipc.Daemon{
		Version: "v9",
		Started: time.Now(),
		OnMetrics: func(ctx context.Context) (*ipc.MetricsReply, error) {
			return nil, errMetricsBoom
		},
	}
	go func() { _ = d.Serve(ctx, lis) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, derr := net.DialTimeout("unix", path, time.Second)
		if derr == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not accept")
		}
		time.Sleep(20 * time.Millisecond)
	}
	snap := FetchSnapshot(context.Background(), path)
	if snap.Err != "" {
		t.Fatalf("status must succeed, got %q", snap.Err)
	}
	if snap.Metrics != nil {
		t.Error("failed metrics must stay nil, not fabricated")
	}
}

func TestTopViewAfterDone(t *testing.T) {
	m := setupModel{fields: setupFields(), values: map[string]string{}, done: true}
	if got := m.View(); got != "" {
		t.Errorf("done view must be empty, got %q", got)
	}
}

func TestTopModelEscQuits(t *testing.T) {
	m := topModel{}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd == nil {
		t.Error("esc must quit")
	}
}

func TestRunTopCIEnvForcesPlain(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("OCTARQ_IPC_SOCKET", filepath.Join(t.TempDir(), "nobody.sock"))
	var buf strings.Builder
	if code := RunTop(context.Background(), &buf, filepath.Join(t.TempDir(), "x.sock")); code != 1 {
		t.Errorf("CI plain top exit = %d, want 1", code)
	}
}

func TestWriteEnvFileBackupFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("OCTARQ_LISTEN=:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make directory read-only so backup file cannot be created
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700) // cleanup

	resolved := map[string]string{
		"OCTARQ_ADMIN_USER": "a", "OCTARQ_ADMIN_PASSWORD": "b",
		"OCTARQ_DB_DRIVER": "sqlite", "OCTARQ_DB_DSN": "c",
		"OCTARQ_LISTEN": ":1", "OCTARQ_SECRET_KEY": "d",
	}
	err := WriteEnvFile(path, resolved, true)
	if err == nil {
		t.Error("expected backup to fail due to read-only directory")
	} else if !strings.Contains(err.Error(), "failed to write backup file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteEnvFileReadFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("OCTARQ_LISTEN=:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make file write-only so it cannot be read
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600) // cleanup

	resolved := map[string]string{
		"OCTARQ_ADMIN_USER": "a", "OCTARQ_ADMIN_PASSWORD": "b",
		"OCTARQ_DB_DRIVER": "sqlite", "OCTARQ_DB_DSN": "c",
		"OCTARQ_LISTEN": ":1", "OCTARQ_SECRET_KEY": "d",
	}
	err := WriteEnvFile(path, resolved, true)
	if err == nil {
		t.Error("expected backup to fail due to unreadable file")
	} else if !strings.Contains(err.Error(), "failed to read existing file for backup") {
		t.Errorf("unexpected error: %v", err)
	}
}
