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

const HarnessSpecVersion = 1

const (
	MaxSpecNameLen  = 64
	MaxPreambleLen  = 2000
	MaxExtraRules   = 20
	MaxExtraRuleLen = 500
	MaxToolAllowLen = 200
	MaxToolNameLen  = 64
	MinSpecMaxSteps = 1
	MaxSpecMaxSteps = MaxStepsWorker
)

type HarnessSpec struct {
	Version        int      `json:"version"`
	Name           string   `json:"name"`
	Preamble       string   `json:"preamble,omitempty"`
	ExtraRules     []string `json:"extraRules,omitempty"`
	ToolAllow      []string `json:"toolAllow,omitempty"`
	Profile        Profile  `json:"profile,omitempty"`
	MaxSteps       int      `json:"maxSteps,omitempty"`
	MaxTokens      int      `json:"maxTokens,omitempty"`
	MaxCostUSD     float64  `json:"maxCostUSD,omitempty"`
	MaxWallTimeSec int      `json:"maxWallTimeSec,omitempty"`
}

func DefaultHarnessSpec() HarnessSpec {
	return HarnessSpec{
		Version:  HarnessSpecVersion,
		Name:     "default",
		MaxSteps: 0,
	}
}

type SpecViolation struct {
	Field   string
	Message string
}

func (v SpecViolation) Error() string {
	return fmt.Sprintf("%s: %s", v.Field, v.Message)
}

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
	if spec.Profile < ProfileNormal || spec.Profile > ProfileWorker {
		vs = append(vs, SpecViolation{Field: "profile", Message: fmt.Sprintf("must be between %d and %d", int(ProfileNormal), int(ProfileWorker))})
	} else if spec.MaxSteps > spec.Profile.MaxSteps(false) {
		vs = append(vs, SpecViolation{Field: "maxSteps", Message: fmt.Sprintf("must not exceed %d for profile %s", spec.Profile.MaxSteps(false), spec.Profile)})
	}
	if spec.MaxSteps != 0 && (spec.MaxSteps < MinSpecMaxSteps || spec.MaxSteps > MaxSpecMaxSteps) {
		vs = append(vs, SpecViolation{Field: "maxSteps", Message: fmt.Sprintf("must be 0 or between %d and %d", MinSpecMaxSteps, MaxSpecMaxSteps)})
	}
	if !validBudgetNumber(float64(spec.MaxTokens)) {
		vs = append(vs, SpecViolation{Field: "maxTokens", Message: "must be zero or positive"})
	}
	if !validBudgetNumber(spec.MaxCostUSD) {
		vs = append(vs, SpecViolation{Field: "maxCostUSD", Message: "must be zero or positive"})
	}
	if !validBudgetNumber(float64(spec.MaxWallTimeSec)) {
		vs = append(vs, SpecViolation{Field: "maxWallTimeSec", Message: "must be zero or positive"})
	}

	if len(vs) > 0 {
		return &SpecValidationError{Violations: vs}
	}
	return nil
}

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

func (s HarnessSpec) EffectiveMaxSteps(hasDestructive bool) (int, error) {
	return StepBudget(s.Profile, s.MaxSteps, hasDestructive)
}

func (s HarnessSpec) WithMaxSteps(n int) HarnessSpec {
	s.MaxSteps = n
	return s
}

func (s HarnessSpec) Fingerprint() string {
	sum := sha256.Sum256([]byte(s.CanonicalJSON()))
	return hex.EncodeToString(sum[:])
}

func (s HarnessSpec) CanonicalJSON() string {
	type canonical struct {
		Version        int      `json:"version"`
		Name           string   `json:"name"`
		Preamble       string   `json:"preamble"`
		ExtraRules     []string `json:"extraRules"`
		ToolAllow      []string `json:"toolAllow"`
		Profile        Profile  `json:"profile,omitempty"`
		MaxSteps       int      `json:"maxSteps"`
		MaxTokens      int      `json:"maxTokens,omitempty"`
		MaxCostUSD     float64  `json:"maxCostUSD,omitempty"`
		MaxWallTimeSec int      `json:"maxWallTimeSec,omitempty"`
	}
	c := canonical{
		Version:        s.Version,
		Name:           s.Name,
		Preamble:       s.Preamble,
		ExtraRules:     append([]string{}, s.ExtraRules...),
		ToolAllow:      append([]string{}, s.ToolAllow...),
		Profile:        s.Profile,
		MaxSteps:       s.MaxSteps,
		MaxTokens:      s.MaxTokens,
		MaxCostUSD:     s.MaxCostUSD,
		MaxWallTimeSec: s.MaxWallTimeSec,
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
		return ""
	}
	return string(raw)
}

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
