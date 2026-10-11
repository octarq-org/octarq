package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kardianos/service"
)

var errServiceBoom = errors.New("service boom")

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

func TestExecuteServiceStatusReportsHonestly(t *testing.T) {
	d := testDeps()
	d.ServiceName = "octarq-test-nonexistent-xyz"
	var out, errb bytes.Buffer
	code := Execute(context.Background(), []string{"service", "status"}, &out, &errb, d)
	combined := out.String() + errb.String()
	if code == 0 {
		for _, want := range []string{"running", "stopped", "unknown"} {
			if strings.Contains(combined, want) {
				return
			}
		}
		t.Errorf("status exit 0 must name a state, got %q", combined)
	} else if code != 1 {
		t.Errorf("status exit = %d, want 0 or 1", code)
	}
}

type fakeServiceBackend struct {
	status service.Status
	err    error
}

func (f *fakeServiceBackend) Install() error   { return f.err }
func (f *fakeServiceBackend) Uninstall() error { return f.err }
func (f *fakeServiceBackend) Start() error     { return f.err }
func (f *fakeServiceBackend) Stop() error      { return f.err }
func (f *fakeServiceBackend) Restart() error   { return f.err }
func (f *fakeServiceBackend) Status() (service.Status, error) {
	return f.status, f.err
}

func TestRunServiceActionAllBranches(t *testing.T) {
	okBackend := &fakeServiceBackend{status: service.StatusRunning}
	cases := map[string]string{
		"install":   "service installed",
		"uninstall": "service uninstalled",
		"start":     "service started",
		"stop":      "service stopped",
		"restart":   "service restarted",
		"status":    "running",
	}
	for action, want := range cases {
		var out bytes.Buffer
		if err := runServiceAction(&out, okBackend, action); err != nil {
			t.Errorf("%s: %v", action, err)
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s output = %q, want %q", action, out.String(), want)
		}
	}

	var out bytes.Buffer
	stopped := &fakeServiceBackend{status: service.StatusStopped}
	if err := runServiceAction(&out, stopped, "status"); err != nil {
		t.Fatalf("stopped status: %v", err)
	}
	if !strings.Contains(out.String(), "stopped") {
		t.Errorf("stopped output = %q", out.String())
	}

	out.Reset()
	unknown := &fakeServiceBackend{status: service.Status(99)}
	if err := runServiceAction(&out, unknown, "status"); err != nil {
		t.Fatalf("unknown status: %v", err)
	}
	if !strings.Contains(out.String(), "unknown") {
		t.Errorf("unknown output = %q", out.String())
	}

	failing := &fakeServiceBackend{err: errServiceBoom}
	for _, action := range []string{"install", "uninstall", "start", "stop", "restart", "status"} {
		if err := runServiceAction(io.Discard, failing, action); err == nil {
			t.Errorf("%s must propagate backend errors", action)
		}
	}
	if err := runServiceAction(io.Discard, okBackend, "explode"); err == nil {
		t.Error("unknown action must fail")
	}
}

func TestServiceCommandBackendConstructionFailure(t *testing.T) {
	old := newServiceBackend
	defer func() { newServiceBackend = old }()
	newServiceBackend = func(prg *program, cfg *service.Config) (serviceBackend, error) {
		return nil, errServiceBoom
	}
	d := testDeps()
	d.ServiceName = "x"
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"service", "status"}, &out, &errb, d); code != 1 {
		t.Errorf("backend failure exit = %d, want 1", code)
	}
}

func TestServiceCommandViaInjectedBackend(t *testing.T) {
	old := newServiceBackend
	defer func() { newServiceBackend = old }()
	newServiceBackend = func(prg *program, cfg *service.Config) (serviceBackend, error) {
		return &fakeServiceBackend{status: service.StatusRunning}, nil
	}
	d := testDeps()
	d.ServiceName = "x"
	var out, errb bytes.Buffer
	if code := Execute(context.Background(), []string{"service", "status"}, &out, &errb, d); code != 0 {
		t.Fatalf("injected status exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "running") {
		t.Errorf("status output = %q", out.String())
	}
}
