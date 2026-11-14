package ontology

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// connect serves srv over an in-memory transport and returns a client for it.
// The connection context carries the workspace and the role, the same way the
// stdio entry point stamps them.
func connect(t *testing.T, srv *mcp.Server) (*mcp.ClientSession, func()) {
	t.Helper()
	return connectAs(t, srv, "member")
}

// connectAs is connect with an explicit role ceiling, for the tools a member
// cannot reach.
func connectAs(t *testing.T, srv *mcp.Server, role string) (*mcp.ClientSession, func()) {
	t.Helper()
	ctx := plugin.WithTokenRole(plugin.WithOrgID(context.Background(), 1), role)

	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	return cs, func() {
		_ = cs.Close()
		_ = ss.Close()
	}
}

func mountedMCP(t *testing.T) *Plugin {
	t.Helper()
	db := openDB(t)
	t.Setenv(EntitiesEnv, ticketDoc)
	t.Setenv(EntitiesFileEnv, "")
	p := New()
	p.Mount(nil, &plugin.Context{DB: db})
	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return p
}

// TestRegisterMCPPublishesDeclaredTools is the agent-facing contract: one tool
// per exposed operation, named for the entity and the operation.
func TestRegisterMCPPublishesDeclaredTools(t *testing.T) {
	p := mountedMCP(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	client, closeFn := connect(t, srv)
	defer closeFn()

	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	got := strings.Join(names, ",")
	want := "octarq_data__create_tickets,octarq_data__delete_tickets,octarq_data__get_tickets,octarq_data__list_tickets,octarq_data__update_tickets"
	if got != want {
		t.Fatalf("tools = %s, want %s", got, want)
	}
}

// TestMCPToolsReadAndWriteRecords drives the generated tools end to end: an
// agent creates a ticket, lists it, updates it and deletes it.
func TestMCPToolsReadAndWriteRecords(t *testing.T) {
	p := mountedMCP(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	client, closeFn := connect(t, srv)
	defer closeFn()
	ctx := context.Background()

	created, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_data__create_tickets",
		Arguments: map[string]any{"title": "Printer is down", "priority": 1},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.IsError {
		t.Fatalf("create failed: %v", created.Content)
	}
	id := resultString(t, created, "id")
	if id == "" {
		t.Fatalf("create returned no id: %v", created.Content)
	}

	listed, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "octarq_data__list_tickets"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listed.IsError {
		t.Fatalf("list failed: %v", listed.Content)
	}
	if !strings.Contains(textOf(listed), "Printer is down") {
		t.Fatalf("list did not return the created record: %s", textOf(listed))
	}

	updated, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_data__update_tickets",
		Arguments: map[string]any{"id": id, "status": "closed"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.IsError {
		t.Fatalf("update failed: %v", updated.Content)
	}
	if !strings.Contains(textOf(updated), `"status": "closed"`) {
		t.Fatalf("update did not apply: %s", textOf(updated))
	}

	deleted, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_data__delete_tickets",
		Arguments: map[string]any{"id": id},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	// delete requires admin and the connection carries member, so this must be
	// refused, and the record must still be there.
	if !deleted.IsError {
		t.Fatalf("delete was allowed for a member: %s", textOf(deleted))
	}
	after, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_data__get_tickets",
		Arguments: map[string]any{"id": id},
	})
	if err != nil {
		t.Fatalf("get after refused delete: %v", err)
	}
	if after.IsError {
		t.Fatalf("the refused delete removed the record: %s", textOf(after))
	}
}

// TestMCPToolsRefuseUnknownField pins that the MCP surface runs the same decoder
// as HTTP: a field the declaration does not contain is refused, not stored.
func TestMCPToolsRefuseUnknownField(t *testing.T) {
	p := mountedMCP(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	client, closeFn := connect(t, srv)
	defer closeFn()

	res, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "octarq_data__create_tickets",
		Arguments: map[string]any{"title": "x", "org_id": 2},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !res.IsError {
		t.Fatalf("create accepted a reserved field: %s", textOf(res))
	}
	if !strings.Contains(textOf(res), "org_id") {
		t.Fatalf("refusal does not name the field: %s", textOf(res))
	}
}

// TestMCPListRefusesAPageOutsideTheBounds is the client-facing half: the bounds
// the list tool publishes in its schema are the ones a client is held to, so an
// agent that asks for 500 rows is told no instead of quietly reading 200.
func TestMCPListRefusesAPageOutsideTheBounds(t *testing.T) {
	p := mountedMCP(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	client, closeFn := connect(t, srv)
	defer closeFn()

	for _, args := range []map[string]any{
		{"limit": 0},
		{"limit": 201},
		{"limit": "50"},
		{"offset": -1},
	} {
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "octarq_data__list_tickets",
			Arguments: args,
		})
		if err != nil {
			t.Fatalf("list %v: %v", args, err)
		}
		if !res.IsError {
			t.Fatalf("list %v was served, want a refusal: %s", args, textOf(res))
		}
	}
}

// TestPageParamsRefusesAPageOutsideTheBounds covers the handler's own check,
// which runs behind the tool schema. A client that skips schema validation — or
// a future schema change that loosens it — must still be refused rather than
// served a page it did not ask for.
func TestPageParamsRefusesAPageOutsideTheBounds(t *testing.T) {
	cases := []struct {
		name    string
		in      toolArgs
		wantErr bool
	}{
		{"no arguments", toolArgs{}, false},
		{"valid page", toolArgs{"limit": float64(200), "offset": float64(10)}, false},
		{"limit zero", toolArgs{"limit": float64(0)}, true},
		{"limit above the cap", toolArgs{"limit": float64(201)}, true},
		{"limit is not a number", toolArgs{"limit": "50"}, true},
		{"limit is fractional", toolArgs{"limit": 1.5}, true},
		{"offset is negative", toolArgs{"offset": float64(-1)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, offset, err := pageParams(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("pageParams(%v) = %d/%d, want a refusal", tc.in, limit, offset)
				}
				if !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("pageParams(%v) = %v, want ErrInvalidArgument", tc.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("pageParams(%v) = %v", tc.in, err)
			}
			if limit < 1 || limit > maxPageSize || offset < 0 {
				t.Fatalf("pageParams(%v) = %d/%d, outside the bounds it enforces", tc.in, limit, offset)
			}
		})
	}
}

func textOf(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func resultString(t *testing.T, res *mcp.CallToolResult, key string) string {
	t.Helper()
	for _, line := range strings.Split(textOf(res), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `"`+key+`":`) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, `"`+key+`":`)), `",`)
		}
	}
	return ""
}

