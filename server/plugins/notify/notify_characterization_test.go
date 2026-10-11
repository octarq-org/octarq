package notify_test

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/notify"
)

// Compile-time interface assertions.
var (
	_ plugin.Plugin    = (*notify.Plugin)(nil)
	_ plugin.Describer = (*notify.Plugin)(nil)
)

func TestNotify_Characterization_Metadata(t *testing.T) {
	p := notify.New()

	if p.Name() != "notify" {
		t.Fatalf("expected Name %q, got %q", "notify", p.Name())
	}
	if p.Models() != nil {
		t.Fatalf("expected Models() == nil, got %v", p.Models())
	}

	info := p.Describe()
	if info.Title != "Notify" {
		t.Errorf("expected Title %q, got %q", "Notify", info.Title)
	}
	if info.Category != plugin.CategoryUtilities {
		t.Errorf("expected Category %q, got %q", plugin.CategoryUtilities, info.Category)
	}
	if !info.Core {
		t.Error("expected Core to be true")
	}
	if !info.EnabledByDefault {
		t.Error("expected EnabledByDefault to be true")
	}
	if info.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestNotify_Characterization_EndpointRegistration(t *testing.T) {
	p := notify.New()
	engine := endpoint.NewEngine()

	p.Mount(nil, &plugin.Context{
		RegisterEndpoint: engine.Register,
	})

	ep, ok := engine.Lookup("notify")
	if !ok || ep == nil {
		t.Fatal("endpoint notify was not registered")
	}

	if ep.EndpointMethod() != "POST" {
		t.Errorf("expected method POST, got %q", ep.EndpointMethod())
	}
	if ep.EndpointPath() != "/api/notify" {
		t.Errorf("expected path /api/notify, got %q", ep.EndpointPath())
	}
	if !ep.EndpointRequireAuth() {
		t.Error("expected RequireAuth to be true")
	}
	if !ep.EndpointExposeMCP() {
		t.Error("expected ExposeMCP to be true")
	}
	if ep.EndpointSummary() != "Send Notification" {
		t.Errorf("expected Summary 'Send Notification', got %q", ep.EndpointSummary())
	}
}

func TestNotify_Characterization_FailClosedInputs(t *testing.T) {
	p := notify.New()
	engine := endpoint.NewEngine()
	p.Mount(nil, &plugin.Context{
		RegisterEndpoint: engine.Register,
	})
	ep, ok := engine.Lookup("notify")
	if !ok {
		t.Fatal("notify endpoint not found")
	}

	// 1. Missing message must fail
	ctx := plugin.WithOrgID(context.Background(), 1)
	if _, err := ep.Execute(ctx, notify.NotifyInput{Message: "   "}); err == nil {
		t.Error("expected error for empty message, got nil")
	}

	// 2. Missing org ID must fail
	if _, err := ep.Execute(context.Background(), notify.NotifyInput{Message: "hello"}); err == nil {
		t.Error("expected error for missing workspace orgID, got nil")
	}
}
