package ontology

import (
	"context"
	"errors"
	"strings"
	"testing"

	core "github.com/octarq-org/octarq/server/internal/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// ticketDoc is the acceptance declaration: a ticket with a unique title, a
// status, a priority and a due date.
const ticketDoc = `{"entities":[{
  "slug":"tickets","label":"Ticket","pluralLabel":"Tickets","icon":"ticket",
  "fields":[
    {"name":"title","type":"string","required":true,"unique":true,"listColumn":true},
    {"name":"status","type":"string","default":"open"},
    {"name":"priority","type":"integer","default":"2"},
    {"name":"due_at","type":"timestamp"}
  ],
  "operations":{
    "list":{"roles":["member"],"exposeMcp":true},
    "get":{"roles":["member"],"exposeMcp":true},
    "create":{"roles":["member"],"exposeMcp":true},
    "update":{"roles":["member"],"exposeMcp":true},
    "delete":{"roles":["admin"],"exposeMcp":true}
  }
}]}`

func mount(t *testing.T, doc string) *Plugin {
	t.Helper()
	t.Setenv(EntitiesEnv, doc)
	t.Setenv(EntitiesFileEnv, "")
	p := New()
	// A mounted plugin publishes menus only when it has a database to apply the
	// declaration to, so the fixture supplies the one a booted server has.
	p.Mount(nil, &plugin.Context{DB: openDB(t)})
	return p
}

// TestMountWithoutDeclarationsPublishesNothing pins the off switch: an
// installation with no declaration file is valid, and nothing appears in the
// sidebar or on the API.
func TestMountWithoutDeclarationsPublishesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EntitiesEnv, "")
	t.Setenv(EntitiesFileEnv, "")

	p := New()
	p.Mount(nil, &plugin.Context{})

	if err := p.Validate(context.Background(), nil); err != nil {
		t.Fatalf("Validate = %v, want nil: no declaration is a valid installation", err)
	}
	if len(p.Menus()) != 0 {
		t.Fatalf("menus = %v, want none", p.Menus())
	}
}

func TestMountRefusesUnparsableDeclaration(t *testing.T) {
	p := mount(t, `{"entities":[{"slug":"tickets","label":"Ticket","fields":[{"name":"title","type":"strin"}]}]}`)
	if err := p.Validate(context.Background(), nil); err == nil {
		t.Fatal("Validate = nil, want the unknown field type refused")
	}
	if len(p.Menus()) != 0 {
		t.Fatal("a refused declaration must publish no menus")
	}
}

func TestMountRefusesUnknownIcon(t *testing.T) {
	doc := strings.Replace(ticketDoc, `"icon":"ticket"`, `"icon":"tabel"`, 1)
	p := mount(t, doc)
	err := p.Validate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "sidebar icon key") {
		t.Fatalf("Validate = %v, want an icon refusal", err)
	}
}

// TestToolNamesFitProtocolLimit pins the arithmetic behind toolPrefix. MCP
// clients reject a tool name longer than 64 bytes, and the longest name comes
// from the longest slug the pattern allows.
func TestToolNamesFitProtocolLimit(t *testing.T) {
	longest := "a" + strings.Repeat("0", 40)
	if !core.SlugPattern.MatchString(longest) || len(longest) != 41 {
		t.Fatalf("longest slug assumption is wrong: %q", longest)
	}
	for _, op := range core.Operations {
		name := mcpToolName(longest, op)
		if len(name) > 64 {
			t.Fatalf("tool name %q is %d bytes, the protocol limit is 64", name, len(name))
		}
	}
}

func TestMenusPublishOneEntryPerEntity(t *testing.T) {
	p := mount(t, ticketDoc)
	menus := p.Menus()
	if len(menus) != 1 {
		t.Fatalf("menus = %v, want one", menus)
	}
	got := menus[0]
	if got.ID != "data-tickets" || got.Path != "/data/tickets" || got.Label != "Tickets" || got.Icon != "ticket" {
		t.Fatalf("menu = %+v", got)
	}
	if got.Category != "Workspace" || got.Area != "operations" {
		t.Fatalf("menu placement = %+v", got)
	}
	// Every operation but delete grants member, so the entry stays visible.
	if got.RequiredRole != "" {
		t.Fatalf("RequiredRole = %q, want empty for a member-visible entity", got.RequiredRole)
	}
}

func TestMenuHidesWhenListingNeedsAdmin(t *testing.T) {
	doc := strings.Replace(ticketDoc, `"list":{"roles":["member"]`, `"list":{"roles":["admin"]`, 1)
	p := mount(t, doc)
	menus := p.Menus()
	if len(menus) != 1 || menus[0].RequiredRole != "admin" {
		t.Fatalf("menus = %+v, want RequiredRole admin", menus)
	}
}

