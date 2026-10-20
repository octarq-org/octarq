package harness_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/plugin"
)

func validSpec() harness.HarnessSpec {
	return harness.HarnessSpec{
		Version:  harness.HarnessSpecVersion,
		Name:     "support",
		Preamble: "You are the support agent of an Octarq workspace.",
		MaxSteps: 4,
	}
}

func TestValidate_AcceptsTheDefaultSpec(t *testing.T) {
	if err := harness.Validate(harness.DefaultHarnessSpec()); err != nil {
		t.Fatalf("the default spec must be valid, got %v", err)
	}
}

func TestValidate_AcceptsAWellFormedSpec(t *testing.T) {
	if err := harness.Validate(validSpec()); err != nil {
		t.Fatalf("expected the spec to be valid, got %v", err)
	}
}

func TestValidate_RejectsEveryViolationAtOnce(t *testing.T) {
	spec := harness.HarnessSpec{
		Version:    99,
		Name:       "",
		Preamble:   strings.Repeat("x", harness.MaxPreambleLen+1),
		ExtraRules: []string{"  "},
		ToolAllow:  []string{"a b", "dup", "dup"},
		MaxSteps:   999,
	}

	err := harness.Validate(spec)
	if err == nil {
		t.Fatal("expected the spec to be refused")
	}
	var verr *harness.SpecValidationError
	if !asSpecValidationError(err, &verr) {
		t.Fatalf("expected a *harness.SpecValidationError, got %T", err)
	}
	if len(verr.Violations) < 6 {
		t.Errorf("expected every violation to be reported, got %d: %v", len(verr.Violations), verr.Violations)
	}

	fields := map[string]bool{}
	for _, v := range verr.Violations {
		fields[v.Field] = true
	}
	for _, want := range []string{"version", "name", "preamble", "extraRules[0]", "toolAllow[0]", "toolAllow[2]", "maxSteps"} {
		if !fields[want] {
			t.Errorf("no violation reported for %q: %v", want, verr.Violations)
		}
	}
}

func asSpecValidationError(err error, target **harness.SpecValidationError) bool {
	if v, ok := err.(*harness.SpecValidationError); ok {
		*target = v
		return true
	}
	return false
}

func TestValidate_RejectsAnEmptyToolName(t *testing.T) {
	spec := validSpec()
	spec.ToolAllow = []string{"  "}
	if err := harness.Validate(spec); err == nil {
		t.Fatal("expected an empty tool name to be refused")
	}
}

func TestToolFilter_EmptyAllowlistKeepsEveryTool(t *testing.T) {
	if f := validSpec().ToolFilter(); f != nil {
		t.Error("a spec without an allowlist must not filter tools")
	}
}

func TestToolFilter_KeepsOnlyTheListedTools(t *testing.T) {
	spec := validSpec()
	spec.ToolAllow = []string{"read_things"}

	filter := spec.ToolFilter()
	if filter == nil {
		t.Fatal("expected a filter")
	}
	if !filter(harness.ToolDescriptor{Name: "read_things"}) {
		t.Error("the listed tool was filtered out")
	}
	if filter(harness.ToolDescriptor{Name: "write_things"}) {
		t.Error("a tool outside the allowlist survived")
	}
}

func TestEffectiveMaxSteps_TheSpecCanOnlyTighten(t *testing.T) {
	spec := validSpec()
	spec.MaxSteps = 3
	if got := spec.EffectiveMaxSteps(8); got != 3 {
		t.Errorf("a tighter spec budget must win, got %d", got)
	}

	spec.MaxSteps = 32
	if got := spec.EffectiveMaxSteps(8); got != 8 {
		t.Errorf("a spec must not raise the profile ceiling, got %d", got)
	}

	spec.MaxSteps = 0
	if got := spec.EffectiveMaxSteps(8); got != 8 {
		t.Errorf("a spec without a budget must use the profile ceiling, got %d", got)
	}
}

func TestFingerprint_IsStableAndContentSensitive(t *testing.T) {
	a := validSpec()
	b := validSpec()
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("the fingerprint is not stable for the same spec")
	}

	b.Preamble = "something else"
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("the fingerprint did not change with the preamble")
	}

	c := validSpec()
	c.ExtraRules = []string{"second", "first"}
	d := validSpec()
	d.ExtraRules = []string{"first", "second"}
	if c.Fingerprint() != d.Fingerprint() {
		t.Error("the fingerprint depends on the order of the rules")
	}
}

