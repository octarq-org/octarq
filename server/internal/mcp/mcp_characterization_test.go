package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func newMCPCharTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mcp_char.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	return db
}

func TestMCP_Characterization_ServerConstruction(t *testing.T) {
	db := newMCPCharTestDB(t)

	// 1. Basic server creation via NewServerInstance
	mock := &mockMCPPlugin{name: "mock-mcp-char"}
	srv := NewServerInstance(db, 1, []plugin.Plugin{mock})
	if srv == nil {
		t.Fatal("expected non-nil MCP server from NewServerInstance()")
	}
	if !mock.mountCalled {
		t.Error("expected plugin Mount to be called in NewServerInstance")
	}
	if !mock.registerCalled {
		t.Error("expected plugin RegisterMCP to be called in NewServerInstance")
	}

	// 2. Networked server creation via NewNetworkedServerInstance
	mockNet := &mockMCPPlugin{name: "mock-mcp-net"}
	srvNet := NewNetworkedServerInstance(db, 1, []plugin.Plugin{mockNet}, func(string) (any, bool) { return nil, false })
	if srvNet == nil {
		t.Fatal("expected non-nil MCP server from NewNetworkedServerInstance()")
	}
	// Networked instances MUST NOT mount plugins
	if mockNet.mountCalled {
		t.Error("expected plugin Mount NOT to be called in NewNetworkedServerInstance")
	}
	// But must register tools
	if !mockNet.registerCalled {
		t.Error("expected plugin RegisterMCP to be called in NewNetworkedServerInstance")
	}
}

func TestMCP_Characterization_SubscriptionLifecycle(t *testing.T) {
	tracker := &subscriptionTracker{}
	uri := "octarq://mailboxes/1/latest"

	if tracker.watched(uri) {
		t.Error("expected uri not watched initially")
	}

	tracker.add(uri)
	if !tracker.watched(uri) {
		t.Error("expected uri watched after add")
	}

	tracker.remove(uri)
	if tracker.watched(uri) {
		t.Error("expected uri not watched after remove")
	}

	// Server level subscribe/unsubscribe
	s := &server{subs: &subscriptionTracker{}}
	if err := s.handleSubscribe(context.Background(), &mcpsdk.SubscribeRequest{Params: &mcpsdk.SubscribeParams{URI: uri}}); err != nil {
		t.Fatalf("handleSubscribe failed: %v", err)
	}
	if !s.subs.watched(uri) {
		t.Error("expected uri watched after handleSubscribe")
	}

	if err := s.handleUnsubscribe(context.Background(), &mcpsdk.UnsubscribeRequest{Params: &mcpsdk.UnsubscribeParams{URI: uri}}); err != nil {
		t.Fatalf("handleUnsubscribe failed: %v", err)
	}
	if s.subs.watched(uri) {
		t.Error("expected uri unwatched after handleUnsubscribe")
	}
}

func TestMCP_Characterization_SystemResources(t *testing.T) {
	db := newMCPCharTestDB(t)
	s := &server{gdb: db, orgID: 1}

	// 1. Read octarq://metrics/traffic
	resTraffic, err := s.handleResourceTraffic(context.Background(), &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: ResourceURITrafficMetrics},
	})
	if err != nil {
		t.Fatalf("handleResourceTraffic failed: %v", err)
	}
	if len(resTraffic.Contents) == 0 {
		t.Fatal("expected non-empty contents in traffic metrics resource")
	}

	// 2. Read octarq://domains/health
	resDomains, err := s.handleResourceDomainsHealth(context.Background(), &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: ResourceURIDomainsHealth},
	})
	if err != nil {
		t.Fatalf("handleResourceDomainsHealth failed: %v", err)
	}
	if len(resDomains.Contents) == 0 {
		t.Fatal("expected non-empty contents in domains health resource")
	}

	// 3. Error on mismatched URI
	_, err = s.handleResourceTraffic(context.Background(), &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: "octarq://unknown"},
	})
	if err == nil {
		t.Error("expected error on mismatched URI")
	}
}
