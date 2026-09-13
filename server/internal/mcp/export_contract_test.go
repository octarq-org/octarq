package mcp

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// The MCP export tool resolves "<resource>.mcp_export" through
// plugin.LookupServiceAs[plugin.MCPExporter]. A provider registering a bare
// function whose signature has drifted still compiles; the lookup then finds
// the service, the assertion fails, and exportData answers "unknown resource"
// for a resource that is in fact mounted — a wrong answer rather than an error.
//
// These cases drive exportData itself rather than re-implementing the lookup,
// so short-circuiting the guard reds them exactly where production breaks.

func lookupProviding(name string, svc any) func(string) (any, bool) {
	return func(n string) (any, bool) {
		if n == name {
			return svc, true
		}
		return nil, false
	}
}

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

// The failure this contract exists to prevent: a provider registered as a bare
// func (a drifted signature, or one that simply never converted) must not be
// silently reported as an unknown resource without the guard noticing.
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
