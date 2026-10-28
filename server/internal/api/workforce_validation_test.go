package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/models"
)

// These tests pin the fail-closed validation of the workforce API. Every
// rejection below protects a database invariant: an empty required field, an
// open-vocabulary status, an assignee from another workspace, or a status
// change the ticket state machine does not allow.
func TestWorkforce_RejectsInvalidAgentPayloads(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	createCases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","role":"Operator","department":"Ops","model":"GPT-4o","budgetLimit":10}`},
		{"empty role", `{"name":"Bot","role":"","department":"Ops","model":"GPT-4o","budgetLimit":10}`},
		{"empty department", `{"name":"Bot","role":"Operator","department":"","model":"GPT-4o","budgetLimit":10}`},
		{"empty model", `{"name":"Bot","role":"Operator","department":"Ops","model":"","budgetLimit":10}`},
		{"zero budget", `{"name":"Bot","role":"Operator","department":"Ops","model":"GPT-4o","budgetLimit":0}`},
		{"negative budget", `{"name":"Bot","role":"Operator","department":"Ops","model":"GPT-4o","budgetLimit":-1}`},
	}
	for _, tc := range createCases {
		if rec := do(srv, "POST", "/api/workforce/agents", cookies, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}

	rec := do(srv, "POST", "/api/workforce/agents", cookies, `{"name":"Bot","role":"Operator","department":"Ops","model":"GPT-4o","budgetLimit":10}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var agent WorkforceAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agent); err != nil {
		t.Fatalf("decode agent: %v", err)
	}

	updateCases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":""}`},
		{"empty role", `{"role":""}`},
		{"empty department", `{"department":""}`},
		{"empty model", `{"model":""}`},
		{"zero budget", `{"budgetLimit":0}`},
		{"unknown status", `{"status":"sleeping"}`},
	}
	for _, tc := range updateCases {
		if rec := do(srv, "PATCH", "/api/workforce/agents/"+agent.ID, cookies, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("update %s: expected 400, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}

	// Every field in the open-ended set is accepted when valid.
	if rec := do(srv, "PATCH", "/api/workforce/agents/"+agent.ID, cookies,
		`{"name":"Ops Bot 2","role":"Senior Operator","department":"Platform","model":"Claude 3.5 Sonnet","directives":"Watch the queue.","heartbeat":"Every 5m","capabilities":["links","dns"],"status":"idle","budgetLimit":25}`); rec.Code != http.StatusOK {
		t.Fatalf("valid update: %d (%s)", rec.Code, rec.Body.String())
	}

	// An unknown agent answers 404, never a fabricated success.
	for _, path := range []string{
		"/api/workforce/agents/agent-missing",
		"/api/workforce/agents/agent-missing/pause",
		"/api/workforce/agents/agent-missing/resume",
		"/api/workforce/agents/agent-missing/heartbeat",
	} {
		method := http.MethodGet
		if path != "/api/workforce/agents/agent-missing" {
			method = http.MethodPost
		}
		if rec := do(srv, method, path, cookies, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: expected 404, got %d (%s)", method, path, rec.Code, rec.Body.String())
		}
	}
	if rec := do(srv, "DELETE", "/api/workforce/agents/agent-missing", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing agent: expected 404, got %d", rec.Code)
	}

	// An agent that still owns a ticket cannot be deleted.
	if rec := do(srv, "POST", "/api/workforce/tasks", cookies,
		`{"title":"Keep me","summary":"Still assigned.","assigneeAgentId":"`+agent.ID+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create task: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "DELETE", "/api/workforce/agents/"+agent.ID, cookies, ""); rec.Code != http.StatusConflict {
		t.Errorf("delete agent with tasks: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestWorkforce_RejectsInvalidTaskPayloads(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)
	agent := seedWorkforceAgent(t, srv, cookies)

	createCases := []struct {
		name string
		body string
	}{
		// Huma refuses an absent field before the handler runs, so the
		// handler's own guards are exercised with empty strings.
		{"empty summary", `{"title":"T","summary":"","assigneeAgentId":"` + agent.ID + `"}`},
		{"empty title", `{"title":"","summary":"S","assigneeAgentId":"` + agent.ID + `"}`},
		{"empty assignee", `{"title":"T","summary":"S","assigneeAgentId":""}`},
		{"unknown priority", `{"title":"T","summary":"S","priority":"whenever","assigneeAgentId":"` + agent.ID + `"}`},
		{"unknown assignee", `{"title":"T","summary":"S","assigneeAgentId":"agent-missing"}`},
	}
	for _, tc := range createCases {
		if rec := do(srv, "POST", "/api/workforce/tasks", cookies, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}

	if rec := do(srv, "GET", "/api/workforce/tasks?status=all", cookies, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown status filter: expected 400, got %d", rec.Code)
	}

	rec := do(srv, "POST", "/api/workforce/tasks", cookies,
		`{"title":"Audit keys","summary":"Rotate stale keys.","assigneeAgentId":"`+agent.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task: %d (%s)", rec.Code, rec.Body.String())
	}
	var task WorkforceTaskDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode task: %v", err)
	}

	updateCases := []struct {
		name string
		body string
	}{
		{"empty title", `{"title":""}`},
		{"empty summary", `{"summary":""}`},
		{"unknown priority", `{"priority":"whenever"}`},
		{"unknown status", `{"status":"blocked"}`},
		{"no fields", `{}`},
	}
	for _, tc := range updateCases {
		if rec := do(srv, "PATCH", "/api/workforce/tasks/"+task.ID, cookies, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("update %s: expected 400, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}

	// Non-status fields update without moving the ticket.
	rec = do(srv, "PATCH", "/api/workforce/tasks/"+task.ID, cookies, `{"context":"Inherited from the audit backlog.","priority":"high","summary":"Rotate the stale keys tonight."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update context: %d (%s)", rec.Code, rec.Body.String())
	}
	var updated WorkforceTaskDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated task: %v", err)
	}
	if updated.Context != "Inherited from the audit backlog." || updated.Status != "todo" {
		t.Errorf("expected the context to change and the status to stay todo, got %q / %q", updated.Context, updated.Status)
	}

	// Unknown tickets answer 404 for both update paths.
	if rec := do(srv, "PATCH", "/api/workforce/tasks/AGT-missing", cookies, `{"title":"T"}`); rec.Code != http.StatusNotFound {
		t.Errorf("update missing task: expected 404, got %d", rec.Code)
	}
	if rec := do(srv, "PATCH", "/api/workforce/tasks/AGT-missing", cookies, `{"status":"in_progress"}`); rec.Code != http.StatusNotFound {
		t.Errorf("move missing task: expected 404, got %d", rec.Code)
	}
	if rec := do(srv, "GET", "/api/workforce/tasks/AGT-missing", cookies, ""); rec.Code != http.StatusNotFound {
		t.Errorf("read missing task: expected 404, got %d", rec.Code)
	}

	// Step logs are validated too.
	if rec := do(srv, "POST", "/api/workforce/tasks/"+task.ID+"/steps", cookies, `{"stepText":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty step text: expected 400, got %d", rec.Code)
	}
	if rec := do(srv, "POST", "/api/workforce/tasks/"+task.ID+"/steps", cookies, `{"stepText":"Ran","status":"unknown"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown step status: expected 400, got %d", rec.Code)
	}

	// An approval decision is a closed vocabulary, and an unknown ticket is 404.
	if rec := do(srv, "POST", "/api/workforce/approvals/"+task.ID, cookies, `{"decision":"maybe"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown decision: expected 400, got %d", rec.Code)
	}
	if rec := do(srv, "POST", "/api/workforce/approvals/AGT-missing", cookies, `{"decision":"approve"}`); rec.Code != http.StatusNotFound {
		t.Errorf("decide missing task: expected 404, got %d", rec.Code)
	}

	// A missing spend switch is refused rather than read as "off".
	if rec := do(srv, "POST", "/api/workforce/budgets/auto-halt", cookies, `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing autoHalt: expected 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestWorkforce_MemberForbidden pins the role gate separately from the store:
// a member cannot read or change the fleet.
func TestWorkforce_MemberForbidden(t *testing.T) {
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	h.SetWorkforceStore(agentstore.NewWorkforceStore(db))

	if err := db.Create(&models.User{ID: 3, Email: "member@example.com"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	cookies := sessionCookies(t, 3, 1)
	// The session helper ensures a membership. Narrow it to member so the role
	// gate, not a missing row, is what the request meets.
	if err := db.Model(&models.OrgMember{}).
		Where("org_id = ? AND user_id = ?", 1, 3).
		Update("role", "member").Error; err != nil {
		t.Fatalf("demote membership: %v", err)
	}

	for _, ep := range []struct{ method, path, body string }{
		{"GET", "/api/workforce/agents", ""},
		{"POST", "/api/workforce/agents", `{"name":"Bot","role":"Operator","department":"Ops","model":"GPT-4o","budgetLimit":10}`},
		{"GET", "/api/workforce/agents/agent-x", ""},
		{"PATCH", "/api/workforce/agents/agent-x", `{"name":"Bot"}`},
		{"POST", "/api/workforce/agents/agent-x/pause", ""},
		{"POST", "/api/workforce/agents/agent-x/resume", ""},
		{"POST", "/api/workforce/agents/agent-x/heartbeat", ""},
		{"DELETE", "/api/workforce/agents/agent-x", ""},
		{"GET", "/api/workforce/tasks", ""},
		{"POST", "/api/workforce/tasks", `{"title":"T","summary":"S","assigneeAgentId":"agent-x"}`},
		{"GET", "/api/workforce/tasks/AGT-x", ""},
		{"PATCH", "/api/workforce/tasks/AGT-x", `{"title":"T"}`},
		{"POST", "/api/workforce/tasks/AGT-x/steps", `{"stepText":"Ran"}`},
		{"GET", "/api/workforce/approvals", ""},
		{"POST", "/api/workforce/approvals/AGT-x", `{"decision":"approve"}`},
		{"GET", "/api/workforce/overview", ""},
		{"POST", "/api/workforce/budgets/auto-halt", `{"autoHalt":true}`},
	} {
		if rec := do(srv, ep.method, ep.path, cookies, ep.body); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403, got %d", ep.method, ep.path, rec.Code)
		}
	}
}
