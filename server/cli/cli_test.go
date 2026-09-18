package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/ipc"
	"github.com/spf13/cobra"
)

func testDeps() Deps {
	return Deps{
		Name:         "octarq",
		PrintVersion: func(w io.Writer) { _, _ = w.Write([]byte("octarq dev\n")) },
		Boot:         func(ctx context.Context) int { return 0 },
		MCP:          func(ctx context.Context) int { return 0 },
		OpenAPI:      func(w io.Writer) int { _, _ = w.Write([]byte(`{"openapi":"x"}`)); return 0 },
		Backup:       func(args []string) int { return 0 },
		Restore:      func(args []string) int { return 0 },
		Plugin:       func(args []string) int { return 0 },
	}
}

func TestExecuteVersionPrefix(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"--version"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("version exit = %d, want 0", code)
	}
	if !strings.HasPrefix(out.String(), "octarq ") {
		t.Errorf("version output = %q, want octarq prefix", out.String())
	}
	if errb.Len() != 0 {
		t.Errorf("version wrote to stderr: %q", errb.String())
	}
}

func TestExecuteRootBootsByDefault(t *testing.T) {
	booted := false
	d := testDeps()
	d.Boot = func(ctx context.Context) int { booted = true; return 0 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), nil, &out, &errb, d); code != 0 {
		t.Fatalf("default exit = %d, want 0", code)
	}
	if !booted {
		t.Error("default command did not boot the server")
	}
}

func TestExecuteHelpListsSubcommands(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"--help"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("help exit = %d, want 0", code)
	}
	for _, want := range []string{"server", "mcp", "openapi", "backup", "restore", "plugin", "service", "completion", "version", "status", "reload", "top", "setup"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestExecuteServerHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"server", "--help"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("server help exit = %d, want 0", code)
	}
	for _, want := range []string{"--port", "--host", "--config"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("server help missing flag %q", want)
		}
	}
}

func TestExecuteUnknownCommandFails(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"frobnicate"}, &out, &errb, testDeps()); code == 0 {
		t.Error("unknown command must exit non-zero")
	}
}

func TestExecuteBackupFlagErrorMapsToBodyCode(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"backup", "--nope"}, &out, &errb, testDeps()); code != 1 {
		t.Errorf("backup --nope exit = %d, want 1", code)
	}
}

func TestExecuteBackupPassesOutFlag(t *testing.T) {
	var got []string
	d := testDeps()
	d.Backup = func(args []string) int { got = args; return 0 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"backup", "--out", "a.db"}, &out, &errb, d); code != 0 {
		t.Fatalf("backup exit = %d, want 0", code)
	}
	if len(got) != 2 || got[0] != "--out" || got[1] != "a.db" {
		t.Errorf("backup args = %v, want [--out a.db]", got)
	}
}

func TestExecuteRestoreBuildsArgs(t *testing.T) {
	var got []string
	d := testDeps()
	d.Restore = func(args []string) int { got = args; return 7 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"restore", "--in", "a.db", "--yes"}, &out, &errb, d); code != 7 {
		t.Fatalf("restore exit = %d, want body code 7", code)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--in") || !strings.Contains(joined, "a.db") || !strings.Contains(joined, "--yes") {
		t.Errorf("restore args = %v, want --in a.db --yes", got)
	}
}

func TestExecutePluginDelegatesRawArgs(t *testing.T) {
	var got []string
	d := testDeps()
	d.Plugin = func(args []string) int { got = args; return 2 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"plugin", "unknown"}, &out, &errb, d); code != 2 {
		t.Fatalf("plugin unknown exit = %d, want 2", code)
	}
	if len(got) != 1 || got[0] != "unknown" {
		t.Errorf("plugin args = %v, want [unknown]", got)
	}
}

func TestExecuteServiceUnknownAction(t *testing.T) {
	d := testDeps()
	d.ServiceName = "octarq-test"
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"service", "explode"}, &out, &errb, d); code != 1 {
		t.Errorf("service unknown action exit = %d, want 1", code)
	}
}

func TestExecuteServiceDisabledWithoutName(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"service", "status"}, &out, &errb, testDeps()); code == 0 {
		t.Error("service without a configured name must not succeed")
	}
}

func TestExecuteCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		var out, errb bytes.Buffer
		if code := Execute(context.Background(), []string{"completion", shell}, &out, &errb, testDeps()); code != 0 {
			t.Errorf("completion %s exit = %d, want 0: %s", shell, code, errb.String())
		}
		if out.Len() == 0 {
			t.Errorf("completion %s produced no output", shell)
		}
	}
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"completion", "tcsh"}, &out, &errb, testDeps()); code != 1 {
		t.Errorf("completion tcsh exit = %d, want 1", code)
	}
}

func TestExecuteVersionSubcommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"version"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("version exit = %d, want 0", code)
	}
	if !strings.HasPrefix(out.String(), "octarq ") {
		t.Errorf("version output = %q, want octarq prefix", out.String())
	}
}

func TestExecuteExtraMounts(t *testing.T) {
	ran := false
	d := testDeps()
	d.Extra = func(root *cobra.Command) {
		root.AddCommand(&cobra.Command{
			Use: "seed-catalog",
			RunE: func(cmd *cobra.Command, _ []string) error {
				ran = true
				return nil
			},
		})
	}
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"seed-catalog"}, &out, &errb, d); code != 0 {
		t.Fatalf("extra command exit = %d, want 0", code)
	}
	if !ran {
		t.Error("mounted extra command did not run")
	}
}

func TestExecuteStatusUnreachable(t *testing.T) {
	t.Setenv("OCTARQ_IPC_SOCKET", filepath.Join(t.TempDir(), "nobody.sock"))
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"status"}, &out, &errb, testDeps()); code != 1 {
		t.Errorf("status exit = %d, want 1", code)
	}
}

func TestExecuteReloadUnreachable(t *testing.T) {
	t.Setenv("OCTARQ_IPC_SOCKET", filepath.Join(t.TempDir(), "nobody.sock"))
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"reload"}, &out, &errb, testDeps()); code != 1 {
		t.Errorf("reload exit = %d, want 1", code)
	}
}

func TestExecuteTopPlainUnreachable(t *testing.T) {
	t.Setenv("OCTARQ_IPC_SOCKET", filepath.Join(t.TempDir(), "nobody.sock"))
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"top"}, &out, &errb, testDeps()); code != 1 {
		t.Errorf("top exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "daemon unreachable") {
		t.Errorf("top output must report unreachable, got %q", out.String())
	}
}

func TestExecuteSetupRequiresTTY(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"setup"}, &out, &errb, testDeps()); code != 1 {
		t.Errorf("setup exit = %d, want 1", code)
	}
}

func TestExecuteStatusAndReloadAgainstLiveDaemon(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "run.sock")
	lis, err := ipc.Listen(socket)
	if err != nil {
		t.Skipf("unix socket unavailable: %v", err)
	}
	defer lis.Close()
	daemon := &ipc.Daemon{
		Version: "test-ipc",
		Started: time.Now(),
		OnReload: func(ctx context.Context) ([]string, error) {
			return []string{"log.level"}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = daemon.Serve(ctx, lis) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("unix", socket, time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not accept")
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Setenv("OCTARQ_IPC_SOCKET", socket)
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"status"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("status exit = %d, want 0: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "test-ipc") {
		t.Errorf("status output missing version, got %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := Execute(context.Background(), []string{"reload"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("reload exit = %d, want 0: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "log.level") {
		t.Errorf("reload output missing applied key, got %q", out.String())
	}
}

func TestProgramStartStop(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	p := &program{boot: func(ctx context.Context) int {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return 0
	}}
	if err := p.Start(nil); err != nil {
		t.Fatalf("program start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("boot did not start")
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("program stop: %v", err)
	}
	if err := (&program{}).Stop(nil); err != nil {
		t.Fatalf("stop without start: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[int64]string{
		5:     "5s",
		75:    "1m15s",
		3723:  "1h2m",
		90000: "1d1h",
	}
	for in, want := range cases {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{
		512:     "512B",
		2048:    "2.0KB",
		8 << 20: "8.0MB",
		5 << 30: "5.0GB",
	}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestExecuteServerDelegatesToBoot(t *testing.T) {
	booted := false
	d := testDeps()
	d.Boot = func(ctx context.Context) int { booted = true; return 0 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"server", "--port", "0"}, &out, &errb, d); code != 0 {
		t.Fatalf("server exit = %d, want 0: %s", code, errb.String())
	}
	if !booted {
		t.Error("server command did not boot")
	}
}

func TestExecuteMCPDelegates(t *testing.T) {
	ran := false
	d := testDeps()
	d.MCP = func(ctx context.Context) int { ran = true; return 3 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"mcp"}, &out, &errb, d); code != 3 {
		t.Fatalf("mcp exit = %d, want body code 3", code)
	}
	if !ran {
		t.Error("mcp body did not run")
	}
}

func TestExecuteOpenAPIDelegates(t *testing.T) {
	d := testDeps()
	d.OpenAPI = func(w io.Writer) int { _, _ = w.Write([]byte("spec")); return 0 }
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"openapi"}, &out, &errb, d); code != 0 {
		t.Fatalf("openapi exit = %d, want 0", code)
	}
	if out.String() != "spec" {
		t.Errorf("openapi output = %q", out.String())
	}
}

func TestExecutePluginHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"plugin", "--help"}, &out, &errb, testDeps()); code != 0 {
		t.Fatalf("plugin help exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "plugin new") {
		t.Errorf("plugin help = %q", out.String())
	}
}