func TestACLAllows(t *testing.T) {
	cases := []struct {
		name  string
		roles []string
		role  string
		want  bool
	}{
		{"member granted member", []string{"member"}, "member", true},
		{"admin granted member", []string{"member"}, "admin", true},
		{"owner granted member", []string{"member"}, "owner", true},
		{"member refused admin", []string{"admin"}, "member", false},
		{"admin granted admin", []string{"admin"}, "admin", true},
		{"no role refused", []string{"member"}, "", false},
		{"unknown role refused", []string{"member"}, "superuser", false},
		{"wide grant takes the lowest", []string{"owner", "member"}, "member", true},
		{"empty acl refused", nil, "owner", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aclAllows(tc.roles, tc.role); got != tc.want {
				t.Fatalf("aclAllows(%v, %q) = %v, want %v", tc.roles, tc.role, got, tc.want)
			}
		})
	}
}

func TestMCPTierTakesTheHighestDeclared(t *testing.T) {
	doc := strings.Replace(ticketDoc, `"get":{"roles":["member"],"exposeMcp":true}`, `"get":{"roles":["member"],"exposeMcp":true,"tier":"pro"}`, 1)
	p := mount(t, doc)
	if got := p.MCPTier(); got != "pro" {
		t.Fatalf("MCPTier = %q, want pro", got)
	}
	free := mount(t, ticketDoc)
	if got := free.MCPTier(); got != "" {
		t.Fatalf("MCPTier = %q, want free", got)
	}
}

// TestMCPTierIgnoresOperationsNotExposed pins that a tier on a hidden operation
// does not license-gate the visible ones.
func TestMCPTierIgnoresOperationsNotExposed(t *testing.T) {
	doc := strings.Replace(ticketDoc, `"delete":{"roles":["admin"],"exposeMcp":true}`, `"delete":{"roles":["admin"],"exposeMcp":false,"tier":"elite"}`, 1)
	p := mount(t, doc)
	if got := p.MCPTier(); got != "" {
		t.Fatalf("MCPTier = %q, want free", got)
	}
}

func TestTierRankOrdersUnknownHighest(t *testing.T) {
	if tierRank("") != 0 || tierRank("pro") != 1 || tierRank("elite") != 2 {
		t.Fatal("known tiers must keep their order")
	}
	if tierRank("typo") <= tierRank("elite") {
		t.Fatal("an unknown tier must outrank every known one, so a typo hides the group")
	}
}

func TestParseSortAcceptsPlatformTimestamps(t *testing.T) {
	p := mount(t, ticketDoc)
	e := p.entities[0]

	got, err := parseSort(e, "-created_at,title")
	if err != nil {
		t.Fatalf("parseSort: %v", err)
	}
	if strings.Join(got, ",") != "-created_at,title" {
		t.Fatalf("sort = %v", got)
	}
	if _, err := parseSort(e, "nope"); err == nil {
		t.Fatal("parseSort accepted an undeclared field")
	}
	if _, err := parseSort(e, "org_id"); err == nil {
		t.Fatal("parseSort accepted the tenant column")
	}
}

func TestInputSchemaForUpdateRequiresID(t *testing.T) {
	p := mount(t, ticketDoc)
	e := p.entities[0]
	schema := inputSchema(e, core.OpUpdate)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["id"]; !ok {
		t.Fatalf("update schema has no id property: %v", props)
	}
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "id" {
		t.Fatalf("update required = %v, want [id]: an update names its target and nothing else", required)
	}
	if _, ok := props["title"]; !ok {
		t.Fatalf("update schema lost the declared fields: %v", props)
	}
}

func TestInputSchemaDefaultsAreTyped(t *testing.T) {
	p := mount(t, ticketDoc)
	e := p.entities[0]
	props, _ := inputSchema(e, core.OpCreate)["properties"].(map[string]any)
	priority, _ := props["priority"].(map[string]any)
	if _, ok := priority["default"].(int64); !ok {
		t.Fatalf("priority default = %#v, want a JSON number: a string default panics the MCP server", priority["default"])
	}
}

// TestValidateReadsTheDeclarationBeforeAnyMount pins the order `octarq mcp`
// uses: it validates first, then mounts. A declaration only Mount read would
// never be checked on that path, and the CLI would start with no tool, no
// message and exit status 0.
func TestValidateReadsTheDeclarationBeforeAnyMount(t *testing.T) {
	t.Setenv(EntitiesEnv, `{"entities":[{"slug":"tickets","label":"Ticket","fields":[{"name":"title","type":"strin"}]}]}`)
	t.Setenv(EntitiesFileEnv, "")

	p := New()
	if err := p.Validate(context.Background(), nil); err == nil {
		t.Fatal("Validate = nil, want the declaration refused before the mount")
	}
}

// TestMenusStayHiddenWhenTheSchemaWasNotApplied pins the sidebar to the routes:
// a composition that applied nothing publishes no menu, because the page the
// entry opens does not exist.
func TestMenusStayHiddenWhenTheSchemaWasNotApplied(t *testing.T) {
	t.Setenv(EntitiesEnv, ticketDoc)
	t.Setenv(EntitiesFileEnv, "")

	p := New()
	p.Mount(nil, &plugin.Context{})
	if !errors.Is(p.schemaErr, ErrNoDatabase) {
		t.Fatalf("schemaErr = %v, want ErrNoDatabase", p.schemaErr)
	}
	if len(p.Menus()) != 0 {
		t.Fatalf("menus = %v, want none without an applied schema", p.Menus())
	}
}
