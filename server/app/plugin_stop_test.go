package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

// stopRecorder is a plugin that records every lifecycle call it receives, so a
// test can assert on order rather than on the mere occurrence of a call.
type stopRecorder struct {
	name   string
	calls  *[]string
	err    error
	onStop func(ctx context.Context)
}

func (r stopRecorder) Name() string                      { return r.name }
func (r stopRecorder) Models() []any                     { return nil }
func (r stopRecorder) Mount(plugin.Mux, *plugin.Context) {}
func (r stopRecorder) Stop(ctx context.Context) error {
	*r.calls = append(*r.calls, r.name)
	if r.onStop != nil {
		r.onStop(ctx)
	}
	return r.err
}

// noStopPlugin is the ordinary case — a plugin that never implements Stopper.
type noStopPlugin struct{}

func (noStopPlugin) Name() string                      { return "plain" }
func (noStopPlugin) Models() []any                     { return nil }
func (noStopPlugin) Mount(plugin.Mux, *plugin.Context) {}

// TestStopPluginsRunsInReverseMountOrder pins the contract that makes drain
// safe: a plugin mounted later may depend on a service provided by one mounted
// earlier, so the later one has to stop first.
func TestStopPluginsRunsInReverseMountOrder(t *testing.T) {
	var calls []string
	plugins := []plugin.Plugin{
		stopRecorder{name: "first", calls: &calls},
		noStopPlugin{},
		stopRecorder{name: "second", calls: &calls},
		stopRecorder{name: "third", calls: &calls},
	}
	if err := stopPlugins(context.Background(), nil, plugins); err != nil {
		t.Fatalf("stopPlugins: %v", err)
	}
	want := []string{"third", "second", "first"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("Stop order = %v, want %v", calls, want)
	}
}

// TestStopPluginsCancelsStartContextFirst pins that Start goroutines are
// signalled before any Stop runs. Without this a plugin can be draining while
// its own Start goroutine is still enqueuing work behind it.
func TestStopPluginsCancelsStartContextFirst(t *testing.T) {
	startCtx, cancel := context.WithCancel(context.Background())
	var cancelledWhenStopRan bool
	plugins := []plugin.Plugin{
		stopRecorder{name: "p", calls: new([]string), onStop: func(context.Context) {
			select {
			case <-startCtx.Done():
				cancelledWhenStopRan = true
			default:
			}
		}},
	}
	if err := stopPlugins(context.Background(), cancel, plugins); err != nil {
		t.Fatalf("stopPlugins: %v", err)
	}
	if !cancelledWhenStopRan {
		t.Fatal("Stop ran before the plugin context was cancelled")
	}
}

// TestStopPluginsCollectsEveryError pins that one failing plugin does not skip
// the plugins after it, and that the returned error names the plugin.
func TestStopPluginsCollectsEveryError(t *testing.T) {
	var calls []string
	boom := errors.New("drain failed")
	plugins := []plugin.Plugin{
		stopRecorder{name: "alpha", calls: &calls, err: boom},
		stopRecorder{name: "beta", calls: &calls},
		stopRecorder{name: "gamma", calls: &calls, err: boom},
	}
	err := stopPlugins(context.Background(), nil, plugins)
	if err == nil {
		t.Fatal("stopPlugins returned nil, want the joined plugin errors")
	}
	if got := len(calls); got != 3 {
		t.Fatalf("Stop called %d times, want 3 (a failure must not skip later plugins)", got)
	}
	for _, name := range []string{"alpha", "gamma"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("joined error does not name plugin %q: %v", name, err)
		}
	}
	if !errors.Is(err, boom) {
		t.Errorf("joined error does not wrap the plugin error: %v", err)
	}
}

// TestStopPluginsWithNoStopperIsNotAnError keeps the optional-interface
// promise: a build whose plugins predate Stopper still shuts down cleanly.
func TestStopPluginsWithNoStopperIsNotAnError(t *testing.T) {
	plugins := []plugin.Plugin{noStopPlugin{}, noStopPlugin{}}
	if err := stopPlugins(context.Background(), nil, plugins); err != nil {
		t.Fatalf("stopPlugins without any Stopper: %v", err)
	}
}

// shutdownProbe is a plugin whose Start blocks until its context is cancelled
// and whose Stop records that it ran. It exercises the wiring — Run's mount
// loop, the cancellable plugin context, serve's drain — rather than
// stopPlugins in isolation.
type shutdownProbe struct {
	started chan context.Context
	mu      sync.Mutex
	stopped bool
}

func (p *shutdownProbe) Name() string                      { return "shutdown-probe" }
func (p *shutdownProbe) Models() []any                     { return nil }
func (p *shutdownProbe) Mount(plugin.Mux, *plugin.Context) {}

func (p *shutdownProbe) Start(ctx context.Context) {
	select {
	case p.started <- ctx:
	default:
	}
	<-ctx.Done()
}

func (p *shutdownProbe) Stop(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
	return nil
}

// TestRunHandsPluginsACancellableContext is the end-to-end half of the drain
// contract. Two things have to hold together, and this test and
// TestStopPluginsCancelsStartContextFirst cover them as a pair:
//
//   - here: Run must NOT hand plugins the root context. main passes
//     context.Background(), so a Start that waits on <-ctx.Done() — which is
//     exactly what plugins/links does before closing its engine — would wait
//     forever on SIGTERM.
//   - there: the context Run does hand out is cancelled before any Stop runs.
//
// Identity is the assertion because it is the defect: the context is either a
// derived, cancellable one or it is the root, and only the latter is broken.
func TestRunHandsPluginsACancellableContext(t *testing.T) {
	a := bootApp(t)
	probe := &shutdownProbe{started: make(chan context.Context, 1)}
	a.Use(probe)

	root, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(root); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}

	select {
	case startCtx := <-probe.started:
		if startCtx == root {
			t.Error("Start received the root context; a signal-driven shutdown would never cancel it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start was never called")
	}

	probe.mu.Lock()
	stopped := probe.stopped
	probe.mu.Unlock()
	if !stopped {
		t.Fatal("Stop was never called on shutdown")
	}
}
