package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/plugin"
)

func lookupProviding(name string, svc any) func(string) (any, bool) {
	return func(n string) (any, bool) {
		if n == name {
			return svc, true
		}
		return nil, false
	}
}

type exportTestPlugin struct {
	name   string
	export plugin.ExportFunc
}

func (p exportTestPlugin) Name() string                          { return p.name }
func (p exportTestPlugin) Models() []any                         { return nil }
func (p exportTestPlugin) Mount(_ plugin.Mux, _ *plugin.Context) {}

func callExport(t *testing.T, lookup func(string) (any, bool), resource string) (*mcpResultProbe, error) {
	t.Helper()
	s := &server{lookup: lookup}
	ctx := plugin.WithOrgID(context.Background(), 7)
	res, _, err := s.exportData(ctx, nil, exportInput{Resource: resource})
	if err != nil {
		return nil, err
	}
	return &mcpResultProbe{isError: res != nil && res.IsError}, nil
}

type mcpResultProbe struct{ isError bool }

func exportPayload(t *testing.T, s *server, resource string) map[string]any {
	t.Helper()
	ctx := plugin.WithOrgID(context.Background(), 7)
	res, _, err := s.exportData(ctx, nil, exportInput{Resource: resource})
	if err != nil {
		t.Fatalf("exportData(%q) returned err: %v", resource, err)
	}
	if res == nil || res.IsError {
		t.Fatalf("exportData(%q) reported an error: %+v", resource, res)
	}
	if len(res.Content) != 1 {
		t.Fatalf("exportData(%q) returned %d content items, want 1", resource, len(res.Content))
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("exportData(%q) content is %T, want *mcp.TextContent", resource, res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("unmarshal export payload: %v", err)
	}
	return payload
}

func TestMCPExport_ResolvesNamedContract(t *testing.T) {
	called := false
	exporter := plugin.MCPExporter(func(ctx context.Context, orgID uint) (any, error) {
		called = true
		if orgID != 7 {
			t.Errorf("orgID = %d, want 7", orgID)
		}
		return map[string]any{"rows": 1}, nil
	})

	probe, err := callExport(t, lookupProviding(plugin.MCPExportServiceName("links"), exporter), "links")
	if err != nil {
		t.Fatalf("exportData returned err: %v", err)
	}
	if !called {
		t.Error("the provided plugin.MCPExporter was never invoked — the named-contract lookup is broken")
	}
	if probe.isError {
		t.Error("exportData reported an error for a mounted resource")
	}
}

func TestMCPExport_BareFuncIsNotAccepted(t *testing.T) {
	bare := func(ctx context.Context, orgID uint) (any, error) {
		t.Error("a bare func must not satisfy the plugin.MCPExporter contract")
		return nil, nil
	}

	probe, err := callExport(t, lookupProviding(plugin.MCPExportServiceName("links"), bare), "links")
	if err != nil {
		t.Fatalf("exportData returned err: %v", err)
	}
	if !probe.isError {
		t.Error("a non-conforming provider should fall through to the unknown-resource reply")
	}
}

func TestMCPExport_AllAndZipURL(t *testing.T) {
	signCalled := false
	signer := plugin.ExportSignedURLFunc(func(orgID uint) (string, error) {
		signCalled = true
		if orgID != 7 {
			t.Errorf("orgID = %d, want 7", orgID)
		}
		return "/api/account/export?token=test-signed-token", nil
	})

	probe, err := callExport(t, lookupProviding(plugin.ServiceExportSignedURL, signer), "all")
	if err != nil {
		t.Fatalf("exportData all returned err: %v", err)
	}
	if probe.isError {
		t.Error("exportData reported error for resource 'all'")
	}
	if !signCalled {
		t.Error("ExportSignedURLFunc was not invoked for resource 'all'")
	}

	signCalled = false
	probe, err = callExport(t, lookupProviding(plugin.ServiceExportSignedURL, signer), "zip_url")
	if err != nil {
		t.Fatalf("exportData zip_url returned err: %v", err)
	}
	if probe.isError {
		t.Error("exportData reported error for resource 'zip_url'")
	}
	if !signCalled {
		t.Error("ExportSignedURLFunc was not invoked for resource 'zip_url'")
	}
}

func TestMCPExport_AllSummaryComesFromPluginExports(t *testing.T) {
	orgSeen := uint(0)
	linksPlugin := exportTestPlugin{name: "links", export: func(orgID uint) map[string]any {
		orgSeen = orgID
		return map[string]any{"links": []int{1, 2, 3}}
	}}
	mailPlugin := exportTestPlugin{name: "mail", export: func(orgID uint) map[string]any {
		return map[string]any{
			"mailboxes": []int{1},
			"emails":    []int{1, 2},
			"note":      "not a collection",
		}
	}}

	reg := plugin.NewRegistry()
	reg.Provide(plugin.ServiceExportSignedURL, plugin.ExportSignedURLFunc(func(orgID uint) (string, error) {
		return "/api/account/export?token=signed", nil
	}))
	reg.Provide(plugin.ExportServiceName("links"), plugin.ExportFunc(linksPlugin.export))
	reg.Provide(plugin.ExportServiceName("mail"), plugin.ExportFunc(mailPlugin.export))

	s := &server{lookup: reg.Lookup, plugins: []plugin.Plugin{linksPlugin, mailPlugin}}
	payload := exportPayload(t, s, "all")

	if orgSeen != 7 {
		t.Errorf("ExportFunc received orgID = %d, want 7", orgSeen)
	}
	summary, ok := payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing or wrong type: %#v", payload["summary"])
	}
	want := map[string]float64{"links": 3, "mailboxes": 1, "emails": 2}
	for key, n := range want {
		if got, _ := summary[key].(float64); got != n {
			t.Errorf("summary[%q] = %v, want %v", key, summary[key], n)
		}
	}
	if _, exists := summary["note"]; exists {
		t.Errorf("non-collection export value must not produce a count: %#v", summary)
	}
	if got := payload["downloadUrl"]; got != "/api/account/export?token=signed" {
		t.Errorf("downloadUrl = %v, want signed URL", got)
	}
}

