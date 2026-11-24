package lab

import (
	"sort"
)

type CandidateScore struct {
	CandidateName   string  `json:"candidate_name"`
	SpecFingerprint string  `json:"spec_fingerprint"`
	TotalTasks      int     `json:"total_tasks"`
	SuccessTasks    int     `json:"success_tasks"`
	SuccessRate     float64 `json:"success_rate"`
	TotalSteps      int     `json:"total_steps"`
	ToolErrors      int     `json:"tool_errors"`
	EstimatedTokens int     `json:"estimated_tokens"`
	Rank            int     `json:"rank"`
}

func ScoreCandidate(res CandidateResult) CandidateScore {
	totalTasks := len(res.TaskResults)
	successTasks := 0
	totalSteps := 0
	toolErrors := 0
	tokens := 0

	for _, tr := range res.TaskResults {
		if tr.Passed {
			successTasks++
		}
		totalSteps += tr.Steps
		toolErrors += tr.ToolErrors
		tokens += tr.TotalTokens
	}

	successRate := 0.0
	if totalTasks > 0 {
		successRate = float64(successTasks) / float64(totalTasks)
	}

	return CandidateScore{
		CandidateName:   res.Candidate.Name,
		SpecFingerprint: res.Candidate.Spec.Fingerprint(),
		TotalTasks:      totalTasks,
		SuccessTasks:    successTasks,
		SuccessRate:     successRate,
		TotalSteps:      totalSteps,
		ToolErrors:      toolErrors,
		EstimatedTokens: tokens,
	}
}

func CompareCandidateScores(a, b CandidateScore) int {
	if a.SuccessTasks != b.SuccessTasks {
		if a.SuccessTasks > b.SuccessTasks {
			return -1
		}
		return 1
	}

	if a.ToolErrors != b.ToolErrors {
		if a.ToolErrors < b.ToolErrors {
			return -1
		}
		return 1
	}

	if a.TotalSteps != b.TotalSteps {
		if a.TotalSteps < b.TotalSteps {
			return -1
		}
		return 1
	}

	if a.EstimatedTokens != b.EstimatedTokens {
		if a.EstimatedTokens < b.EstimatedTokens {
			return -1
		}
		return 1
	}

	if a.CandidateName != b.CandidateName {
		if a.CandidateName < b.CandidateName {
			return -1
		}
		return 1
	}

	return 0
}

func RankScores(scores []CandidateScore) []CandidateScore {
	ranked := make([]CandidateScore, len(scores))
	copy(ranked, scores)

	sort.Slice(ranked, func(i, j int) bool {
		return CompareCandidateScores(ranked[i], ranked[j]) < 0
	})

	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	return ranked
}
