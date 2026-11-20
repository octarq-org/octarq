package lab

import (
	"fmt"
	"sort"

	"github.com/octarq-org/octarq/server/agent/harness"
)

type StringFieldDiff struct {
	Baseline  string `json:"baseline"`
	Candidate string `json:"candidate"`
	Changed   bool   `json:"changed"`
}

type SliceFieldDiff struct {
	Baseline  []string `json:"baseline"`
	Candidate []string `json:"candidate"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Unchanged []string `json:"unchanged"`
	Changed   bool     `json:"changed"`
}

type StepsFieldDiff struct {
	Baseline           int  `json:"baseline"`
	Candidate          int  `json:"candidate"`
	EffectiveBaseline  int  `json:"effective_baseline"`
	EffectiveCandidate int  `json:"effective_candidate"`
	Changed            bool `json:"changed"`
}

type SpecDiff struct {
	BaselineName  string          `json:"baseline_name"`
	CandidateName string          `json:"candidate_name"`
	Fingerprint   StringFieldDiff `json:"fingerprint"`
	Preamble      StringFieldDiff `json:"preamble"`
	ExtraRules    SliceFieldDiff  `json:"extra_rules"`
	ToolAllow     SliceFieldDiff  `json:"tool_allow"`
	MaxSteps      StepsFieldDiff  `json:"max_steps"`
	AnyChanged    bool            `json:"any_changed"`
}

func diffSlice(base, cand []string) SliceFieldDiff {
	baseMap := make(map[string]bool, len(base))
	for _, s := range base {
		baseMap[s] = true
	}
	candMap := make(map[string]bool, len(cand))
	for _, s := range cand {
		candMap[s] = true
	}

	var added, removed, unchanged []string
	for _, s := range cand {
		if !baseMap[s] {
			added = append(added, s)
		} else {
			unchanged = append(unchanged, s)
		}
	}
	for _, s := range base {
		if !candMap[s] {
			removed = append(removed, s)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(unchanged)

	changed := len(added) > 0 || len(removed) > 0

	return SliceFieldDiff{
		Baseline:  append([]string{}, base...),
		Candidate: append([]string{}, cand...),
		Added:     added,
		Removed:   removed,
		Unchanged: unchanged,
		Changed:   changed,
	}
}

func DiffSpecs(baseline, cand harness.HarnessSpec) (SpecDiff, error) {
	effBase, err := baseline.EffectiveMaxSteps(false)
	if err != nil {
		return SpecDiff{}, fmt.Errorf("baseline spec %q: %w", baseline.Name, err)
	}
	effCand, err := cand.EffectiveMaxSteps(false)
	if err != nil {
		return SpecDiff{}, fmt.Errorf("candidate spec %q: %w", cand.Name, err)
	}

	preambleDiff := StringFieldDiff{
		Baseline:  baseline.Preamble,
		Candidate: cand.Preamble,
		Changed:   baseline.Preamble != cand.Preamble,
	}
	fpDiff := StringFieldDiff{
		Baseline:  baseline.Fingerprint(),
		Candidate: cand.Fingerprint(),
		Changed:   baseline.Fingerprint() != cand.Fingerprint(),
	}
	extraRulesDiff := diffSlice(baseline.ExtraRules, cand.ExtraRules)
	toolAllowDiff := diffSlice(baseline.ToolAllow, cand.ToolAllow)
	maxStepsDiff := StepsFieldDiff{
		Baseline:           baseline.MaxSteps,
		Candidate:          cand.MaxSteps,
		EffectiveBaseline:  effBase,
		EffectiveCandidate: effCand,
		Changed:            baseline.MaxSteps != cand.MaxSteps || effBase != effCand,
	}

	anyChanged := preambleDiff.Changed || extraRulesDiff.Changed || toolAllowDiff.Changed || maxStepsDiff.Changed || fpDiff.Changed

	return SpecDiff{
		BaselineName:  baseline.Name,
		CandidateName: cand.Name,
		Fingerprint:   fpDiff,
		Preamble:      preambleDiff,
		ExtraRules:    extraRulesDiff,
		ToolAllow:     toolAllowDiff,
		MaxSteps:      maxStepsDiff,
		AnyChanged:    anyChanged,
	}, nil
}
