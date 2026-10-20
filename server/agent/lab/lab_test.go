package lab_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/agent/lab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplay_ScriptedResponses(t *testing.T) {
	t.Parallel()

	task := lab.Task{
		ID:    "test-step-execution",
		Input: "Run diagnostic tool",
		MockResponses: []string{
			"```tool_call\n{\"name\": \"diagnostic\", \"args\": {\"target\": \"db\"}}\n```",
			"Diagnostic completed successfully.",
		},
		MockTools: map[string]lab.ToolMockResponse{
			"diagnostic": {
				Output: `{"status": "healthy"}`,
			},
		},
		Criteria: lab.TaskCriteria{
			ExpectedStatus: "done",
			ExpectedTools:  []string{"diagnostic"},
			FinalContains:  "Diagnostic completed",
			MaxSteps:       2,
			MaxToolErrors:  ptr(0),
		},
	}

	cand := lab.Candidate{
		Name: "test-cand",
		Spec: harness.DefaultHarnessSpec(),
	}

	res, err := lab.RunTask(context.Background(), task, cand, nil)
	require.NoError(t, err)

	assert.True(t, res.Passed, "task should pass criteria")
	assert.Equal(t, "done", res.Status)
	assert.Equal(t, 1, res.Steps)
	assert.Equal(t, 0, res.ToolErrors)
	assert.Equal(t, []string{"diagnostic"}, res.ToolCalls)
	assert.NotEmpty(t, res.CapturedSystem)
	assert.NotEmpty(t, res.TraceEvents)

	// Verify trace events
	kinds := make([]harness.TraceKind, 0, len(res.TraceEvents))
	for _, ev := range res.TraceEvents {
		kinds = append(kinds, ev.Kind)
	}
	assert.Contains(t, kinds, harness.TraceKindTurnStart)
	assert.Contains(t, kinds, harness.TraceKindStep)
	assert.Contains(t, kinds, harness.TraceKindTurnEnd)
}

func TestReplay_CompleterCapturesSystemPrompt(t *testing.T) {
	t.Parallel()

	ts := &lab.TaskSet{
		Version: 1,
		Name:    "prompt-diff-test",
		Tasks: []lab.Task{
			{
				ID:            "simple-ping",
				Input:         "ping",
				MockResponses: []string{"pong"},
				Criteria:      lab.TaskCriteria{ExpectedStatus: "done"},
			},
		},
	}

	specA := harness.DefaultHarnessSpec()
	specA.Name = "cand-a"
	specA.Preamble = "Unique Preamble Alpha"

	specB := harness.DefaultHarnessSpec()
	specB.Name = "cand-b"
	specB.Preamble = "Unique Preamble Beta"

	candA := lab.Candidate{Name: specA.Name, Spec: specA}
	candB := lab.Candidate{Name: specB.Name, Spec: specB}

	resA, err := lab.RunCandidate(context.Background(), candA, ts)
	require.NoError(t, err)

	resB, err := lab.RunCandidate(context.Background(), candB, ts)
	require.NoError(t, err)

	require.Len(t, resA.TaskResults, 1)
	require.Len(t, resB.TaskResults, 1)

	assert.Contains(t, resA.TaskResults[0].CapturedSystem, "Unique Preamble Alpha")
	assert.Contains(t, resB.TaskResults[0].CapturedSystem, "Unique Preamble Beta")
	assert.NotEqual(t, resA.TaskResults[0].CapturedSystem, resB.TaskResults[0].CapturedSystem)
}

func TestScorer_Deterministic(t *testing.T) {
	t.Parallel()

	tsPath := filepath.Join("testdata", "tasks.example.json")
	candDir := filepath.Join("testdata", "candidates")

	ts, err := lab.LoadTaskSet(tsPath)
	require.NoError(t, err)

	cands, err := lab.LoadCandidates([]string{candDir})
	require.NoError(t, err)

	ctx := context.Background()
	engine := lab.NewEngine("baseline")

	tmp1 := t.TempDir()
	rep1, err := engine.Run(ctx, ts, cands, tmp1)
	require.NoError(t, err)

	tmp2 := t.TempDir()
	rep2, err := engine.Run(ctx, ts, cands, tmp2)
	require.NoError(t, err)

	// Compare scores struct-by-struct
	require.Equal(t, len(rep1.Scores), len(rep2.Scores))
	for i := range rep1.Scores {
		assert.Equal(t, rep1.Scores[i], rep2.Scores[i], "scores must be bit-for-bit identical across runs")
	}

	// Compare written JSON reports byte-for-byte
	b1, err := os.ReadFile(filepath.Join(tmp1, "report.json"))
	require.NoError(t, err)
	b2, err := os.ReadFile(filepath.Join(tmp2, "report.json"))
	require.NoError(t, err)
	assert.Equal(t, string(b1), string(b2), "report.json must be identical across runs")
}

