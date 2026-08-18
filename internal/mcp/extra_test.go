package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type mockMCPPlugin struct {
	name           string
	registerCalled bool
	mountCalled    bool
}

func (m *mockMCPPlugin) Name() string                              { return m.name }
func (m *mockMCPPlugin) Models() []any                             { return nil }
func (m *mockMCPPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) { m.mountCalled = true }
func (m *mockMCPPlugin) RegisterMCP(srv *mcp.Server)               { m.registerCalled = true }

func TestBuildServerInstance_WithMCPProvider(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mcpextra.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	p := &mockMCPPlugin{name: "mock-mcp"}
	srv := NewServerInstance(gdb, 1, []plugin.Plugin{p})
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if !p.mountCalled {
		t.Error("expected plugin Mount to be called")
	}
	if !p.registerCalled {
		t.Error("expected plugin RegisterMCP to be called")
	}
}

func TestAuditQuery_TruncateAndNilDB(t *testing.T) {
	// 1. gdb is nil
	nilServer := &server{gdb: nil}
	nilServer.auditQuery("SELECT 1", 1, nil) // should not panic

	// 2. long query truncated at 500 chars
	dbPath := filepath.Join(t.TempDir(), "mcpaudit.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}

	s := &server{gdb: gdb, orgID: 1}
	longSQL := "SELECT '" + strings.Repeat("A", 600) + "'"
	s.auditQuery(longSQL, 1, nil)

	var log auditRow
	if err := gdb.Where("action = ?", "ai.mcp.query").Last(&log).Error; err != nil {
		t.Fatalf("audit log not found: %v", err)
	}
	if !strings.Contains(log.Meta, strings.Repeat("A", 100)) {
		t.Errorf("expected audit log to contain SQL snippet, got: %s", log.Meta)
	}
}

func TestExportData_HandlerError(t *testing.T) {
	expectedErr := errors.New("exporter db failure")
	exporter := plugin.MCPExporter(func(ctx context.Context, orgID uint) (any, error) {
		return nil, expectedErr
	})

	s := &server{
		lookup: lookupProviding(plugin.MCPExportServiceName("emails"), exporter),
	}
	ctx := plugin.WithOrgID(context.Background(), 1)
	_, _, err := s.exportData(ctx, nil, exportInput{Resource: "emails"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestJsonResult_Error(t *testing.T) {
	// Channels cannot be marshaled to JSON
	ch := make(chan int)
	_, _, err := jsonResult(ch)
	if err == nil {
		t.Error("expected json marshal error for chan, got nil")
	}
}

func TestRunAndRunWithPlugins(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mcprun.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-1234567890123456")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled context so StdioTransport does not block
	_ = Run(ctx)
	_ = RunWithPlugins(ctx, nil)
}
