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
	gdb := setupExtraResourcesTestDB(t)

	// 1. RegisterTenantView in NewServerInstance
	srv := NewServerInstance(gdb, 1, []plugin.Plugin{dummyTenantViewPlugin{}})
	if srv == nil {
		t.Fatalf("expected non-nil server")
	}

	// 2. RunWithPlugins config load error with invalid driver
	t.Setenv("OCTARQ_DB_DRIVER", "invalid_driver")
	err := RunWithPlugins(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "mcp: load config") {
		t.Errorf("expected mcp: load config error, got %v", err)
	}

	// 3. RunWithPlugins db open error with unopenable sqlite DSN
	t.Setenv("OCTARQ_SECRET_KEY", "abcdefghijklmnopqrstuvwxyz1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "admin123456")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", "/dev/null/nonexistent/db.sqlite")
	err = RunWithPlugins(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "mcp: open db") {
		t.Errorf("expected mcp: open db error, got %v", err)
	}
}

func TestCoverageLiftExtraResourceErrorPaths(t *testing.T) {
	ctx := context.Background()
	gdb := setupExtraResourcesTestDB(t)

	s := &server{
		gdb:   gdb,
		orgID: 0, // Unscoped, so resolveOrg will error without context org
		subs:  nil,
	}

	// 1. handleSubscribe when s.subs is nil
	err := s.handleSubscribe(ctx, &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: "octarq://test"},
	})
	if err != nil {
		t.Fatalf("handleSubscribe failed: %v", err)
	}
	if s.subs == nil || !s.subs.watched("octarq://test") {
		t.Fatalf("expected uri to be added to subscriptions")
	}

	// 2. handleUnsubscribe invalid params
	err = s.handleUnsubscribe(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "missing unsubscribe URI") {
		t.Errorf("expected missing unsubscribe URI error, got %v", err)
	}

	// handleUnsubscribe with s.subs == nil
	s.subs = nil
	err = s.handleUnsubscribe(ctx, &mcp.UnsubscribeRequest{
		Params: &mcp.UnsubscribeParams{URI: "octarq://test"},
	})
	if err != nil {
		t.Errorf("expected nil error when s.subs is nil, got %v", err)
	}

	// 3. handleResourceLinksTrending error paths
	_, err = s.handleResourceLinksTrending(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "missing request params") {
		t.Errorf("expected missing request params error, got %v", err)
	}

	_, err = s.handleResourceLinksTrending(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "octarq://wrong"},
	})
	if err == nil {
		t.Errorf("expected ResourceNotFoundError for wrong URI, got nil")
	}

	_, err = s.handleResourceLinksTrending(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: ResourceURILinksTrending},
	})
	if err == nil {
		t.Errorf("expected resolveOrg error when orgID is 0, got nil")
	}

	// 4. handleResourceMailboxLatest error paths
	_, err = s.handleResourceMailboxLatest(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "missing request params") {
		t.Errorf("expected missing request params error, got %v", err)
	}

	_, err = s.handleResourceMailboxLatest(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "octarq://mailboxes/invalid/latest"},
	})
	if err == nil {
		t.Errorf("expected ResourceNotFoundError for invalid mailbox URI, got nil")
	}

	_, err = s.handleResourceMailboxLatest(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "octarq://mailboxes/1/latest"},
	})
	if err == nil {
		t.Errorf("expected resolveOrg error when orgID is 0, got nil")
	}

	// handleResourceMailboxLatest when gdb is nil
	sNilDB := &server{gdb: nil, orgID: 1}
	res, err := sNilDB.handleResourceMailboxLatest(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "octarq://mailboxes/1/latest"},
	})
	if err != nil || res == nil {
		t.Errorf("expected successful empty resource when gdb is nil, got %v", err)
	}

	// 5. marshalResource with unmarshalable object (channels)
	_, err = marshalResource("octarq://test", make(chan int))
	if err == nil {
		t.Errorf("expected json marshal error, got nil")
	}
}
