package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/plugin"
)

// A caller that composes its own preamble needs the tool list on its own.
// The section must be rendered by the same code as the full prompt, and it
// must say so when there is nothing to call — never an empty section, which
// would read as "no tools are needed".
func TestToolSection_RendersTheToolList(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("read_things", "GET", "Read")}}

	section := harness.BuildCatalog(src, nil).ToolSection()

	if !strings.Contains(section, "## Available tools") {
		t.Errorf("the section has no heading:\n%s", section)
	}
	if !strings.Contains(section, "read_things") {
		t.Errorf("the tool is missing:\n%s", section)
	}
}

func TestToolSection_AppearsInTheFullPrompt(t *testing.T) {
	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("read_things", "GET", "Read")}}
	catalog := harness.BuildCatalog(src, nil)

	section := catalog.ToolSection()
	prompt := catalog.SystemPrompt(specWithSteps(8))

	if !strings.Contains(prompt, section) {
		t.Errorf("the section is not part of the prompt:\n%s", prompt)
	}
}

func TestToolSection_SaysWhenThereAreNoTools(t *testing.T) {
	section := harness.BuildCatalog(nil, nil).ToolSection()

	if !strings.Contains(section, "## Available tools") {
		t.Errorf("the empty section has no heading:\n%s", section)
	}
	if !strings.Contains(section, "No tools are available") {
		t.Errorf("the empty section does not say that no tool can be called:\n%s", section)
	}
}

func TestToolSection_MarksApprovalGatedTools(t *testing.T) {
	ep := plugin.EndpointSpec[catalogInput, catalogOutput]{
		Name:            "write_things",
		Method:          "POST",
		Path:            "/api/write_things",
		Summary:         "Write",
		RequireApproval: true,
		Handler:         func(ctx context.Context, in catalogInput) (*catalogOutput, error) { return &catalogOutput{}, nil },
	}

	section := harness.BuildCatalog(&catalogSource{eps: []plugin.Endpoint{ep}}, nil).ToolSection()

	if !strings.Contains(section, "needs approval") {
		t.Errorf("the approval requirement is missing:\n%s", section)
	}
}