// TestReadyTellsAMissingRoleApartFromALowOne pins the three refusals apart. A
// transport that carries no role — stdio — cannot be fixed by changing the
// token, so it must not answer with the message that says it can.
func TestReadyTellsAMissingRoleApartFromALowOne(t *testing.T) {
	p := mountedMCP(t)
	e := p.entities[0]
	ctx := plugin.WithOrgID(context.Background(), 1)

	if err := p.ready(ctx, e, core.OpList); !errors.Is(err, ErrNoRole) {
		t.Fatalf("ready without a role = %v, want ErrNoRole", err)
	}
	if err := p.ready(plugin.WithTokenRole(ctx, "member"), e, core.OpList); err != nil {
		t.Fatalf("ready as member = %v, want nil", err)
	}
	if err := p.ready(plugin.WithTokenRole(ctx, "member"), e, core.OpDelete); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ready as member for delete = %v, want ErrForbidden", err)
	}
}

// TestInputSchemaLeavesAJSONFieldUntyped pins the schema to what the decoder
// accepts: a json field holds any JSON value, so a declared "object" would make
// the SDK refuse an array the server would store.
func TestInputSchemaLeavesAJSONFieldUntyped(t *testing.T) {
	doc := strings.Replace(ticketDoc,
		`{"name":"due_at","type":"timestamp"}`,
		`{"name":"due_at","type":"timestamp"},{"name":"extra","type":"json"}`, 1)
	p := mount(t, doc)

	props, ok := inputSchema(p.entities[0], core.OpCreate)["properties"].(map[string]any)
	if !ok {
		t.Fatal("create schema has no properties")
	}
	extra, ok := props["extra"].(map[string]any)
	if !ok {
		t.Fatalf("extra = %#v, want a property", props["extra"])
	}
	if typ, ok := extra["type"]; ok {
		t.Fatalf("extra advertises type %v, want none: the column stores any JSON value", typ)
	}
}

// TestMCPOwnerDeletesARecord pins the success path of the delete tool, and the
// not-found answer of get, update and delete once the record is gone: an agent
// asking for a record that no longer exists is told, not handed an empty object.
func TestMCPOwnerDeletesARecord(t *testing.T) {
	p := mountedMCP(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	p.RegisterMCP(srv)
	client, closeFn := connectAs(t, srv, "owner")
	defer closeFn()
	ctx := context.Background()

	created, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_data__create_tickets",
		Arguments: map[string]any{"title": "Printer is down"},
	})
	if err != nil || created.IsError {
		t.Fatalf("create = %v, %v", err, created)
	}
	id := resultString(t, created, "id")

	deleted, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "octarq_data__delete_tickets",
		Arguments: map[string]any{"id": id},
	})
	if err != nil || deleted.IsError {
		t.Fatalf("delete as owner = %v, %v", err, deleted)
	}

	for _, call := range []struct {
		tool string
		args map[string]any
	}{
		{"octarq_data__get_tickets", map[string]any{"id": id}},
		{"octarq_data__update_tickets", map[string]any{"id": id, "status": "closed"}},
		{"octarq_data__delete_tickets", map[string]any{"id": id}},
	} {
		res, err := client.CallTool(ctx, &mcp.CallToolParams{Name: call.tool, Arguments: call.args})
		if err != nil {
			t.Fatalf("%s: %v", call.tool, err)
		}
		if !res.IsError {
			t.Fatalf("%s on a deleted record = %s, want a refusal", call.tool, textOf(res))
		}
	}
}
