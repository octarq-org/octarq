package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/internal/agentstore"
)

// The lab writes the proposal; the admin API imports it. The bytes the lab
// produces are pinned in a golden file (server/agent/lab/testdata/proposal.golden.json),
// and this test imports exactly those bytes.
//
// The golden file is the contract: if the lab changes the shape, its own
// golden test fails; if the API stops understanding that shape, this test
// fails. Neither side can drift alone.
func TestAgentHarness_TheLabProposalIsImportable(t *testing.T) {
	goldenPath := filepath.Join("..", "..", "agent", "lab", "testdata", "proposal.golden.json")
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read the golden proposal: %v", err)
	}

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
		t.Errorf("status = %q, want draft — a proposal must not go live by itself", doc.Status)
	}
	if doc.Evidence == nil {
		t.Fatal("the evidence did not survive the import")
	}
	var evidence map[string]any
	if err := json.Unmarshal(doc.Evidence, &evidence); err != nil {
		t.Fatalf("the evidence is not JSON: %v", err)
	}
	if evidence["candidate"] != "candidate-optimized" {
		t.Errorf("evidence names %v, want the recommended candidate", evidence["candidate"])
	}
}
