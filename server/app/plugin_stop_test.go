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

type noStopPlugin struct{}

func (noStopPlugin) Name() string                      { return "plain" }
func (noStopPlugin) Models() []any                     { return nil }
func (noStopPlugin) Mount(plugin.Mux, *plugin.Context) {}

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

func TestStopPluginsWithNoStopperIsNotAnError(t *testing.T) {
	plugins := []plugin.Plugin{noStopPlugin{}, noStopPlugin{}}
	if err := stopPlugins(context.Background(), nil, plugins); err != nil {
		t.Fatalf("stopPlugins without any Stopper: %v", err)
	}
}

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
