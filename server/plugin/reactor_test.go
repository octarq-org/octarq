package plugin_test

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

type dummyReactor struct {
	events []string
	react  func(ctx context.Context, env plugin.Envelope) error
}

func (d *dummyReactor) Events() []string { return d.events }
func (d *dummyReactor) React(ctx context.Context, env plugin.Envelope) error {
	if d.react != nil {
		return d.react(ctx, env)
	}
	return nil
}

type debounceDummyReactor struct {
	dummyReactor
	interval time.Duration
}

func (d *debounceDummyReactor) MinInterval() time.Duration { return d.interval }

type customEntityKeyReactor struct {
	dummyReactor
	keyFn func(env plugin.Envelope) string
}

func (c *customEntityKeyReactor) EntityKey(env plugin.Envelope) string {
	if c.keyFn != nil {
		return c.keyFn(env)
	}
	return env.EntityKey
}

func TestRegisterReactor_NilContext(t *testing.T) {
	r := &dummyReactor{events: []string{"link.click"}}
	if err := plugin.RegisterReactor(nil, r); err != nil {
		t.Fatalf("expected nil error when ctx is nil, got %v", err)
	}
}

func TestRegisterReactor_NilHook(t *testing.T) {
	ctx := &plugin.Context{}
	r := &dummyReactor{events: []string{"link.click"}}
	if err := plugin.RegisterReactor(ctx, r); err != nil {
		t.Fatalf("expected nil error when ctx.RegisterReactor is nil, got %v", err)
	}
}

func TestRegisterReactor_NilReactor(t *testing.T) {
	ctx := &plugin.Context{
		RegisterReactor: func(r plugin.EventReactor) error {
			return nil
		},
	}
	if err := plugin.RegisterReactor(ctx, nil); err == nil {
		t.Fatal("expected error when reactor is nil, got nil")
	}
}

func TestRegisterReactor_EmptyEvents(t *testing.T) {
	ctx := &plugin.Context{
		RegisterReactor: func(r plugin.EventReactor) error {
			return nil
		},
	}
	r := &dummyReactor{events: []string{}}
	if err := plugin.RegisterReactor(ctx, r); err == nil {
		t.Fatal("expected error when reactor has empty events, got nil")
	}
}

func TestRegisterReactor_InvalidEventKey(t *testing.T) {
	ctx := &plugin.Context{
		RegisterReactor: func(r plugin.EventReactor) error {
			return nil
		},
	}
	r := &dummyReactor{events: []string{"   "}}
	if err := plugin.RegisterReactor(ctx, r); err == nil {
		t.Fatal("expected error when event key is whitespace, got nil")
	}
}

func TestRegisterReactor_Delegation(t *testing.T) {
	var received plugin.EventReactor
	ctx := &plugin.Context{
		RegisterReactor: func(r plugin.EventReactor) error {
			received = r
			return nil
		},
	}
	r := &dummyReactor{events: []string{"link.click", "link.anomaly"}}
	if err := plugin.RegisterReactor(ctx, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != r {
		t.Fatalf("expected received reactor %v, got %v", r, received)
	}
}

func TestReactor_Interfaces(t *testing.T) {
	deb := &debounceDummyReactor{
		dummyReactor: dummyReactor{events: []string{"test.event"}},
		interval:     5 * time.Second,
	}
	if deb.MinInterval() != 5*time.Second {
		t.Errorf("expected MinInterval 5s, got %v", deb.MinInterval())
	}

	cek := &customEntityKeyReactor{
		dummyReactor: dummyReactor{events: []string{"test.event"}},
		keyFn: func(env plugin.Envelope) string {
			return "custom:" + env.EntityKey
		},
	}
	env := plugin.Envelope{EntityKey: "123"}
	if cek.EntityKey(env) != "custom:123" {
		t.Errorf("expected custom:123, got %s", cek.EntityKey(env))
	}
}
