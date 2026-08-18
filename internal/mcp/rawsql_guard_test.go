package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listToolNames drives a real MCP session against srv and returns the tool names
// the server actually advertises. Asserting on the wire listing rather than on an
// internal bookkeeping flag is deliberate: the flag can agree with the comments
// while the tool is still reachable by a client.
func listToolNames(t *testing.T, srv *mcp.Server) []string {
	t.Helper()
	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "guard-test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// No transport may expose a tool that runs caller-supplied SQL. Such a query
// reaches arbitrary tables, so nothing can confine it to the caller's owner_id —
// the content denylist in plugin.ValidateReadOnlyQuery leaves the analytics
// tables queryable and `emails` has no owner_id column at all. The capability is
// gone from the code, not gated by a flag, so no call site can restore it.
func TestNoTransportExposesRawSQL(t *testing.T) {
	cases := []struct {
		name string
		srv  *mcp.Server
	}{
		{"stdio", NewServerInstance(nil, 1, nil)},
		{"networked", NewNetworkedServerInstance(nil, 42, nil, nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := listToolNames(t, tc.srv)

			// Fail loudly if the listing came back empty — otherwise the
			// absence assertions below would pass without proving anything.
			if len(names) == 0 {
				t.Fatal("no tools advertised at all; the listing is not exercising the server")
			}
			var sawExport bool
			for _, n := range names {
				if n == "export_data" {
					sawExport = true
				}
				if n == "query_db_readonly" {
					t.Errorf("CROSS-TENANT: the %s transport advertises query_db_readonly; raw SQL cannot be scoped to one owner_id", tc.name)
				}
				if strings.Contains(strings.ToLower(n), "sql") {
					t.Errorf("tool %q looks like a raw-SQL tool on the %s transport", n, tc.name)
				}
			}
			if !sawExport {
				t.Errorf("expected the tenant-scoped export_data tool in %v", names)
			}
		})
	}
}
