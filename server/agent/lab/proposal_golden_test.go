package lab

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The proposal file is a contract: the core's admin API imports exactly these
// bytes. The golden file is that contract made visible — if the lab starts
// writing a different shape, this test fails here instead of an operator
// discovering it as a rejected import.
func TestProposal_MatchesTheGoldenFile(t *testing.T) {
	taskSet, err := LoadTaskSet(filepath.Join("testdata", "tasks.example.json"))
	if err != nil {
		t.Fatalf("load task set: %v", err)
	}
	candidates, err := LoadCandidates([]string{filepath.Join("testdata", "candidates")})
	if err != nil {
		t.Fatalf("load candidates: %v", err)
	}

	outDir := t.TempDir()
	engine := NewEngine("baseline")
	report, err := engine.Run(context.Background(), taskSet, candidates, outDir)
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}

	proposal, err := BuildProposal(report)
	if err != nil {
		t.Fatalf("build proposal: %v", err)
	}

	produced, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		t.Fatalf("marshal proposal: %v", err)
	}

	goldenPath := filepath.Join("testdata", "proposal.golden.json")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	var producedDoc, goldenDoc any
	if err := json.Unmarshal(produced, &producedDoc); err != nil {
		t.Fatalf("decode produced proposal: %v", err)
	}
	if err := json.Unmarshal(golden, &goldenDoc); err != nil {
		t.Fatalf("decode golden proposal: %v", err)
	}
	producedJSON, _ := json.Marshal(producedDoc)
	goldenJSON, _ := json.Marshal(goldenDoc)
	if string(producedJSON) != string(goldenJSON) {
		t.Errorf("the proposal no longer matches the golden file\nproduced:\n%s\ngolden:\n%s", producedJSON, goldenJSON)
	}
}