func TestCandidate_InvalidSpecRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	tests := []struct {
		name        string
		jsonContent string
		wantErrMsg  string
	}{
		{
			name: "invalid version",
			jsonContent: `{
				"version": 99,
				"name": "invalid-version"
			}`,
			wantErrMsg: "version",
		},
		{
			name: "empty name",
			jsonContent: `{
				"version": 1,
				"name": ""
			}`,
			wantErrMsg: "name",
		},
		{
			name: "excessive maxSteps",
			jsonContent: `{
				"version": 1,
				"name": "too-many-steps",
				"maxSteps": 100
			}`,
			wantErrMsg: "maxSteps",
		},
		{
			name: "unknown field rejected",
			jsonContent: `{
				"version": 1,
				"name": "unknown-field",
				"unsupportedField": true
			}`,
			wantErrMsg: "unknown field",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			filePath := filepath.Join(dir, tc.name+".json")
			require.NoError(t, os.WriteFile(filePath, []byte(tc.jsonContent), 0644))

			_, err := lab.LoadCandidateFile(filePath)
			require.Error(t, err, "must reject invalid candidate spec")
			assert.Contains(t, err.Error(), tc.wantErrMsg)

			// Also verify LoadCandidates fails closed
			_, errAll := lab.LoadCandidates([]string{filePath})
			require.Error(t, errAll)
		})
	}
}

func TestDiff_FieldByField(t *testing.T) {
	t.Parallel()

	base := harness.HarnessSpec{
		Version:    1,
		Name:       "baseline",
		Preamble:   "Base Preamble",
		ExtraRules: []string{"Rule 1", "Rule 2"},
		ToolAllow:  []string{"toolA", "toolB"},
		MaxSteps:   0,
	}

	cand := harness.HarnessSpec{
		Version:    1,
		Name:       "cand",
		Preamble:   "Custom Preamble",
		ExtraRules: []string{"Rule 2", "Rule 3"},
		ToolAllow:  []string{"toolB", "toolC"},
		MaxSteps:   4,
	}

	diff := lab.DiffSpecs(base, cand)

	assert.True(t, diff.AnyChanged)
	assert.True(t, diff.Preamble.Changed)
	assert.Equal(t, "Base Preamble", diff.Preamble.Baseline)
	assert.Equal(t, "Custom Preamble", diff.Preamble.Candidate)

	assert.True(t, diff.ExtraRules.Changed)
	assert.Equal(t, []string{"Rule 3"}, diff.ExtraRules.Added)
	assert.Equal(t, []string{"Rule 1"}, diff.ExtraRules.Removed)
	assert.Equal(t, []string{"Rule 2"}, diff.ExtraRules.Unchanged)

	assert.True(t, diff.ToolAllow.Changed)
	assert.Equal(t, []string{"toolC"}, diff.ToolAllow.Added)
	assert.Equal(t, []string{"toolA"}, diff.ToolAllow.Removed)
	assert.Equal(t, []string{"toolB"}, diff.ToolAllow.Unchanged)

	assert.True(t, diff.MaxSteps.Changed)
	assert.Equal(t, 8, diff.MaxSteps.EffectiveBaseline)
	assert.Equal(t, 4, diff.MaxSteps.EffectiveCandidate)
}

func TestReporter_GeneratesJSONAndMarkdown(t *testing.T) {
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

	assert.Equal(t, "candidate-optimized", report.RecommendedCandidate)
	assert.Equal(t, "baseline", report.BaselineName)
	assert.False(t, report.IsBaselineRecommended)
	assert.Len(t, report.Scores, 3)

	// Check files generated
	jsonPath := filepath.Join(tmp, "report.json")
	mdPath := filepath.Join(tmp, "report.md")

	assert.FileExists(t, jsonPath)
	assert.FileExists(t, mdPath)

	mdBytes, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	md := string(mdBytes)

	assert.Contains(t, md, "# Agent Harness Lab Evaluation Report")
	assert.Contains(t, md, "Offline Human Review Only")
	assert.Contains(t, md, "Candidate Leaderboard")
	assert.Contains(t, md, "Spec Diff: Baseline vs. Recommended Candidate")
	assert.Contains(t, md, "Task Breakdown")
}

func TestEngine_FailClosed(t *testing.T) {
	t.Parallel()

	engine := lab.NewEngine("")
	validTS := &lab.TaskSet{
		Version: 1,
		Name:    "valid",
		Tasks: []lab.Task{
			{ID: "t1", Input: "hi", MockResponses: []string{"hey"}},
		},
	}
	validCand := lab.Candidate{Name: "c1", Spec: harness.DefaultHarnessSpec()}

	// Nil task set
	_, err := engine.Run(context.Background(), nil, []lab.Candidate{validCand}, t.TempDir())
	assert.Error(t, err)

	// Empty candidates
	_, err = engine.Run(context.Background(), validTS, nil, t.TempDir())
	assert.Error(t, err)

	// Empty outDir
	_, err = engine.Run(context.Background(), validTS, []lab.Candidate{validCand}, "")
	assert.Error(t, err)
}

func ptr[T any](v T) *T {
	return &v
}
