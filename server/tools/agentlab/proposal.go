package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/octarq-org/octarq/server/agent/harness"
)

type Proposal struct {
	Spec     harness.HarnessSpec `json:"spec"`
	Evidence ProposalEvidence    `json:"evidence"`
}

type ProposalEvidence struct {
	TaskSet              string  `json:"taskSet"`
	Baseline             string  `json:"baseline"`
	Candidate            string  `json:"candidate"`
	BaselineFingerprint  string  `json:"baselineFingerprint"`
	CandidateFingerprint string  `json:"candidateFingerprint"`
	SuccessRate          float64 `json:"successRate"`
	ToolErrors           int     `json:"toolErrors"`
	TotalSteps           int     `json:"totalSteps"`
	EstimatedTokens      int     `json:"estimatedTokens"`
}

func BuildProposal(report *LabReport) (*Proposal, error) {
	if report == nil {
		return nil, errors.New("cannot build proposal: report is nil")
	}
	if strings.TrimSpace(report.RecommendedCandidate) == "" {
		return nil, errors.New("cannot build proposal: no recommended candidate in report")
	}
	if len(report.Scores) == 0 {
		return nil, errors.New("cannot build proposal: report contains no scores")
	}

	var recCand *Candidate
	for _, cr := range report.CandidateResults {
		if cr.Candidate.Name == report.RecommendedCandidate {
			c := cr.Candidate
			recCand = &c
			break
		}
	}
	if recCand == nil {
		return nil, fmt.Errorf("recommended candidate %q not found in candidate results", report.RecommendedCandidate)
	}

	var recScore *CandidateScore
	for _, sc := range report.Scores {
		if sc.CandidateName == report.RecommendedCandidate {
			s := sc
			recScore = &s
			break
		}
	}
	if recScore == nil {
		return nil, fmt.Errorf("recommended candidate %q score not found in report", report.RecommendedCandidate)
	}

	if strings.TrimSpace(recCand.Spec.Name) == "" {
		return nil, errors.New("candidate spec name must not be empty")
	}
	if len(recCand.Spec.Name) > harness.MaxSpecNameLen {
		return nil, fmt.Errorf("candidate spec name exceeds %d characters", harness.MaxSpecNameLen)
	}

	effectiveSteps, err := recCand.Spec.EffectiveMaxSteps(false)
	if err != nil {
		return nil, fmt.Errorf("candidate %q: %w", recCand.Name, err)
	}
	specToPropose := recCand.Spec.WithMaxSteps(effectiveSteps)

	if err := harness.Validate(specToPropose); err != nil {
		return nil, fmt.Errorf("proposal spec validation failed: %w", err)
	}

	baselineFp := report.SpecDiffFromBaseline.Fingerprint.Baseline
	if baselineFp == "" {
		for _, sc := range report.Scores {
			if sc.CandidateName == report.BaselineName {
				baselineFp = sc.SpecFingerprint
				break
			}
		}
	}
	if baselineFp == "" {
		for _, cr := range report.CandidateResults {
			if cr.Candidate.Name == report.BaselineName {
				baselineFp = cr.Candidate.Spec.Fingerprint()
				break
			}
		}
	}

	evidence := ProposalEvidence{
		TaskSet:              report.TaskSetName,
		Baseline:             report.BaselineName,
		Candidate:            recCand.Name,
		BaselineFingerprint:  baselineFp,
		CandidateFingerprint: specToPropose.Fingerprint(),
		SuccessRate:          recScore.SuccessRate,
		ToolErrors:           recScore.ToolErrors,
		TotalSteps:           recScore.TotalSteps,
		EstimatedTokens:      recScore.EstimatedTokens,
	}

	return &Proposal{
		Spec:     specToPropose,
		Evidence: evidence,
	}, nil
}

func WriteProposalFile(p *Proposal, path string) error {
	if p == nil {
		return errors.New("cannot write nil proposal")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("proposal file path must not be empty")
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create proposal directory %q: %w", dir, err)
		}
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal proposal json: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write proposal file %q: %w", path, err)
	}

	return nil
}
