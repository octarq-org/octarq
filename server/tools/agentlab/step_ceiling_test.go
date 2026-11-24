package lab_test

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/tools/agentlab"
	"github.com/stretchr/testify/require"
)

func overCeilingCandidate() lab.Candidate {
	spec := harness.DefaultHarnessSpec()
	spec.Name = "greedy"
	spec.MaxSteps = harness.MaxStepsNormal + 12
	return lab.Candidate{Name: spec.Name, Spec: spec}
}

func scriptedTaskSet() *lab.TaskSet {
	return &lab.TaskSet{
		Version: lab.TaskSetVersion,
		Name:    "step-ceiling",
		Tasks: []lab.Task{
			{
				ID:            "ping",
				Input:         "ping",
				MockResponses: []string{"pong"},
				Criteria:      lab.TaskCriteria{ExpectedStatus: "done"},
			},
		},
	}
}

func TestLab_RefusesASpecAboveItsProfileCeiling(t *testing.T) {
	t.Parallel()

	cand := overCeilingCandidate()
	ts := scriptedTaskSet()

	_, err := lab.RunTask(context.Background(), ts.Tasks[0], cand, nil)
	require.ErrorIs(t, err, harness.ErrMaxStepsExceedsProfile)

	_, err = lab.RunCandidate(context.Background(), cand, ts)
	require.ErrorIs(t, err, harness.ErrMaxStepsExceedsProfile)

	report := &lab.LabReport{
		TaskSetName:          ts.Name,
		BaselineName:         cand.Name,
		RecommendedCandidate: cand.Name,
		Scores: []lab.CandidateScore{{
			CandidateName: cand.Name,
			TotalTasks:    1,
			SuccessTasks:  1,
			SuccessRate:   1.0,
		}},
		CandidateResults: []lab.CandidateResult{{Candidate: cand}},
	}
	_, err = lab.BuildProposal(report)
	require.ErrorIs(t, err, harness.ErrMaxStepsExceedsProfile)
}

func TestLab_AcceptsASpecAtTheWorkerCeiling(t *testing.T) {
	t.Parallel()

	spec := harness.DefaultHarnessSpec()
	spec.Name = "worker"
	spec.Profile = harness.ProfileWorker
	spec.MaxSteps = 20
	cand := lab.Candidate{Name: spec.Name, Spec: spec}
	ts := scriptedTaskSet()

	res, err := lab.RunTask(context.Background(), ts.Tasks[0], cand, nil)
	require.NoError(t, err)
	require.Equal(t, "done", res.Status)
	require.Contains(t, res.CapturedSystem, "at most 20 tool-calling steps")

	_, err = lab.RunCandidate(context.Background(), cand, ts)
	require.NoError(t, err)

	report := &lab.LabReport{
		TaskSetName:          ts.Name,
		BaselineName:         cand.Name,
		RecommendedCandidate: cand.Name,
		Scores: []lab.CandidateScore{{
			CandidateName: cand.Name,
			TotalTasks:    1,
			SuccessTasks:  1,
			SuccessRate:   1.0,
		}},
		CandidateResults: []lab.CandidateResult{{Candidate: cand}},
	}
	proposal, err := lab.BuildProposal(report)
	require.NoError(t, err)
	require.Equal(t, harness.ProfileWorker, proposal.Spec.Profile)
	require.Equal(t, 20, proposal.Spec.MaxSteps)
}
