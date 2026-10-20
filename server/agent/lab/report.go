package lab

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LabReport is the complete offline evaluation report.
type LabReport struct {
	TaskSetName           string            `json:"task_set_name"`
	TotalTasks            int               `json:"total_tasks"`
	BaselineName          string            `json:"baseline_name"`
	RecommendedCandidate  string            `json:"recommended_candidate"`
	RecommendationReason  string            `json:"recommendation_reason"`
	IsBaselineRecommended bool              `json:"is_baseline_recommended"`
	Scores                []CandidateScore  `json:"scores"`
	SpecDiffFromBaseline  SpecDiff          `json:"spec_diff_from_baseline"`
	CandidateResults      []CandidateResult `json:"candidate_results"`
}

// BuildReport constructs the report by scoring and ranking candidate results.
func BuildReport(ts *TaskSet, results []CandidateResult, preferredBaseline string) (*LabReport, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no candidate results to report")
	}

	scores := make([]CandidateScore, len(results))
	candMap := make(map[string]CandidateResult)
	for i, r := range results {
		scores[i] = ScoreCandidate(r)
		candMap[r.Candidate.Name] = r
	}

	rankedScores := RankScores(scores)
	bestScore := rankedScores[0]
	bestResult := candMap[bestScore.CandidateName]

	// Determine baseline candidate
	baselineName := preferredBaseline
	if baselineName == "" {
		if _, ok := candMap["baseline"]; ok {
			baselineName = "baseline"
		} else if _, ok := candMap["default"]; ok {
			baselineName = "default"
		} else {
			baselineName = results[0].Candidate.Name
		}
	}

	baselineResult, ok := candMap[baselineName]
	if !ok {
		baselineResult = results[0]
		baselineName = baselineResult.Candidate.Name
	}

	// Baseline score
	var baselineScore CandidateScore
	for _, sc := range rankedScores {
		if sc.CandidateName == baselineName {
			baselineScore = sc
			break
		}
	}

	diff := DiffSpecs(baselineResult.Candidate.Spec, bestResult.Candidate.Spec)

	var recReason string
	isBase := bestScore.CandidateName == baselineName
	if isBase {
		recReason = fmt.Sprintf("Baseline %q is already the top-performing candidate (%d/%d tasks passed, %d tool errors).",
			baselineName, bestScore.SuccessTasks, bestScore.TotalTasks, bestScore.ToolErrors)
	} else {
		recReason = fmt.Sprintf("Candidate %q outperforms baseline %q: %d/%d passed (baseline %d/%d), %d tool errors (baseline %d), %d steps (baseline %d), %d est. tokens (baseline %d).",
			bestScore.CandidateName, baselineName,
			bestScore.SuccessTasks, bestScore.TotalTasks, baselineScore.SuccessTasks, baselineScore.TotalTasks,
			bestScore.ToolErrors, baselineScore.ToolErrors,
			bestScore.TotalSteps, baselineScore.TotalSteps,
			bestScore.EstimatedTokens, baselineScore.EstimatedTokens)
	}

	return &LabReport{
		TaskSetName:           ts.Name,
		TotalTasks:            len(ts.Tasks),
		BaselineName:          baselineName,
		RecommendedCandidate:  bestScore.CandidateName,
		RecommendationReason:  recReason,
		IsBaselineRecommended: isBase,
		Scores:                rankedScores,
		SpecDiffFromBaseline:  diff,
		CandidateResults:      results,
	}, nil
}

