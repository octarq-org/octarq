package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/ipc"
)

func TestCoverageLiftExitErrorAndDefaultName(t *testing.T) {
	// 1. exitError Error() string
	ee := &exitError{code: 123}
	if ee.Error() != "exit 123" {
		t.Errorf("expected 'exit 123', got %q", ee.Error())
	}

	// 2. Default binary name in NewRootCmd
	root := NewRootCmd(Deps{})
	if root.Name() != "octarq" {
		t.Errorf("expected default name 'octarq', got %q", root.Name())
	}
}

func TestCoverageLiftUnsupportedCommands(t *testing.T) {
	ctx := context.Background()
	d := Deps{Name: "customapp"}

	commands := []struct {
		args []string
		want string
	}{
		{nil, "server is not supported by customapp"},
		{[]string{"server"}, "server is not supported by customapp"},
		{[]string{"mcp"}, "mcp is not supported by customapp"},
		{[]string{"openapi"}, "openapi is not supported by customapp"},
		{[]string{"backup"}, "backup is not supported by customapp"},
		{[]string{"restore"}, "restore is not supported by customapp"},
		{[]string{"plugin"}, "plugin is not supported by customapp"},
	}

	for _, tc := range commands {
		var out, errb bytes.Buffer
		code := Execute(ctx, tc.args, &out, &errb, d)
		if code == 0 {
			t.Errorf("args %v: expected failure, got exit code 0", tc.args)
		}
		if !strings.Contains(errb.String(), tc.want) {
			t.Errorf("args %v: expected error containing %q, got %q", tc.args, tc.want, errb.String())
		}
	}
}

func TestCoverageLiftServerFlagsError(t *testing.T) {
	ctx := context.Background()
	d := testDeps()
	d.ApplyServerFlags = func(port, host, configPath string) error {
		return errors.New("bad configuration parameter")
	}

	var out, errb bytes.Buffer
	code := Execute(ctx, []string{"server", "--port", "8080"}, &out, &errb, d)
	if code == 0 {
		t.Fatalf("expected non-zero exit code on ApplyServerFlags failure")
	}
	if !strings.Contains(errb.String(), "bad configuration parameter") {
		t.Errorf("expected error message to contain 'bad configuration parameter', got %q", errb.String())
	}
}

func TestCoverageLiftIPCStatusAndReload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, err := os.MkdirTemp("", "cl")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, "s.sock")
	t.Setenv("OCTARQ_IPC_SOCKET", sockPath)

	var reloadMu sync.Mutex
	appliedKeys := []string{}

	daemon := &ipc.Daemon{
		Version: "2.5.0-lift",
		Started: time.Now().Add(-10 * time.Minute),
		OnReload: func(ctx context.Context) ([]string, error) {
			reloadMu.Lock()
			defer reloadMu.Unlock()
			return appliedKeys, nil
		},
	}

	lis, err := ipc.Listen(sockPath)
	if err != nil {
		t.Fatalf("listen ipc: %v", err)
	}
	defer lis.Close()

	srv := &http.Server{Handler: daemon.Handler()}
	go func() {
		_ = srv.Serve(lis)
	}()
	defer srv.Shutdown(context.Background())

	// 1. status command
	var out, errb bytes.Buffer
	code := Execute(ctx, []string{"status"}, &out, &errb, testDeps())
	if code != 0 {
		t.Fatalf("status failed: exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "version: 2.5.0-lift") || !strings.Contains(out.String(), "goroutines:") {
		t.Errorf("unexpected status output: %q", out.String())
	}

	// 2. reload command with no keys changed
	out.Reset()
	errb.Reset()
	code = Execute(ctx, []string{"reload"}, &out, &errb, testDeps())
	if code != 0 {
		t.Fatalf("reload failed: exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "reloaded (no keys changed)") {
		t.Errorf("unexpected reload output: %q", out.String())
	}

	// 3. reload command with applied keys
	reloadMu.Lock()
	appliedKeys = []string{"listen", "rate_limits"}
	reloadMu.Unlock()

	out.Reset()
	errb.Reset()
	code = Execute(ctx, []string{"reload"}, &out, &errb, testDeps())
	if code != 0 {
		t.Fatalf("reload with applied keys failed: exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "reloaded: listen, rate_limits") {
		t.Errorf("unexpected reload output with keys: %q", out.String())
	}
}
