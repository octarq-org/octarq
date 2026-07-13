package mcp

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestNetworkedConstructorNeverEnablesRawSQL asserts the hard invariant that the
// general-purpose raw-SQL tool is NEVER wired on a networked (HTTP/SSE)
// transport, while the stdio path still gets it. The raw-SQL tool cannot be
// scoped to a single tenant, so exposing it over the network would allow reads
// across tenants.
func TestNetworkedConstructorNeverEnablesRawSQL(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:mcpguard?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// Networked constructor path: allowRawSQL is hard-wired false.
	_, netSrv := buildServerInstance(gdb, 1, nil, false)
	if netSrv.rawSQLEnabled {
		t.Fatal("networked MCP server must never register the raw-SQL tool")
	}

	// Stdio path still enables it.
	_, stdioSrv := buildServerInstance(gdb, 1, nil, true)
	if !stdioSrv.rawSQLEnabled {
		t.Fatal("stdio MCP server should register the raw-SQL tool")
	}
}