// RenderMarkdown formats the LabReport into a comprehensive Markdown summary.
func RenderMarkdown(r *LabReport) string {
	var b strings.Builder

	b.WriteString("# Agent Harness Lab Evaluation Report\n\n")
	b.WriteString("> **Offline Human Review Only**: This report was generated offline via deterministic replay.\n")
	b.WriteString("> The lab does not modify runtime databases or production prompts. All adoptions require human operator review.\n\n")

	fmt.Fprintf(&b, "- **Task Set**: `%s` (%d tasks)\n", r.TaskSetName, r.TotalTasks)
	fmt.Fprintf(&b, "- **Baseline Candidate**: `%s`\n", r.BaselineName)
	fmt.Fprintf(&b, "- **Recommended Candidate**: `%s`\n\n", r.RecommendedCandidate)

	b.WriteString("## Recommendation\n\n")
	if r.IsBaselineRecommended {
		fmt.Fprintf(&b, "**Keep Baseline (`%s`)**: %s\n\n", r.RecommendedCandidate, r.RecommendationReason)
	} else {
		fmt.Fprintf(&b, "**Adopt Candidate (`%s`)**: %s\n\n", r.RecommendedCandidate, r.RecommendationReason)
	}

	b.WriteString("## Candidate Leaderboard\n\n")
	b.WriteString("| Rank | Candidate | Success Rate | Tasks Passed | Tool Errors | Total Steps | Est. Tokens | Notes |\n")
	b.WriteString("|:---:|:---|:---:|:---:|:---:|:---:|:---:|:---|\n")

	for _, sc := range r.Scores {
		notes := ""
		if sc.CandidateName == r.RecommendedCandidate {
			notes = "**Recommended**"
		} else if sc.CandidateName == r.BaselineName {
			notes = "Baseline"
		}
		fmt.Fprintf(&b, "| %d | `%s` | %.1f%% | %d/%d | %d | %d | %d | %s |\n",
			sc.Rank, sc.CandidateName, sc.SuccessRate*100, sc.SuccessTasks, sc.TotalTasks,
			sc.ToolErrors, sc.TotalSteps, sc.EstimatedTokens, notes)
	}
	b.WriteString("\n")

	b.WriteString("## Spec Diff: Baseline vs. Recommended Candidate\n\n")
	diff := r.SpecDiffFromBaseline
	if !diff.AnyChanged {
		b.WriteString("No differences detected between baseline and recommended candidate.\n\n")
	} else {
		fmt.Fprintf(&b, "- **Spec Fingerprint**: `%s` → `%s`\n", diff.Fingerprint.Baseline, diff.Fingerprint.Candidate)

		if diff.Preamble.Changed {
			b.WriteString("- **Preamble**: Modified\n")
			fmt.Fprintf(&b, "  - *Baseline*: %q\n", diff.Preamble.Baseline)
			fmt.Fprintf(&b, "  - *Candidate*: %q\n", diff.Preamble.Candidate)
		} else {
			b.WriteString("- **Preamble**: Unchanged\n")
		}

		if diff.ExtraRules.Changed {
			b.WriteString("- **Extra Rules**: Modified\n")
			if len(diff.ExtraRules.Added) > 0 {
				fmt.Fprintf(&b, "  - *Added*: %v\n", diff.ExtraRules.Added)
			}
			if len(diff.ExtraRules.Removed) > 0 {
				fmt.Fprintf(&b, "  - *Removed*: %v\n", diff.ExtraRules.Removed)
			}
		} else {
			b.WriteString("- **Extra Rules**: Unchanged\n")
		}

		if diff.ToolAllow.Changed {
			b.WriteString("- **Tool Allowlist**: Modified\n")
			if len(diff.ToolAllow.Added) > 0 {
				fmt.Fprintf(&b, "  - *Added*: %v\n", diff.ToolAllow.Added)
			}
			if len(diff.ToolAllow.Removed) > 0 {
				fmt.Fprintf(&b, "  - *Removed*: %v\n", diff.ToolAllow.Removed)
			}
		} else {
			b.WriteString("- **Tool Allowlist**: Unchanged\n")
		}

		if diff.MaxSteps.Changed {
			fmt.Fprintf(&b, "- **MaxSteps**: %d (effective: %d) → %d (effective: %d)\n",
				diff.MaxSteps.Baseline, diff.MaxSteps.EffectiveBaseline,
				diff.MaxSteps.Candidate, diff.MaxSteps.EffectiveCandidate)
		} else {
			fmt.Fprintf(&b, "- **MaxSteps**: Unchanged (%d, effective %d)\n",
				diff.MaxSteps.Baseline, diff.MaxSteps.EffectiveBaseline)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Task Breakdown\n\n")
	for _, cr := range r.CandidateResults {
		fmt.Fprintf(&b, "### Candidate: `%s`\n\n", cr.Candidate.Name)
		b.WriteString("| Task ID | Outcome | Status | Steps | Errors | Tokens | Details |\n")
		b.WriteString("|:---|:---:|:---:|:---:|:---:|:---:|:---|\n")
		for _, tr := range cr.TaskResults {
			outcome := "PASS"
			if !tr.Passed {
				outcome = "**FAIL**"
			}
			details := ""
			if len(tr.FailureReasons) > 0 {
				details = strings.Join(tr.FailureReasons, "; ")
			} else {
				details = strings.Join(tr.ToolCalls, " → ")
			}
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %d | %d | %d | %s |\n",
				tr.TaskID, outcome, tr.Status, tr.Steps, tr.ToolErrors, tr.TotalTokens, details)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// WriteReportFiles exports report.json and report.md to the output directory.
func WriteReportFiles(r *LabReport, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create report directory %q: %w", outDir, err)
	}

	jsonBytes, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report json: %w", err)
	}

	jsonPath := filepath.Join(outDir, "report.json")
	if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("write %q: %w", jsonPath, err)
	}

	mdContent := RenderMarkdown(r)
	mdPath := filepath.Join(outDir, "report.md")
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		return fmt.Errorf("write %q: %w", mdPath, err)
	}

	return nil
}
