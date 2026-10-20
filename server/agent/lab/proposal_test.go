package lab_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/agent/lab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProposal_BuildAndDecodeContract(t *testing.T) {
	t.Parallel()

	tsPath := filepath.Join("testdata", "tasks.example.json")
	candDir := filepath.Join("testdata", "candidates")

	ts, err := lab.LoadTaskSet(tsPath)
	require.NoError(t, err)

	cands, err := lab.LoadCandidates([]string{candDir})
	require.NoError(t, err)

	tmp := t.TempDir()
	engine := lab.NewEngine("baseline")
	report, err := engine.Run(context.Background(), ts, cands, tmp)
	require.NoError(t, err)

	proposal, err := lab.BuildProposal(report)
	require.NoError(t, err)
	require.NotNil(t, proposal)

	// Invariant: name matches recommended candidate
	assert.Equal(t, report.RecommendedCandidate, proposal.Name)
	assert.Equal(t, report.RecommendedCandidate, proposal.Spec.Name)

	// Contract Test: proposal.Spec can be decoded by core's harness.DecodeSpec
	rawSpec, err := json.Marshal(proposal.Spec)
	require.NoError(t, err)

	decodedSpec, err := harness.DecodeSpec(rawSpec)
	require.NoError(t, err, "core harness.DecodeSpec must decode proposal spec without error")
	assert.Equal(t, proposal.Spec.Version, decodedSpec.Version)
	assert.Equal(t, proposal.Spec.Name, decodedSpec.Name)
	assert.Equal(t, proposal.Spec.Preamble, decodedSpec.Preamble)
	assert.Equal(t, proposal.Spec.ExtraRules, decodedSpec.ExtraRules)
	assert.Equal(t, proposal.Spec.ToolAllow, decodedSpec.ToolAllow)
	assert.Equal(t, proposal.Spec.MaxSteps, decodedSpec.MaxSteps)

	// Contract Test: Whole proposal JSON document import verification
	rawProposal, err := json.Marshal(proposal)
	require.NoError(t, err)

	var doc struct {
		Name     string          `json:"name"`
		Spec     json.RawMessage `json:"spec"`
		Evidence json.RawMessage `json:"evidence"`
	}
	require.NoError(t, json.Unmarshal(rawProposal, &doc))
	assert.Equal(t, proposal.Name, doc.Name)

	specFromDoc, err := harness.DecodeSpec(doc.Spec)
	require.NoError(t, err, "core harness.DecodeSpec must decode proposal.spec from full document")
	assert.Equal(t, proposal.Spec.Name, specFromDoc.Name)

	// Evidence field-by-field verification
	assert.Equal(t, report.TaskSetName, proposal.Evidence.TaskSet)
	assert.Equal(t, report.BaselineName, proposal.Evidence.Baseline)
	assert.Equal(t, report.RecommendedCandidate, proposal.Evidence.Candidate)
	assert.Equal(t, report.SpecDiffFromBaseline.Fingerprint.Candidate, proposal.Evidence.CandidateFingerprint)
	assert.Equal(t, report.SpecDiffFromBaseline.Fingerprint.Baseline, proposal.Evidence.BaselineFingerprint)
	assert.Equal(t, report.Scores[0].SuccessRate, proposal.Evidence.SuccessRate)
	assert.Equal(t, report.Scores[0].ToolErrors, proposal.Evidence.ToolErrors)
	assert.Equal(t, report.Scores[0].TotalSteps, proposal.Evidence.TotalSteps)
	assert.Equal(t, report.Scores[0].EstimatedTokens, proposal.Evidence.EstimatedTokens)
}

func TestProposal_EffectiveMaxStepsTightening(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		inputMaxSteps int
		wantMaxSteps  int
	}{
		{
			name:          "exceeds normal profile ceiling 8 tightened to 8",
			inputMaxSteps: 32,
			wantMaxSteps:  8,
		},
		{
			name:          "zero steps resolved to normal profile ceiling 8",
			inputMaxSteps: 0,
			wantMaxSteps:  8,
		},
		{
			name:          "stricter ceiling 4 preserved",
			inputMaxSteps: 4,
			wantMaxSteps:  4,
		},
		{
			name:          "stricter ceiling 6 preserved",
			inputMaxSteps: 6,
			wantMaxSteps:  6,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := harness.DefaultHarnessSpec()
			spec.Name = "cand-test"
			spec.MaxSteps = tc.inputMaxSteps

			cand := lab.Candidate{Name: spec.Name, Spec: spec}
			score := lab.CandidateScore{
				CandidateName:   spec.Name,
				SpecFingerprint: spec.Fingerprint(),
				TotalTasks:      1,
				SuccessTasks:    1,
				SuccessRate:     1.0,
			}

			report := &lab.LabReport{
				TaskSetName:          "test-ts",
				BaselineName:         spec.Name,
				RecommendedCandidate: spec.Name,
				Scores:               []lab.CandidateScore{score},
				CandidateResults: []lab.CandidateResult{
					{Candidate: cand},
				},
			}

			prop, err := lab.BuildProposal(report)
			require.NoError(t, err)
			assert.Equal(t, tc.wantMaxSteps, prop.Spec.MaxSteps)
		})
	}
}

func TestProposal_ValidationFailClosed(t *testing.T) {
	t.Parallel()

	// Nil report
	_, err := lab.BuildProposal(nil)
	assert.Error(t, err)

	// Empty recommended candidate
	_, err = lab.BuildProposal(&lab.LabReport{
		Scores: []lab.CandidateScore{{CandidateName: "c1"}},
	})
	assert.Error(t, err)

	// Candidate not in results
	_, err = lab.BuildProposal(&lab.LabReport{
		RecommendedCandidate: "missing",
		Scores:               []lab.CandidateScore{{CandidateName: "missing"}},
	})
	assert.Error(t, err)

	// Empty candidate spec name
	badSpec := harness.DefaultHarnessSpec()
	badSpec.Name = ""
	_, err = lab.BuildProposal(&lab.LabReport{
		RecommendedCandidate: "bad",
		Scores:               []lab.CandidateScore{{CandidateName: "bad"}},
		CandidateResults: []lab.CandidateResult{
			{Candidate: lab.Candidate{Name: "bad", Spec: badSpec}},
		},
	})
	assert.Error(t, err)
}

func TestProposal_WriteProposalFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "subdir", "proposal.json")

	spec := harness.DefaultHarnessSpec()
	spec.Name = "opt-cand"
	spec.MaxSteps = 4

	proposal := &lab.Proposal{
		Name: spec.Name,
		Spec: spec,
		Evidence: lab.ProposalEvidence{
			TaskSet:              "test-ts",
			Baseline:             "baseline",
			Candidate:            "opt-cand",
			BaselineFingerprint:  "fp-base",
			CandidateFingerprint: "fp-cand",
			SuccessRate:          1.0,
			ToolErrors:           0,
			TotalSteps:           2,
			EstimatedTokens:      500,
		},
	}

	err := lab.WriteProposalFile(proposal, outPath)
	require.NoError(t, err)
	assert.FileExists(t, outPath)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	var readProp lab.Proposal
	require.NoError(t, json.Unmarshal(data, &readProp))
	assert.Equal(t, proposal.Name, readProp.Name)
	assert.Equal(t, proposal.Spec.MaxSteps, readProp.Spec.MaxSteps)
	assert.Equal(t, proposal.Evidence.EstimatedTokens, readProp.Evidence.EstimatedTokens)

	// Fail closed on invalid inputs
	assert.Error(t, lab.WriteProposalFile(nil, outPath))
	assert.Error(t, lab.WriteProposalFile(proposal, ""))
}
