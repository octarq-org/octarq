package lab

import (
	"context"
	"errors"
	"fmt"
)

// Engine runs the complete offline evaluation workflow: Propose → Probe → Compose.
type Engine struct {
	PreferredBaseline string
}

// NewEngine constructs an Engine instance.
func NewEngine(preferredBaseline string) *Engine {
	return &Engine{
		PreferredBaseline: preferredBaseline,
	}
}

// Run executes the evaluation of candidates against the task set and writes reports to outDir.
func (e *Engine) Run(ctx context.Context, ts *TaskSet, candidates []Candidate, outDir string) (*LabReport, error) {
	if ts == nil {
		return nil, errors.New("task set must not be nil")
	}
	if len(candidates) == 0 {
		return nil, errors.New("at least one candidate must be provided")
	}
	if outDir == "" {
		return nil, errors.New("output directory must not be empty")
	}

	var results []CandidateResult
	for _, cand := range candidates {
		res, err := RunCandidate(ctx, cand, ts)
		if err != nil {
			return nil, fmt.Errorf("candidate %q failed evaluation: %w", cand.Name, err)
		}
		results = append(results, res)
	}

	preferred := ""
	if e != nil {
		preferred = e.PreferredBaseline
	}

	report, err := BuildReport(ts, results, preferred)
	if err != nil {
		return nil, fmt.Errorf("build report: %w", err)
	}

	if err := WriteReportFiles(report, outDir); err != nil {
		return nil, fmt.Errorf("write report files: %w", err)
	}

	return report, nil
}
