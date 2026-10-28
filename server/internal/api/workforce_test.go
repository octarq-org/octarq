package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/agentstore"
)

func newWorkforceTestHandler(t *testing.T) (http.Handler, *Handler) {
	t.Helper()
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewWorkforceStore(db)
	h.SetWorkforceStore(store)
	return srv, h
}

func TestWorkforce_ListAgentsAutoSeeded(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/workforce/agents", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Items []WorkforceAgentDTO `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(res.Items) != 4 {
		t.Fatalf("expected 4 seeded agents, got %d", len(res.Items))
	}
}

func TestWorkforce_CreateAndUpdateAgent(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	// Create Agent
	createPayload := `{"name":"QA Automation Specialist","role":"Test Engineer","department":"Quality Assurance","model":"Claude 3.5 Sonnet","directives":"Run regression test suite on build."}`
	rec := do(srv, "POST", "/api/workforce/agents", cookies, createPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created WorkforceAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.Name != "QA Automation Specialist" {
		t.Errorf("expected QA Automation Specialist, got %s", created.Name)
	}

	// Update Directives
	updatePayload := `{"directives":"Run regression and race test suites on build."}`
	rec = do(srv, "PATCH", "/api/workforce/agents/"+created.ID, cookies, updatePayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated WorkforceAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if updated.Directives != "Run regression and race test suites on build." {
		t.Errorf("expected updated directives, got %s", updated.Directives)
	}

	// Delete Agent
	rec = do(srv, "DELETE", "/api/workforce/agents/"+created.ID, cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Subsequent GET returns 404
	rec = do(srv, "GET", "/api/workforce/agents/"+created.ID, cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rec.Code)
	}
}

func TestWorkforce_AgentStateTransitions(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	// Trigger seeding
	_ = do(srv, "GET", "/api/workforce/agents", cookies, "")

	// Pause lead-orchestrator
	rec := do(srv, "POST", "/api/workforce/agents/lead-orchestrator/pause", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var paused WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &paused)
	if paused.Status != "paused" {
		t.Errorf("expected paused, got %s", paused.Status)
	}

	// Resume lead-orchestrator
	rec = do(srv, "POST", "/api/workforce/agents/lead-orchestrator/resume", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resumed WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &resumed)
	if resumed.Status != "active" {
		t.Errorf("expected active, got %s", resumed.Status)
	}

	// Trigger heartbeat
	rec = do(srv, "POST", "/api/workforce/agents/lead-orchestrator/heartbeat", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var executing WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &executing)
	if executing.Status != "executing" {
		t.Errorf("expected executing, got %s", executing.Status)
	}
}

func TestWorkforce_TaskLifecycleAndStepLogs(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	// List seeded tasks
	rec := do(srv, "GET", "/api/workforce/tasks", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listRes struct {
		Items []WorkforceTaskDTO `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listRes)
	if len(listRes.Items) != 4 {
		t.Fatalf("expected 4 seeded tasks, got %d", len(listRes.Items))
	}

	// Create custom task
	taskPayload := `{"title":"Optimize SQLite query plan for agentstore","summary":"Add composite index and analyze query plan.","priority":"high"}`
	rec = do(srv, "POST", "/api/workforce/tasks", cookies, taskPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createdTask WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &createdTask)

	// Append step log
	stepPayload := `{"stepText":"Ran EXPLAIN QUERY PLAN on workforce_agents table"}`
	rec = do(srv, "POST", "/api/workforce/tasks/"+createdTask.ID+"/steps", cookies, stepPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Get task details
	rec = do(srv, "GET", "/api/workforce/tasks/"+createdTask.ID, cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var fetchedTask WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &fetchedTask)
	if len(fetchedTask.ExecutionSteps) != 1 {
		t.Errorf("expected 1 execution step, got %d", len(fetchedTask.ExecutionSteps))
	}

	// Update task status
	updateTaskPayload := `{"status":"done"}`
	rec = do(srv, "PATCH", "/api/workforce/tasks/"+createdTask.ID, cookies, updateTaskPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updatedTask WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &updatedTask)
	if updatedTask.Status != "done" {
		t.Errorf("expected done, got %s", updatedTask.Status)
	}
}

func TestWorkforce_OverviewAndAutoHaltEndpoints(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	// Get overview
	rec := do(srv, "GET", "/api/workforce/overview", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var overview WorkforceOverviewDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if overview.ActiveAgents != 4 {
		t.Errorf("expected 4 active agents, got %d", overview.ActiveAgents)
	}
	if !overview.AutoHalt {
		t.Errorf("expected default autoHalt to be true")
	}

	// Toggle auto-halt
	togglePayload := `{"autoHalt":false}`
	rec = do(srv, "POST", "/api/workforce/budgets/auto-halt", cookies, togglePayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(srv, "GET", "/api/workforce/overview", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if overview.AutoHalt {
		t.Errorf("expected autoHalt to be false after toggle")
	}
}

func TestWorkforce_UnauthenticatedFailClosed(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)

	// No session cookies
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/workforce/agents"},
		{"POST", "/api/workforce/agents"},
		{"GET", "/api/workforce/tasks"},
		{"POST", "/api/workforce/tasks"},
		{"GET", "/api/workforce/overview"},
		{"POST", "/api/workforce/budgets/auto-halt"},
	}

	for _, ep := range endpoints {
		rec := do(srv, ep.method, ep.path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for %s %s, got %d", ep.method, ep.path, rec.Code)
		}
	}
}

func TestOrgIsolation_Workforce(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)

	cookiesOrg1 := sessionCookies(t, 1, 1)
	cookiesOrg2 := sessionCookies(t, 2, 2)

	// Org 1 creates an agent
	createAgentPayload := `{"name":"Org1 Agent","role":"Specialist","model":"GPT-4o","directives":"Strictly org 1 only."}`
	rec := do(srv, "POST", "/api/workforce/agents", cookiesOrg1, createAgentPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent in org 1: %d (%s)", rec.Code, rec.Body.String())
	}
	var org1Agent WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &org1Agent)

	// Org 1 creates a task
	createTaskPayload := `{"title":"Org1 Secret Task","summary":"Confidential payload"}`
	rec = do(srv, "POST", "/api/workforce/tasks", cookiesOrg1, createTaskPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task in org 1: %d (%s)", rec.Code, rec.Body.String())
	}
	var org1Task WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &org1Task)

	// Org 2 attempts to GET Org 1's agent -> MUST return 404 (zero existence disclosure)
	rec = do(srv, "GET", "/api/workforce/agents/"+org1Agent.ID, cookiesOrg2, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when reading foreign agent, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Org 2 attempts to PATCH Org 1's agent -> MUST return 404
	rec = do(srv, "PATCH", "/api/workforce/agents/"+org1Agent.ID, cookiesOrg2, `{"directives":"Hacked"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when mutating foreign agent, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Org 2 attempts to DELETE Org 1's agent -> MUST return 404
	rec = do(srv, "DELETE", "/api/workforce/agents/"+org1Agent.ID, cookiesOrg2, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when deleting foreign agent, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Org 2 attempts to GET Org 1's task -> MUST return 404
	rec = do(srv, "GET", "/api/workforce/tasks/"+org1Task.ID, cookiesOrg2, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when reading foreign task, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Org 2 attempts to append step to Org 1's task -> MUST return 404
	rec = do(srv, "POST", "/api/workforce/tasks/"+org1Task.ID+"/steps", cookiesOrg2, `{"stepText":"Injected"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when appending step to foreign task, got %d (%s)", rec.Code, rec.Body.String())
	}
}
