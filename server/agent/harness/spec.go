package harness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// A HarnessSpec is the versioned description of the shell around the model:
// what the agent is told, which tools it is offered, and how many steps it may
// take. It is the unit that gets published, rolled back and compared.
//
// Only the components that actually change behaviour today are represented.
// A slot that does not exist cannot be set to a value that is silently
// ignored — the validator rejects it, so an operator never believes a knob
// is in effect when it is not.
//
// The components of a harness that octarq does not implement yet (long-term
// memory, subagents, skills, helper scripts, loop middleware) are listed in
// server/agent/harness/SPEC.md. They are deliberately absent here rather than
// present-and-ignored.

// HarnessSpecVersion is the version of the spec document format. It changes
// when the meaning of a field changes, not when a spec is edited.
const HarnessSpecVersion = 1

// Limits enforced by Validate. They exist so a spec cannot be used to smuggle
// an unbounded prompt or an unbounded tool list into every turn.
const (
	MaxSpecNameLen  = 64
	MaxPreambleLen  = 2000
	MaxExtraRules   = 20
	MaxExtraRuleLen = 500
	MaxToolAllowLen = 200
	MaxToolNameLen  = 64
	MinSpecMaxSteps = 1
	// MaxSpecMaxSteps is the highest step budget a spec may ask for: the
	// ceiling of the most generous profile (MaxStepsNormal). A value above it
	// would be clamped at run time, so accepting it would store a number the
	// agent never honours — the operator would read the spec, believe the
	// budget was 32, and watch the agent stop at 8.
	MaxSpecMaxSteps = MaxStepsNormal
	// DefaultMaxStepsFor is the budget a spec gets when it asks for none.
	DefaultMaxStepsFor = MaxStepsNormal
)

// HarnessSpec is the published description of an agent's shell.
type HarnessSpec struct {
	// Version is the spec document version. Only HarnessSpecVersion is valid.
	Version int `json:"version"`
	// Name labels the spec for operators ("default", "support-bot", ...).
	Name string `json:"name"`
	// Preamble is the opening of the system prompt: the agent's role and the
	// operator context. Empty means the built-in preamble.
	Preamble string `json:"preamble,omitempty"`
	// ExtraRules are appended after the built-in rules. They cannot replace a
	// built-in rule — the built-in ones always come first and cannot be
	// removed.
	ExtraRules []string `json:"extraRules,omitempty"`
	// ToolAllow is the allowlist of tool names advertised to the model. Empty
	// means every tool the session may call.
	ToolAllow []string `json:"toolAllow,omitempty"`
	// MaxSteps is the step budget for a turn. Zero means the profile default.
	MaxSteps int `json:"maxSteps,omitempty"`
}

// DefaultHarnessSpec is the built-in spec: the stock preamble, no extra rules,
// every tool, and the normal profile's step budget.
func DefaultHarnessSpec() HarnessSpec {
	return HarnessSpec{
		Version:  HarnessSpecVersion,
		Name:     "default",
		MaxSteps: 0, // the profile decides
	}
}

// SpecViolation is one reason a spec was refused.
type SpecViolation struct {
	Field   string
	Message string
}

func (v SpecViolation) Error() string {
	return fmt.Sprintf("%s: %s", v.Field, v.Message)
}

// SpecValidationError lists every violation of a spec. It is returned
// instead of the spec, so a caller can never compile a partially valid spec.
type SpecValidationError struct {
	Violations []SpecViolation
}

func (e *SpecValidationError) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.Error())
	}
	return "invalid harness spec: " + strings.Join(parts, "; ")
}

// Validate checks a spec. It fails closed: a spec with any violation is
// refused as a whole, never partially applied.
func Validate(spec HarnessSpec) error {
	var vs []SpecViolation

	if spec.Version != HarnessSpecVersion {
		vs = append(vs, SpecViolation{Field: "version", Message: fmt.Sprintf("must be %d, got %d", HarnessSpecVersion, spec.Version)})
	}
	if strings.TrimSpace(spec.Name) == "" {
		vs = append(vs, SpecViolation{Field: "name", Message: "must not be empty"})
	} else if len(spec.Name) > MaxSpecNameLen {
		vs = append(vs, SpecViolation{Field: "name", Message: fmt.Sprintf("must be at most %d characters", MaxSpecNameLen)})
	}
	if len(spec.Preamble) > MaxPreambleLen {
		vs = append(vs, SpecViolation{Field: "preamble", Message: fmt.Sprintf("must be at most %d characters", MaxPreambleLen)})
	}
	if len(spec.ExtraRules) > MaxExtraRules {
		vs = append(vs, SpecViolation{Field: "extraRules", Message: fmt.Sprintf("must hold at most %d rules", MaxExtraRules)})
	}
	for i, rule := range spec.ExtraRules {
		if strings.TrimSpace(rule) == "" {
			vs = append(vs, SpecViolation{Field: fmt.Sprintf("extraRules[%d]", i), Message: "must not be empty"})
			continue
		}
		if len(rule) > MaxExtraRuleLen {
			vs = append(vs, SpecViolation{Field: fmt.Sprintf("extraRules[%d]", i), Message: fmt.Sprintf("must be at most %d characters", MaxExtraRuleLen)})
		}
	}
	if len(spec.ToolAllow) > MaxToolAllowLen {
		vs = append(vs, SpecViolation{Field: "toolAllow", Message: fmt.Sprintf("must hold at most %d names", MaxToolAllowLen)})
	}
	seen := make(map[string]bool, len(spec.ToolAllow))
	for i, name := range spec.ToolAllow {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			vs = append(vs, SpecViolation{Field: fmt.Sprintf("toolAllow[%d]", i), Message: "must not be empty"})
			continue
		}
		if len(trimmed) > MaxToolNameLen {
			vs = append(vs, SpecViolation{Field: fmt.Sprintf("toolAllow[%d]", i), Message: fmt.Sprintf("must be at most %d characters", MaxToolNameLen)})
		}
		if strings.ContainsAny(trimmed, " \t\n") {
			vs = append(vs, SpecViolation{Field: fmt.Sprintf("toolAllow[%d]", i), Message: "must be a single tool name"})
		}
		if seen[trimmed] {
			vs = append(vs, SpecViolation{Field: fmt.Sprintf("toolAllow[%d]", i), Message: fmt.Sprintf("duplicates %q", trimmed)})
		}
		seen[trimmed] = true
	}
	if spec.MaxSteps != 0 && (spec.MaxSteps < MinSpecMaxSteps || spec.MaxSteps > MaxSpecMaxSteps) {
		vs = append(vs, SpecViolation{Field: "maxSteps", Message: fmt.Sprintf("must be 0 or between %d and %d", MinSpecMaxSteps, MaxSpecMaxSteps)})
	}

	if len(vs) > 0 {
		return &SpecValidationError{Violations: vs}
	}
	return nil
}

