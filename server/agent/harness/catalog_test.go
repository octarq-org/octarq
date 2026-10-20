package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/plugin"
)

// catalogInput mirrors a real declarative endpoint input: json names plus the
// doc tags huma turns into schema descriptions.
type catalogInput struct {
	Slug        string   `json:"slug,omitempty" doc:"Custom short link slug"`
	Destination string   `json:"destination" doc:"Target destination URL"`
	Tags        []string `json:"tags,omitempty" doc:"Optional list of grouping tags"`
}

type catalogOutput struct {
	ID uint `json:"id"`
}

func catalogEndpoint(name, method, summary string) plugin.Endpoint {
	return plugin.EndpointSpec[catalogInput, catalogOutput]{
		Name:        name,
		Method:      method,
		Path:        "/api/" + name,
		Summary:     summary,
		Description: summary + " — longer description",
		Handler:     func(ctx context.Context, in catalogInput) (*catalogOutput, error) { return &catalogOutput{}, nil },
	}
}

type catalogSource struct{ eps []plugin.Endpoint }

func (s *catalogSource) Lookup(name string) (plugin.Endpoint, bool) {
	for _, ep := range s.eps {
		if ep.EndpointName() == name {
			return ep, true
		}
	}
	return nil, false
}

func (s *catalogSource) Endpoints() []plugin.Endpoint { return s.eps }

// specWithSteps builds a spec whose compiled prompt states the given step
// budget.
func specWithSteps(n int) harness.HarnessSpec {
	return harness.DefaultHarnessSpec().WithMaxSteps(n)
}

func TestBuildCatalog_ListsEndpointsSortedByName(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{
		catalogEndpoint("zeta_tool", "GET", "Zeta"),
		catalogEndpoint("alpha_tool", "GET", "Alpha"),
	}}

	catalog := harness.BuildCatalog(src, nil)

	if len(catalog.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(catalog.Tools))
	}
	if catalog.Tools[0].Name != "alpha_tool" || catalog.Tools[1].Name != "zeta_tool" {
		t.Fatalf("tools are not sorted by name: %q, %q", catalog.Tools[0].Name, catalog.Tools[1].Name)
	}
}

func TestBuildCatalog_SkipsNamelessEndpoints(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{
		catalogEndpoint("", "GET", "Anonymous"),
		catalogEndpoint("real_tool", "GET", "Real"),
	}}

	catalog := harness.BuildCatalog(src, nil)

	if len(catalog.Tools) != 1 || catalog.Tools[0].Name != "real_tool" {
		t.Fatalf("expected only the named tool, got %+v", catalog.Tools)
	}
}

func TestBuildCatalog_NilSourceYieldsEmptyCatalog(t *testing.T) {
	catalog := harness.BuildCatalog(nil, nil)
	if len(catalog.Tools) != 0 {
		t.Fatalf("expected empty catalog, got %d tools", len(catalog.Tools))
	}
}

func TestBuildCatalog_FilterExcludesTools(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{
		catalogEndpoint("read_tool", "GET", "Read"),
		catalogEndpoint("write_tool", "POST", "Write"),
	}}

	catalog := harness.BuildCatalog(src, func(d harness.ToolDescriptor) bool {
		return d.Method == "GET"
	})

	if len(catalog.Tools) != 1 || catalog.Tools[0].Name != "read_tool" {
		t.Fatalf("filter did not apply: %+v", catalog.Tools)
	}
}

func TestBuildCatalog_DescribesParameters(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("create_link", "POST", "Create")}}

	catalog := harness.BuildCatalog(src, nil)
	if len(catalog.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(catalog.Tools))
	}

	params := map[string]harness.ParameterDescriptor{}
	for _, p := range catalog.Tools[0].Parameters {
		params[p.Name] = p
	}

	dest, ok := params["destination"]
	if !ok {
		t.Fatalf("destination parameter missing: %+v", params)
	}
	if !dest.Required {
		t.Error("destination must be required")
	}
	if dest.Type != "string" {
		t.Errorf("destination type = %q, want string", dest.Type)
	}
	if !strings.Contains(dest.Description, "Target destination URL") {
		t.Errorf("destination description = %q, want the doc tag text", dest.Description)
	}

	slug, ok := params["slug"]
	if !ok {
		t.Fatalf("slug parameter missing: %+v", params)
	}
	if slug.Required {
		t.Error("slug must be optional")
	}

	tags, ok := params["tags"]
	if !ok {
		t.Fatalf("tags parameter missing: %+v", params)
	}
	if !strings.HasPrefix(tags.Type, "array") {
		t.Errorf("tags type = %q, want an array type", tags.Type)
	}
}

