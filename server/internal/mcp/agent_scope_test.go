package mcp_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/auth"
	mcp_internal "github.com/octarq-org/octarq/server/internal/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/links"
	"gorm.io/gorm"
)

func setupAgentScopeTest(t *testing.T) (*gorm.DB, *links.Plugin, *links.Link) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agent_scope.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.Token{}, &links.Link{}, &links.LinkEvent{}); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}

	lp := links.New()
	pctx := &plugin.Context{
		DB: db,
	}
	pctx.Host = plugin.EnsureHost(pctx)
	lp.Mount(nil, pctx)

	testLink := &links.Link{
		OrgID:  1,
		Host:   "go.example.com",
		Slug:   "test-agent-link",
		Target: "https://example.com/target",
		Title:  "Test Agent Link",
	}
	if err := db.Create(testLink).Error; err != nil {
		t.Fatalf("failed to create test link: %v", err)
	}

	return db, lp, testLink
}

func runToolCall(ctx context.Context, srv *mcpsdk.Server, toolName string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent-test-client", Version: "1.0"}, nil)
	sTrans, cTrans := mcpsdk.NewInMemoryTransports()

	sSes, err := srv.Connect(ctx, sTrans, nil)
	if err != nil {
		return nil, err
	}
	defer sSes.Close()

	cSes, err := client.Connect(context.Background(), cTrans, nil)
	if err != nil {
		return nil, err
	}
	defer cSes.Close()

	return cSes.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

// Test用例 1（正常放行）: 签发仅允许 octarq_network__list_links 的 Agent Token，调用成功返回。
func TestAgentScope_TestCase1_AllowedToolPasses(t *testing.T) {
	db, lp, testLink := setupAgentScopeTest(t)
	srv := mcp_internal.NewNetworkedServerInstance(db, 1, []plugin.Plugin{lp}, nil)

	agentTok := models.Token{
		IsAgent:      true,
		AllowedTools: `["octarq_network__list_links"]`,
	}

	ctx := auth.WithOrgID(context.Background(), 1)
	ctx = auth.WithTokenAgent(ctx, agentTok.IsAgent, agentTok.AllowedTools)

	res, err := runToolCall(ctx, srv, "octarq_network__list_links", nil)
	if err != nil {
		t.Fatalf("unexpected error calling allowed tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected tool call to succeed, but got error: %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty tool output content")
	}

	textRes, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(textRes.Text, testLink.Slug) {
		t.Errorf("expected output to contain link slug %q, got: %s", testLink.Slug, textRes.Text)
	}
}

// Test用例 2（越权拦截 Fail-Closed）: 尝试调用未在白名单中的 octarq_network__delete_link，
// 必须严格 Fail-Closed 返回 403 FORBIDDEN_AGENT_TOOL_DISALLOWED，且数据库记录未发生任何变更。
func TestAgentScope_TestCase2_DisallowedToolFailClosed(t *testing.T) {
	db, lp, testLink := setupAgentScopeTest(t)
	srv := mcp_internal.NewNetworkedServerInstance(db, 1, []plugin.Plugin{lp}, nil)

	agentTok := models.Token{
		IsAgent:      true,
		AllowedTools: `["octarq_network__list_links"]`,
	}

	ctx := auth.WithOrgID(context.Background(), 1)
	ctx = auth.WithTokenAgent(ctx, agentTok.IsAgent, agentTok.AllowedTools)

	// Attempt unauthorized tool invocation
	res, err := runToolCall(ctx, srv, "octarq_network__delete_link", map[string]any{
		"id": testLink.ID,
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected tool call to be rejected with error, but got success: %+v", res)
	}

	// Verify error code in MCP response content
	var contentText string
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			contentText += tc.Text
		}
	}
	if !strings.Contains(contentText, "FORBIDDEN_AGENT_TOOL_DISALLOWED") {
		t.Errorf("expected response to contain FORBIDDEN_AGENT_TOOL_DISALLOWED, got: %s", contentText)
	}
	if !strings.Contains(contentText, "not authorized for this agent token") {
		t.Errorf("expected descriptive authorization failure message, got: %s", contentText)
	}

	// Assert DB record unchanged: testLink MUST still exist in database
	var remainingLink links.Link
	if err := db.First(&remainingLink, testLink.ID).Error; err != nil {
		t.Fatalf("expected link %d to remain in database, but query returned: %v", testLink.ID, err)
	}
	if remainingLink.Slug != testLink.Slug {
		t.Errorf("database link slug mismatch: got %q, want %q", remainingLink.Slug, testLink.Slug)
	}
}

// Test用例 3（人类 Token 向后兼容）: 普通 Token (is_agent == false) 调用任意注册工具不受限制。
func TestAgentScope_TestCase3_HumanTokenBackwardCompatible(t *testing.T) {
	db, lp, testLink := setupAgentScopeTest(t)
	srv := mcp_internal.NewNetworkedServerInstance(db, 1, []plugin.Plugin{lp}, nil)

	humanTok := models.Token{
		IsAgent:      false,
		AllowedTools: "",
	}

	ctx := auth.WithOrgID(context.Background(), 1)
	ctx = auth.WithTokenAgent(ctx, humanTok.IsAgent, humanTok.AllowedTools)

	// 1. Human token can call octarq_network__list_links
	resList, err := runToolCall(ctx, srv, "octarq_network__list_links", nil)
	if err != nil {
		t.Fatalf("unexpected error calling list links with human token: %v", err)
	}
	if resList.IsError {
		t.Fatalf("expected list links to succeed for human token, got: %+v", resList)
	}

	// 2. Human token can call octarq_network__delete_link
	resDelete, err := runToolCall(ctx, srv, "octarq_network__delete_link", map[string]any{
		"id": testLink.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error calling delete link with human token: %v", err)
	}
	if resDelete.IsError {
		t.Fatalf("expected delete link to succeed for human token, got: %+v", resDelete)
	}

	// Assert DB record was actually deleted by human token
	var deletedLink links.Link
	if err := db.First(&deletedLink, testLink.ID).Error; err == nil {
		t.Errorf("expected link %d to be deleted from database by human token, but it still exists", testLink.ID)
	}
}

// Test empty AllowedTools on Agent Token strictly fails closed on all tools.
func TestAgentScope_EmptyAllowedTools_StrictFailClosed(t *testing.T) {
	db, lp, _ := setupAgentScopeTest(t)
	srv := mcp_internal.NewNetworkedServerInstance(db, 1, []plugin.Plugin{lp}, nil)

	emptyAgentTok := models.Token{
		IsAgent:      true,
		AllowedTools: "",
	}

	ctx := auth.WithOrgID(context.Background(), 1)
	ctx = auth.WithTokenAgent(ctx, emptyAgentTok.IsAgent, emptyAgentTok.AllowedTools)

	res, err := runToolCall(ctx, srv, "octarq_network__list_links", nil)
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected empty agent token to fail closed, got: %+v", res)
	}

	var contentText string
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			contentText += tc.Text
		}
	}
	if !strings.Contains(contentText, "FORBIDDEN_AGENT_TOOL_DISALLOWED") {
		t.Errorf("expected response to contain FORBIDDEN_AGENT_TOOL_DISALLOWED, got: %s", contentText)
	}
}