// ToolFilter returns the filter the catalog must apply for this spec: the
// allowlist of tool names, or nil when the spec does not restrict them.
//
// The filter is applied to the tools the session may call, so a spec can
// narrow the advertised surface but never widen it past the session policy.
func (s HarnessSpec) ToolFilter() ToolFilter {
	if len(s.ToolAllow) == 0 {
		return nil
	}
	allow := make(map[string]bool, len(s.ToolAllow))
	for _, name := range s.ToolAllow {
		allow[strings.TrimSpace(name)] = true
	}
	return func(d ToolDescriptor) bool {
		return allow[d.Name]
	}
}

// EffectiveMaxSteps returns the step budget for a turn. The spec may only
// tighten the profile's ceiling: raising it is a safety invariant the
// profile owns, not something a published spec may do.
func (s HarnessSpec) EffectiveMaxSteps(profileDefault int) int {
	if s.MaxSteps > 0 && s.MaxSteps < profileDefault {
		return s.MaxSteps
	}
	return profileDefault
}

// WithMaxSteps returns a copy of the spec with the step budget set. It is
// used to write the effective budget into the compiled prompt, so the model
// is told the ceiling the runner will actually enforce.
func (s HarnessSpec) WithMaxSteps(n int) HarnessSpec {
	s.MaxSteps = n
	return s
}

// Fingerprint identifies the spec's content. Two specs with the same
// fingerprint compile the same prompt for the same tool surface, which is
// what makes recorded turns comparable across harness versions.
func (s HarnessSpec) Fingerprint() string {
	sum := sha256.Sum256([]byte(s.CanonicalJSON()))
	return hex.EncodeToString(sum[:])
}

// CanonicalJSON renders the spec in a stable order, so the fingerprint does
// not depend on map iteration or field order.
func (s HarnessSpec) CanonicalJSON() string {
	type canonical struct {
		Version    int      `json:"version"`
		Name       string   `json:"name"`
		Preamble   string   `json:"preamble"`
		ExtraRules []string `json:"extraRules"`
		ToolAllow  []string `json:"toolAllow"`
		MaxSteps   int      `json:"maxSteps"`
	}
	c := canonical{
		Version:    s.Version,
		Name:       s.Name,
		Preamble:   s.Preamble,
		ExtraRules: append([]string{}, s.ExtraRules...),
		ToolAllow:  append([]string{}, s.ToolAllow...),
		MaxSteps:   s.MaxSteps,
	}
	if c.ExtraRules == nil {
		c.ExtraRules = []string{}
	}
	if c.ToolAllow == nil {
		c.ToolAllow = []string{}
	}
	sort.Strings(c.ExtraRules)
	sort.Strings(c.ToolAllow)
	raw, err := json.Marshal(c)
	if err != nil {
		// The canonical struct cannot fail to marshal.
		return ""
	}
	return string(raw)
}

// DecodeSpec decodes a spec document. Unknown fields are rejected: a spec
// that sets a component octarq does not implement must fail loudly, not be
// silently ignored.
//
// Content after the document is rejected too. A file holding two JSON objects
// would otherwise decode as the first one, and the rest of the file — a
// hand-edited second version, a second candidate — would vanish without a
// word.
func DecodeSpec(raw []byte) (HarnessSpec, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var spec HarnessSpec
	if err := dec.Decode(&spec); err != nil {
		return HarnessSpec{}, fmt.Errorf("harness: decode spec: %w", err)
	}
	if dec.More() {
		return HarnessSpec{}, errors.New("harness: decode spec: the document has content after the spec")
	}
	if err := Validate(spec); err != nil {
		return HarnessSpec{}, err
	}
	return spec, nil
}