func TestBuildCatalog_CapturesRiskAndApproval(t *testing.T) {
	ep := plugin.EndpointSpec[catalogInput, catalogOutput]{
		Name:            "danger_tool",
		Method:          "DELETE",
		Path:            "/api/danger",
		Summary:         "Danger",
		RiskLevel:       plugin.RiskLevelDestructive,
		RequireApproval: true,
		Handler:         func(ctx context.Context, in catalogInput) (*catalogOutput, error) { return &catalogOutput{}, nil },
	}

	catalog := harness.BuildCatalog(&catalogSource{eps: []plugin.Endpoint{ep}}, nil)
	if len(catalog.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(catalog.Tools))
	}
	if catalog.Tools[0].Risk != plugin.RiskLevelDestructive {
		t.Errorf("risk = %q, want %q", catalog.Tools[0].Risk, plugin.RiskLevelDestructive)
	}
	if !catalog.Tools[0].RequiresApproval {
		t.Error("approval requirement was lost")
	}
}

func TestSystemPrompt_DocumentsTheToolCallFormat(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("create_link", "POST", "Create")}}
	prompt := harness.BuildCatalog(src, nil).SystemPrompt(specWithSteps(8))

	if !strings.Contains(prompt, "```tool_call") {
		t.Errorf("prompt does not show the tool_call fence:\n%s", prompt)
	}
	if !strings.Contains(prompt, `"args": {`) {
		t.Errorf("prompt does not show args as an object:\n%s", prompt)
	}
}

func TestSystemPrompt_DocumentsTheUntrustedFence(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("create_link", "POST", "Create")}}
	prompt := harness.BuildCatalog(src, nil).SystemPrompt(specWithSteps(8))

	if !strings.Contains(prompt, "```untrusted") {
		t.Errorf("prompt does not explain the untrusted fence:\n%s", prompt)
	}
	if !strings.Contains(prompt, "DATA") {
		t.Errorf("prompt does not state that tool output is data:\n%s", prompt)
	}
}

func TestSystemPrompt_ListsToolGuidance(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("create_link", "POST", "Create")}}
	prompt := harness.BuildCatalog(src, nil).SystemPrompt(specWithSteps(8))

	for _, want := range []string{
		"create_link",
		"Create",
		"destination",
		"Target destination URL",
		"required",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, prompt)
		}
	}
}

func TestSystemPrompt_MentionsTheStepBudget(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("create_link", "POST", "Create")}}
	prompt := harness.BuildCatalog(src, nil).SystemPrompt(specWithSteps(3))

	if !strings.Contains(prompt, "3") {
		t.Errorf("prompt does not mention the step budget:\n%s", prompt)
	}
}

func TestSystemPrompt_EmptyCatalogStatesNoTools(t *testing.T) {
	prompt := harness.BuildCatalog(nil, nil).SystemPrompt(specWithSteps(8))

	if !strings.Contains(prompt, "no tools") && !strings.Contains(prompt, "No tools") {
		t.Errorf("prompt does not say that no tools are available:\n%s", prompt)
	}
	if strings.Contains(prompt, "```tool_call") {
		t.Errorf("prompt invites tool calls although none exist:\n%s", prompt)
	}
}

func TestSystemPrompt_FingerprintIsStableAndTracksTheToolSet(t *testing.T) {
	one := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("create_link", "POST", "Create")}}
	two := &catalogSource{eps: []plugin.Endpoint{
		catalogEndpoint("create_link", "POST", "Create"),
		catalogEndpoint("read_links", "GET", "Read"),
	}}

	a := harness.BuildCatalog(one, nil).Fingerprint(specWithSteps(8))
	b := harness.BuildCatalog(one, nil).Fingerprint(specWithSteps(8))
	c := harness.BuildCatalog(two, nil).Fingerprint(specWithSteps(8))

	if a == "" {
		t.Fatal("fingerprint is empty")
	}
	if a != b {
		t.Error("fingerprint is not stable for the same catalog")
	}
	if a == c {
		t.Error("fingerprint did not change when the tool set changed")
	}
}