func TestMCPExport_AllWithoutPluginsDoesNotInventCounts(t *testing.T) {
	s := &server{}
	payload := exportPayload(t, s, "all")

	summary, ok := payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing or wrong type: %#v", payload["summary"])
	}
	if len(summary) != 0 {
		t.Errorf("summary must stay empty without plugin exporters, got %#v", summary)
	}
	for _, name := range []string{"links", "emails", "domains", "mailboxes"} {
		if _, exists := summary[name]; exists {
			t.Errorf("summary invented a count for %q", name)
		}
	}
}

func TestMCPExport_UnknownResourceTextDoesNotEnumerateBusinessTables(t *testing.T) {
	s := &server{}
	res, _, err := s.exportData(plugin.WithOrgID(context.Background(), 7), nil, exportInput{Resource: "unknown"})
	if err != nil {
		t.Fatalf("exportData returned err: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("unknown resource must report an error: %+v", res)
	}
	text, _ := res.Content[0].(*mcp.TextContent)
	if text == nil {
		t.Fatalf("unknown resource content is %T, want *mcp.TextContent", res.Content[0])
	}
	for _, name := range []string{"links", "emails", "domains", "mailboxes"} {
		if strings.Contains(text.Text, name) {
			t.Errorf("unknown-resource reply names business table %q: %s", name, text.Text)
		}
	}
	if !strings.Contains(text.Text, "all") || !strings.Contains(text.Text, "zip_url") {
		t.Errorf("unknown-resource reply must mention all and zip_url: %s", text.Text)
	}
}

func TestMCPExport_AgentAllowlistPreserved(t *testing.T) {
	s := &server{}
	ctx := auth.WithTokenAgent(context.Background(), true, `[]`)
	ctx = plugin.WithOrgID(ctx, 7)

	_, _, err := s.exportData(ctx, nil, exportInput{Resource: "all"})
	if err == nil {
		t.Fatal("agent token without export_data in allowed_tools must be rejected")
	}
	agentErr, ok := plugin.AsAgentError(err)
	if !ok {
		t.Fatalf("expected *plugin.AgentError, got %T", err)
	}
	if agentErr.Code != "FORBIDDEN_AGENT_TOOL_DISALLOWED" {
		t.Errorf("code = %q, want FORBIDDEN_AGENT_TOOL_DISALLOWED", agentErr.Code)
	}

	allowed := auth.WithTokenAgent(context.Background(), true, []string{"octarq_infra__export_data"})
	allowed = plugin.WithOrgID(allowed, 7)
	if _, _, err := s.exportData(allowed, nil, exportInput{Resource: "all"}); err != nil {
		t.Errorf("allowed agent tool must pass: %v", err)
	}
}

func TestMCPExport_HandlerError(t *testing.T) {
	expectedErr := errors.New("exporter db failure")
	exporter := plugin.MCPExporter(func(ctx context.Context, orgID uint) (any, error) {
		return nil, expectedErr
	})

	s := &server{lookup: lookupProviding(plugin.MCPExportServiceName("emails"), exporter)}
	ctx := plugin.WithOrgID(context.Background(), 1)
	_, _, err := s.exportData(ctx, nil, exportInput{Resource: "emails"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMCPExport_NoBusinessTableCountsInSummaryShape(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.Provide(plugin.ExportServiceName("widgets"), plugin.ExportFunc(func(orgID uint) map[string]any {
		return map[string]any{"widgets": []string{"a"}}
	}))
	s := &server{lookup: reg.Lookup, plugins: []plugin.Plugin{exportTestPlugin{name: "widgets"}}}
	payload := exportPayload(t, s, "zip_url")
	summary := payload["summary"].(map[string]any)
	if got, _ := summary["widgets"].(float64); got != 1 {
		t.Errorf("summary[widgets] = %v, want 1", summary["widgets"])
	}
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"widgets"}) {
		t.Errorf("summary keys = %v, want [widgets]", keys)
	}
}