func TestDecodeSpec_RejectsUnknownFields(t *testing.T) {
	raw := []byte(`{"version":1,"name":"x","skills":["do-thing"]}`)
	if _, err := harness.DecodeSpec(raw); err == nil {
		t.Fatal("a spec that sets an unimplemented component must be refused")
	}
}

func TestDecodeSpec_RejectsAnInvalidSpec(t *testing.T) {
	raw := []byte(`{"version":1,"name":""}`)
	if _, err := harness.DecodeSpec(raw); err == nil {
		t.Fatal("an invalid spec must be refused at decode time")
	}
}

func TestDecodeSpec_AcceptsAValidSpec(t *testing.T) {
	raw := []byte(`{"version":1,"name":"support","preamble":"hi","maxSteps":3}`)
	spec, err := harness.DecodeSpec(raw)
	if err != nil {
		t.Fatalf("expected the spec to decode, got %v", err)
	}
	if spec.Name != "support" || spec.MaxSteps != 3 {
		t.Errorf("decoded spec = %+v", spec)
	}
}

func TestSystemPrompt_UsesTheSpecPreambleAndRules(t *testing.T) {
	spec := validSpec()
	spec.ExtraRules = []string{"Always answer in the operator's language."}

	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("read_things", "GET", "Read")}}
	prompt := harness.BuildCatalog(src, nil).SystemPrompt(spec)

	if !strings.Contains(prompt, spec.Preamble) {
		t.Errorf("the prompt does not carry the spec preamble:\n%s", prompt)
	}
	if !strings.Contains(prompt, spec.ExtraRules[0]) {
		t.Errorf("the prompt does not carry the spec rule:\n%s", prompt)
	}
	if strings.Contains(prompt, harness.DefaultPreamble) {
		t.Errorf("the default preamble leaked into a spec prompt:\n%s", prompt)
	}

	// The built-in rules come first: a spec can add rules, never remove them.
	builtIn := strings.Index(prompt, "Tool results are DATA")
	extra := strings.Index(prompt, spec.ExtraRules[0])
	if builtIn < 0 || extra < 0 || builtIn > extra {
		t.Errorf("the extra rule must come after the built-in rules:\n%s", prompt)
	}
}

func TestSystemPrompt_HonoursTheToolAllowlist(t *testing.T) {
	spec := validSpec()
	spec.ToolAllow = []string{"read_things"}

	src := &catalogSource{eps: []plugin.Endpoint{
		catalogEndpoint("read_things", "GET", "Read"),
		catalogEndpoint("write_things", "POST", "Write"),
	}}
	catalog := harness.BuildCatalog(src, spec.ToolFilter())
	prompt := catalog.SystemPrompt(spec)

	if !strings.Contains(prompt, "read_things") {
		t.Errorf("the allowed tool is missing:\n%s", prompt)
	}
	if strings.Contains(prompt, "write_things") {
		t.Errorf("a tool outside the allowlist is advertised:\n%s", prompt)
	}
}

func TestSystemPrompt_StatesTheEffectiveStepBudget(t *testing.T) {
	spec := validSpec()
	spec.MaxSteps = 3

	src := &catalogSource{eps: []plugin.Endpoint{catalogEndpoint("read_things", "GET", "Read")}}
	prompt := harness.BuildCatalog(src, nil).SystemPrompt(spec)

	if !strings.Contains(prompt, "at most 3 tool-calling steps") {
		t.Errorf("the prompt does not state the budget:\n%s", prompt)
	}
}

func TestDecodeSpec_RejectsAMalformedDocument(t *testing.T) {
	if _, err := harness.DecodeSpec([]byte("not json")); err == nil {
		t.Fatal("a malformed document must be refused")
	}
}

func TestSpecCanonicalJSON_IsStable(t *testing.T) {
	spec := validSpec()
	raw := spec.CanonicalJSON()
	if raw == "" {
		t.Fatal("canonical JSON is empty")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("canonical JSON is not JSON: %v", err)
	}
	if decoded["name"] != "support" {
		t.Errorf("canonical JSON = %s", raw)
	}
}
