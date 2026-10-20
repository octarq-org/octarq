package lab

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
)

// The lab's helpers are the lab's own contract with the operator: what a
// validation failure says, what the replay recorded, how the base catalog is
// derived when the task set does not declare one. These tests pin that
// behaviour so a refactor cannot quietly change what a report means.

func TestFormatValidationError_ListsEveryViolation(t *testing.T) {
	spec := harness.HarnessSpec{Version: 99, Name: ""}
	err := harness.Validate(spec)
	if err == nil {
		t.Fatal("expected the spec to be refused")
	}

	formatted := FormatValidationError(err)
	if !strings.Contains(formatted, "spec validation failed") {
		t.Errorf("formatted = %q, want the header", formatted)
	}
	if !strings.Contains(formatted, "version") || !strings.Contains(formatted, "name") {
		t.Errorf("formatted = %q, want the offending fields", formatted)
	}
}

func TestFormatValidationError_PassesOtherErrorsThrough(t *testing.T) {
	if got := FormatValidationError(context.Canceled); got != context.Canceled.Error() {
		t.Errorf("formatted = %q, want the original message", got)
	}
}

func TestMockToolExecutor_RecordsTheCallsItRan(t *testing.T) {
	exec := NewMockToolExecutor(map[string]ToolMockResponse{
		"read_links": {Output: "[]"},
	})

	if _, err := exec.Execute(context.Background(), 1, "read_links", `{"limit":5}`); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	calls := exec.ExecutedCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(calls))
	}
	if calls[0].Name != "read_links" {
		t.Errorf("call = %+v", calls[0])
	}
	if calls[0].ArgsJSON != `{"limit":5}` {
		t.Errorf("args = %q, want the arguments as they were passed", calls[0].ArgsJSON)
	}
}

func TestEstimateTokens_IsDeterministicAndNeverZeroForText(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
	if got := EstimateTokens("a"); got != 1 {
		t.Errorf("EstimateTokens(\"a\") = %d, want 1", got)
	}
	if got := EstimateTokens("abcd"); got != 1 {
		t.Errorf("EstimateTokens(\"abcd\") = %d, want 1", got)
	}
	if got := EstimateTokens("abcde"); got != 2 {
		t.Errorf("EstimateTokens(\"abcde\") = %d, want 2", got)
	}
	// Multi-byte runes count as one, not as their bytes.
	if got := EstimateTokens("中文字"); got != 1 {
		t.Errorf("EstimateTokens of three runes = %d, want 1", got)
	}
}

func TestResolveBaseCatalog_DerivesTheToolsFromTheMocks(t *testing.T) {
	ts := &TaskSet{
		Tasks: []Task{
			{ID: "t1", MockTools: map[string]ToolMockResponse{"read_links": {Output: "[]"}}},
			{ID: "t2", MockTools: map[string]ToolMockResponse{"list_domains": {Output: "[]"}, "read_links": {Output: "[]"}}},
		},
	}

	tools := ResolveBaseCatalog(ts)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %+v", len(tools), tools)
	}
	if tools[0].Name != "list_domains" || tools[1].Name != "read_links" {
		t.Errorf("tools are not ordered by name: %+v", tools)
	}
	if tools[0].Summary == "" || tools[0].Description == "" {
		t.Errorf("a derived tool has no guidance: %+v", tools[0])
	}
}

func TestResolveBaseCatalog_PrefersTheDeclaredCatalog(t *testing.T) {
	ts := &TaskSet{
		Catalog: []ToolDef{{Name: "declared_tool"}},
		Tasks:   []Task{{ID: "t1", MockTools: map[string]ToolMockResponse{"mock_tool": {Output: "[]"}}}},
	}

	tools := ResolveBaseCatalog(ts)
	if len(tools) != 1 || tools[0].Name != "declared_tool" {
		t.Errorf("tools = %+v, want the declared catalog", tools)
	}
}

func TestLoadCandidates_RefusesAnEmptyTarget(t *testing.T) {
	if _, err := LoadCandidates(nil); err == nil {
		t.Error("expected an empty target list to be refused")
	}
}

func TestLoadCandidates_RefusesAMissingFile(t *testing.T) {
	if _, err := LoadCandidates([]string{filepath.Join(t.TempDir(), "missing.json")}); err == nil {
		t.Error("expected a missing candidate file to be refused")
	}
}

func TestLoadCandidateFile_RefusesAnInvalidSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "candidate.json")
	raw := map[string]any{
		"name": "broken",
		"spec": map[string]any{"version": 99, "name": ""},
	}
	body, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadCandidateFile(path); err == nil {
		t.Error("expected the invalid candidate to be refused")
	}
}

func TestBuildProposal_RefusesAReportWithoutARecommendation(t *testing.T) {
	if _, err := BuildProposal(nil); err == nil {
		t.Error("expected a nil report to be refused")
	}
	if _, err := BuildProposal(&LabReport{}); err == nil {
		t.Error("expected a report without a recommendation to be refused")
	}
	if _, err := BuildProposal(&LabReport{RecommendedCandidate: "x"}); err == nil {
		t.Error("expected a report without scores to be refused")
	}
}
