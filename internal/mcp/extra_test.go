package mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	links "github.com/octarq-org/octarq/plugins/links"
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
	srv := NewServerInstance(gdb, 1, []plugin.Plugin{p}, true)
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

func TestReadOnlyQuery_MaxRowsAndBlob(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mcpmaxrows.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&links.Link{}, &models.AuditLog{}); err != nil {
		t.Fatal(err)
	}

	// Insert maxRows + 5 rows
	for i := 1; i <= maxRows+5; i++ {
		gdb.Create(&links.Link{
			OrgID:  1,
			Slug:   fmt.Sprintf("slug-%d", i),
			Target: "https://example.com",
		})
	}

	s := &server{gdb: gdb, orgID: 1}
	res, anyOut, err := s.queryDBReadonly(context.Background(), nil, queryInput{
		Query: "SELECT id, slug, CAST(slug AS BLOB) as blob_col FROM links ORDER BY id ASC",
	})
	if err != nil {
		t.Fatalf("queryDBReadonly error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected successful query result, got isError")
	}
	qOut, ok := anyOut.(queryOutput)
	if !ok {
		t.Fatalf("expected queryOutput, got %T", anyOut)
	}
	if qOut.Count != maxRows {
		t.Errorf("expected count == %d (maxRows), got %d", maxRows, qOut.Count)
	}
	if !qOut.Truncated {
		t.Errorf("expected Truncated == true")
	}
}

func TestRunReadOnlyQuery_QueryError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mcpqerr.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{gdb: gdb, orgID: 1}
	// Valid SELECT syntax according to validator, but table does not exist in DB
	_, _, err = s.runReadOnlyQuery(context.Background(), "SELECT * FROM nonexistent_table_123")
	if err == nil {
		t.Error("expected error querying non-existent table, got nil")
	}
}

func TestNormalizeSQLValue(t *testing.T) {
	b := []byte("hello")
	if got := normalizeSQLValue(b); got != "hello" {
		t.Errorf("got %v, want hello", got)
	}
	if got := normalizeSQLValue(123); got != 123 {
		t.Errorf("got %v, want 123", got)
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
