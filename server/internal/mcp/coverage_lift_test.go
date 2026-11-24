package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

type dummyTenantViewPlugin struct{}

func (dummyTenantViewPlugin) Name() string  { return "dummy_tv" }
func (dummyTenantViewPlugin) Models() []any { return nil }
func (dummyTenantViewPlugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	if ctx.RegisterTenantView != nil {
		ctx.RegisterTenantView(plugin.TenantView{
			Name: "tenant_test_tv_view",
			Definition: func(orgID uint) string {
				return "SELECT 1"
			},
		})
	}
}

func TestCoverageLiftRegisterTenantViewAndRunWithPlugins(t *testing.T) {
	gdb := newMCPCharTestDB(t)

	srv := NewServerInstance(gdb, 1, []plugin.Plugin{dummyTenantViewPlugin{}}, nil)
	if srv == nil {
		t.Fatalf("expected non-nil server")
	}

	t.Setenv("OCTARQ_DB_DRIVER", "invalid_driver")
	err := RunWithPlugins(context.Background(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "mcp: load config") {
		t.Errorf("expected mcp: load config error, got %v", err)
	}

	t.Setenv("OCTARQ_SECRET_KEY", "abcdefghijklmnopqrstuvwxyz1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "octarq-test-pw")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", "/dev/null/nonexistent/db.sqlite")
	err = RunWithPlugins(context.Background(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "mcp: open db") {
		t.Errorf("expected mcp: open db error, got %v", err)
	}
}

func TestCoverageLiftSubscriptionErrorPaths(t *testing.T) {
	ctx := context.Background()
	s := &server{}

	if err := s.handleSubscribe(ctx, nil); err == nil {
		t.Error("expected missing subscribe URI error for nil request")
	}
	if err := s.handleSubscribe(ctx, &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{}}); err == nil {
		t.Error("expected missing subscribe URI error for empty URI")
	}

	if err := s.handleSubscribe(ctx, &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: "octarq://test"}}); err != nil {
		t.Fatalf("handleSubscribe failed: %v", err)
	}
	if s.subs == nil || !s.subs.watched("octarq://test") {
		t.Fatalf("expected uri to be added to subscriptions")
	}

	if err := s.handleUnsubscribe(ctx, nil); err == nil || !strings.Contains(err.Error(), "missing unsubscribe URI") {
		t.Errorf("expected missing unsubscribe URI error, got %v", err)
	}

	if err := s.handleUnsubscribe(ctx, &mcp.UnsubscribeRequest{Params: &mcp.UnsubscribeParams{URI: "octarq://test"}}); err != nil {
		t.Fatalf("handleUnsubscribe failed: %v", err)
	}
	if s.subs.watched("octarq://test") {
		t.Error("expected uri to be removed from subscriptions")
	}

	s.subs = nil
	if err := s.handleUnsubscribe(ctx, &mcp.UnsubscribeRequest{Params: &mcp.UnsubscribeParams{URI: "octarq://test"}}); err != nil {
		t.Errorf("expected nil error when subs is nil, got %v", err)
	}
}
