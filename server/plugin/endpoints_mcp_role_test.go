package plugin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

type roleEchoIn struct{}
type roleEchoOut struct {
	OK bool `json:"ok"`
}

func roleSpec() plugin.EndpointSpec[roleEchoIn, roleEchoOut] {
	return plugin.EndpointSpec[roleEchoIn, roleEchoOut]{
		Name:        "admin_desk",
		MCPToolName: "octarq_test__admin_desk",
		Method:      "POST",
		Path:        "/api/admin-desk",
		Summary:     "Admin desk",
		Description: "A tool that only an admin or owner may call.",
		RequireAuth: true,
		RequireRole: []string{"owner", "admin"},
		ExposeMCP:   true,
		Handler: func(context.Context, roleEchoIn) (*roleEchoOut, error) {
			return &roleEchoOut{OK: true}, nil
		},
	}
}

// callMCPTool registers the spec on a fresh server, connects an in-memory
// client, and invokes the tool. The role travels on the session context: that is
// the context a real transport builds from the HTTP request that carried the
// token, and it is what the server handler sees. Put it only on the client's
// CallTool context and the server never receives it.
func callMCPTool(t *testing.T, sessionCtx context.Context) (*mcp.CallToolResult, error) {
	t.Helper()

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	if err := roleSpec().RegisterMCP(srv); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	cli := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, nil)
	sT, cT := mcp.NewInMemoryTransports()
	sSes, err := srv.Connect(sessionCtx, sT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer sSes.Close()
	cSes, err := cli.Connect(sessionCtx, cT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cSes.Close()

	return cSes.CallTool(sessionCtx, &mcp.CallToolParams{Name: "octarq_test__admin_desk"})
}

func TestRegisterMCP_EnforcesRequireRole(t *testing.T) {
	t.Run("member is refused", func(t *testing.T) {
		ctx := plugin.WithTokenRole(plugin.WithOrgID(context.Background(), 7), "member")
		res, err := callMCPTool(t, ctx)
		if err == nil && (res == nil || !res.IsError) {
			t.Fatalf("member called an owner/admin-only tool (result %+v, err %v)", res, err)
		}
		if err != nil && !strings.Contains(err.Error(), "FORBIDDEN") && !strings.Contains(err.Error(), "role") {
			t.Logf("refused with: %v", err)
		}
	})

	t.Run("admin is allowed", func(t *testing.T) {
		ctx := plugin.WithTokenRole(plugin.WithOrgID(context.Background(), 7), "admin")
		res, err := callMCPTool(t, ctx)
		if err != nil {
			t.Fatalf("admin was refused: %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("admin got an error result: %+v", res)
		}
	})

	t.Run("owner is allowed", func(t *testing.T) {
		ctx := plugin.WithTokenRole(plugin.WithOrgID(context.Background(), 7), "owner")
		res, err := callMCPTool(t, ctx)
		if err != nil {
			t.Fatalf("owner was refused: %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("owner got an error result: %+v", res)
		}
	})

	t.Run("no role is refused", func(t *testing.T) {
		// stdio and browser sessions carry no token, so no role ceiling. That is
		// the least privileged case, never a match.
		res, err := callMCPTool(t, plugin.WithOrgID(context.Background(), 7))
		if err == nil && (res == nil || !res.IsError) {
			t.Fatalf("role-less caller reached an owner/admin-only tool (result %+v, err %v)", res, err)
		}
	})
}

func TestRegisterMCP_WithoutRequireRoleStillWorks(t *testing.T) {
	// A spec that declares no roles must stay callable with no role ceiling.
	spec := plugin.EndpointSpec[roleEchoIn, roleEchoOut]{
		Name:        "open_desk",
		MCPToolName: "octarq_test__open_desk",
		Method:      "POST",
		Path:        "/api/open-desk",
		RequireAuth: true,
		ExposeMCP:   true,
		Handler: func(context.Context, roleEchoIn) (*roleEchoOut, error) {
			return &roleEchoOut{OK: true}, nil
		},
	}

	base := plugin.WithOrgID(context.Background(), 7)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	if err := spec.RegisterMCP(srv); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	cli := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, nil)
	sT, cT := mcp.NewInMemoryTransports()
	sSes, err := srv.Connect(base, sT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer sSes.Close()
	cSes, err := cli.Connect(base, cT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cSes.Close()

	res, err := cSes.CallTool(plugin.WithOrgID(context.Background(), 7), &mcp.CallToolParams{Name: "octarq_test__open_desk"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("CallTool returned an error result: %+v", res)
	}
}
