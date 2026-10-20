package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/models"
)

// The harness API's refusals are part of its contract: a workspace that
// published nothing still has a list, a version that does not exist is a 404,
// a lesson that does not exist is a 404, and a store that is not wired is a
// 500 rather than a silent empty list.

func TestAgentHarness_ErrorsAreRefusedNotSwallowed(t *testing.T) {
	srv, _, h := newAIStreamTestHandlerFull(t, nil, nil)
	// No store wired: every endpoint must say so instead of pretending the
	// workspace has no versions.
	cookies := loginCookies(t, srv)

	if rec := do(srv, "GET", "/api/agent-harness/specs", cookies, ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("list without a store = %d, want 500", rec.Code)
	}
	if rec := do(srv, "GET", "/api/agent-harness/specs/1", cookies, ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("get without a store = %d, want 500", rec.Code)
	}
	if rec := stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default"}}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("stage without a store = %d, want 500", rec.Code)
	}
	if rec := do(srv, "POST", "/api/agent-harness/proposals", cookies, `{"name":"x","spec":{"version":1,"name":"default"}}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("import without a store = %d, want 500", rec.Code)
	}
	if rec := do(srv, "GET", "/api/agent-harness/lessons", cookies, ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("lessons without a store = %d, want 500", rec.Code)
	}
	_ = h
}

func TestAgentHarness_UnknownVersionAndLessonAre404(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	if rec := do(srv, "GET", "/api/agent-harness/specs/4242", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("get an unknown version = %d, want 404", rec.Code)
	}
	if rec := do(srv, "POST", "/api/agent-harness/specs/4242/activate", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("activate an unknown version = %d, want 404", rec.Code)
	}
	if rec := do(srv, "POST", "/api/agent-harness/specs/4242/discard", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("discard an unknown version = %d, want 404", rec.Code)
	}
	if rec := do(srv, "POST", "/api/agent-harness/lessons/4242/dismiss", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("dismiss an unknown lesson = %d, want 404", rec.Code)
	}
}

// A version whose stored document cannot be decoded is a broken row: the API
// says so instead of handing out the default spec under its name.
func TestAgentHarness_ABrokenVersionIsReported(t *testing.T) {
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)
	cookies := loginCookies(t, srv)

	if err := db.Create(&models.AgentHarnessSpec{
		OrgID:       1,
		Name:        "default",
		Version:     1,
		SpecJSON:    `{"version":1,"name":"default","skills":["do-thing"]}`,
		Fingerprint: "abc",
		Status:      models.AgentHarnessStatusDraft,
		CreatedBy:   1,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := do(srv, "GET", "/api/agent-harness/specs/1", cookies, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("get a broken version = %d, want 500: %s", rec.Code, rec.Body.String())
	}
}

// Evidence that is not valid JSON is still handed out as JSON: the caller
// must never receive a body it cannot parse.
func TestAgentHarness_UnreadableEvidenceIsStillValidJSON(t *testing.T) {
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)
	cookies := loginCookies(t, srv)

	row, err := store.Stage(context.Background(), 1, harness.HarnessSpec{
		Version: harness.HarnessSpecVersion,
		Name:    "default",
	}, 1)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := db.Model(&models.AgentHarnessSpec{}).Where("id = ?", row.ID).
		Update("evidence", "not json at all").Error; err != nil {
		t.Fatalf("seed evidence: %v", err)
	}

	rec := do(srv, "GET", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(row.ID), 10), cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var doc AgentHarnessSpecDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the response is not JSON: %v (%s)", err, rec.Body.String())
	}
	if doc.Evidence == nil {
		t.Fatal("the unreadable evidence was dropped instead of being reported")
	}
	var asText string
	if err := json.Unmarshal(doc.Evidence, &asText); err != nil {
		t.Fatalf("the evidence is neither a document nor a string: %v", err)
	}
	if asText != "not json at all" {
		t.Errorf("evidence = %q, want the raw text", asText)
	}
}
