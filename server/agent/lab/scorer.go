package lab

import (
	"sort"
)

// CandidateScore summarizes the deterministic performance of a candidate across tasks.
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

// ScoreCandidate derives deterministic metrics from a candidate's execution evidence.
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

// CompareCandidateScores returns -1 if a is better, 1 if b is better, and 0 if tied.
// Criteria:
// 1. Higher SuccessTasks
// 2. Lower ToolErrors
// 3. Lower TotalSteps
// 4. Lower EstimatedTokens
// 5. CandidateName ascending (stable deterministic tie-breaker)
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

// RankScores orders candidates deterministically by score and assigns rank 1..N.
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
