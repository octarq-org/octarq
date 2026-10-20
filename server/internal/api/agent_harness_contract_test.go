package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/agent/lab"
	"github.com/octarq-org/octarq/server/internal/agentstore"
)

// The lab writes the proposal; the API imports it. This test crosses that
// boundary on purpose: the two sides are written by different hands (a CLI and
// an HTTP handler), and a field renamed on either side would only show up as
// a 400 at the moment an operator tried to adopt a proposal.
func TestAgentHarness_TheLabProposalIsImportable(t *testing.T) {
	labDir := filepath.Join("..", "..", "agent", "lab")

	taskSet, err := lab.LoadTaskSet(filepath.Join(labDir, "testdata", "tasks.example.json"))
	if err != nil {
		t.Fatalf("load task set: %v", err)
	}
	candidates, err := lab.LoadCandidates([]string{filepath.Join(labDir, "testdata", "candidates")})
	if err != nil {
		t.Fatalf("load candidates: %v", err)
	}

	outDir := t.TempDir()
	engine := lab.NewEngine("baseline")
	report, err := engine.Run(context.Background(), taskSet, candidates, outDir)
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}

	proposal, err := lab.BuildProposal(report)
	if err != nil {
		t.Fatalf("build proposal: %v", err)
	}

	proposalPath := filepath.Join(t.TempDir(), "proposal.json")
	if err := lab.WriteProposalFile(proposal, proposalPath); err != nil {
		t.Fatalf("write proposal: %v", err)
	}
	raw, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}

	// The bytes the lab wrote are the bytes the operator would submit.
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/agent-harness/proposals", cookies, string(raw))
	if rec.Code != http.StatusCreated {
		t.Fatalf("the lab's proposal was not importable: %d %s", rec.Code, rec.Body.String())
	}

	doc := harnessVersionDoc(t, rec)
	if doc.Status != "draft" {
		t.Errorf("status = %q, want draft", doc.Status)
	}
	if doc.Evidence == nil {
		t.Fatal("the evidence did not survive the import")
	}
	var evidence map[string]any
	if err := json.Unmarshal(doc.Evidence, &evidence); err != nil {
		t.Fatalf("the evidence is not JSON: %v", err)
	}
	if evidence["candidate"] != report.RecommendedCandidate {
		t.Errorf("evidence names %v, want the recommended candidate %q", evidence["candidate"], report.RecommendedCandidate)
	}
}
