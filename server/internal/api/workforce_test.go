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

// seedWorkforceAgent creates one agent so tasks have a real assignee.
func seedWorkforceAgent(t *testing.T, srv http.Handler, cookies []*http.Cookie) WorkforceAgentDTO {
	t.Helper()
	rec := do(srv, "POST", "/api/workforce/agents", cookies,
		`{"name":"Ops Bot","role":"Operator","department":"Operations","model":"GPT-4o","budgetLimit":10}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var agent WorkforceAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agent); err != nil {
		t.Fatalf("decode seeded agent: %v", err)
	}
	return agent
}

func TestWorkforce_ListAgentsStartsEmptyAndReturnsCreated(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	// A new workspace has no fabricated fleet.
	rec := do(srv, "GET", "/api/workforce/agents", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var empty struct {
		Items []WorkforceAgentDTO `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(empty.Items) != 0 {
		t.Fatalf("expected 0 agents in a new workspace, got %d", len(empty.Items))
	}

	// A created agent is persisted and listed.
	rec = do(srv, "POST", "/api/workforce/agents", cookies, `{"name":"Ops Bot","role":"Operator","department":"Operations","model":"GPT-4o","budgetLimit":10}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(srv, "GET", "/api/workforce/agents", cookies, "")
	var res struct {
		Items []WorkforceAgentDTO `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(res.Items))
	}
}

func TestWorkforce_CreateAndUpdateAgent(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	// Create Agent
	createPayload := `{"name":"QA Automation Specialist","role":"Test Engineer","department":"Quality Assurance","model":"Claude 3.5 Sonnet","budgetLimit":15,"directives":"Run regression test suite on build."}`
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

	rec := do(srv, "POST", "/api/workforce/agents", cookies, `{"name":"Lead Orchestrator","role":"Chief Operational Assistant","department":"Executive","model":"GPT-4o","budgetLimit":10}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var created WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// Pause the agent
	rec = do(srv, "POST", "/api/workforce/agents/"+created.ID+"/pause", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var paused WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &paused)
	if paused.Status != "paused" {
		t.Errorf("expected paused, got %s", paused.Status)
	}

	// Resume the agent
	rec = do(srv, "POST", "/api/workforce/agents/"+created.ID+"/resume", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resumed WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &resumed)
	if resumed.Status != "active" {
		t.Errorf("expected active, got %s", resumed.Status)
	}

	// Trigger heartbeat
	rec = do(srv, "POST", "/api/workforce/agents/"+created.ID+"/heartbeat", cookies, "")
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

	// A new workspace has no fabricated task history.
	rec := do(srv, "GET", "/api/workforce/tasks", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listRes struct {
		Items []WorkforceTaskDTO `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listRes)
	if len(listRes.Items) != 0 {
		t.Fatalf("expected 0 tasks in a new workspace, got %d", len(listRes.Items))
	}

	agent := seedWorkforceAgent(t, srv, cookies)

	// Create custom task
	taskPayload := `{"title":"Optimize SQLite query plan for agentstore","summary":"Add composite index and analyze query plan.","priority":"high","assigneeAgentId":"` + agent.ID + `"}`
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

	// A ticket must be worked before it can be finished, and the assignee
	// shown on it comes from the agent row.
	if createdTask.AssigneeAgentID != agent.ID || createdTask.Assignee != agent.Name {
		t.Errorf("task must carry the resolved assignee, got %q / %q", createdTask.AssigneeAgentID, createdTask.Assignee)
	}
	rec = do(srv, "PATCH", "/api/workforce/tasks/"+createdTask.ID, cookies, `{"status":"done"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("todo -> done must conflict, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(srv, "PATCH", "/api/workforce/tasks/"+createdTask.ID, cookies, `{"status":"in_progress"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("start task: %d: %s", rec.Code, rec.Body.String())
	}
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

	// A new workspace reports no agents and no spend.
	rec := do(srv, "GET", "/api/workforce/overview", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var overview WorkforceOverviewDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if overview.ActiveAgents != 0 || overview.MonthlySpend != 0 {
		t.Errorf("expected a zeroed overview for a new workspace, got %+v", overview)
	}
	if !overview.AutoHalt {
		t.Errorf("expected default autoHalt to be true")
	}

	// Two agents give the overview real numbers to aggregate.
	var ops WorkforceAgentDTO
	for i, body := range []string{
		`{"name":"Ops Bot","role":"Operator","department":"Operations","model":"GPT-4o","budgetLimit":20}`,
		`{"name":"Audit Bot","role":"Auditor","department":"Security","model":"Claude 3.5 Sonnet","budgetLimit":10}`,
	} {
		rec := do(srv, "POST", "/api/workforce/agents", cookies, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create agent: %d (%s)", rec.Code, rec.Body.String())
		}
		if i == 0 {
			if err := json.Unmarshal(rec.Body.Bytes(), &ops); err != nil {
				t.Fatalf("decode agent: %v", err)
			}
		}
	}

	rec = do(srv, "GET", "/api/workforce/overview", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if overview.ActiveAgents != 2 {
		t.Errorf("expected 2 active agents, got %d", overview.ActiveAgents)
	}
	if overview.MonthlyLimit != 30 {
		t.Errorf("expected 30 monthly limit, got %f", overview.MonthlyLimit)
	}
	// Both agents appear in the breakdown. No turn has been billed yet, so no
	// provider has spend to report: the provider rows come from the ledger.
	if len(overview.SpendByAgent) != 2 || len(overview.SpendByProvider) != 0 {
		t.Errorf("expected two agents and no provider spend, got agents=%d providers=%d", len(overview.SpendByAgent), len(overview.SpendByProvider))
	}

	// One priced, billed turn attributes its cost to the provider that was
	// actually billed.
	if rec := do(srv, "PUT", "/api/workforce/model-prices", cookies,
		`{"model":"GPT-4o","inputPerMTok":5,"outputPerMTok":15}`); rec.Code != http.StatusOK {
		t.Fatalf("set model price: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "POST", "/api/workforce/agents/"+ops.ID+"/spend", cookies,
		`{"model":"GPT-4o","inputTokens":1000000,"outputTokens":0,"turnId":"turn-overview-1"}`); rec.Code != http.StatusCreated {
		t.Fatalf("record spend: %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "GET", "/api/workforce/overview", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if overview.MonthlySpend != 5 {
		t.Errorf("expected monthly spend 5, got %f", overview.MonthlySpend)
	}
	if len(overview.SpendByProvider) != 1 || overview.SpendByProvider[0].Provider != "OpenAI" {
		t.Errorf("expected one OpenAI provider row, got %+v", overview.SpendByProvider)
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

func TestWorkforce_TaskApprovalGate(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)
	cookies := loginCookies(t, srv)

	agent := seedWorkforceAgent(t, srv, cookies)

	// A task that is not gated cannot be decided.
	rec := do(srv, "POST", "/api/workforce/tasks", cookies, `{"title":"Revoke stale tokens","summary":"Prepared revocation batch.","assigneeAgentId":"`+agent.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task: %d (%s)", rec.Code, rec.Body.String())
	}
	var task WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &task)

	rec = do(srv, "POST", "/api/workforce/approvals/"+task.ID, cookies, `{"decision":"approve"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for an ungated task, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Gate it.
	rec = do(srv, "PATCH", "/api/workforce/tasks/"+task.ID, cookies, `{"status":"awaiting_approval"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("gate task: %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(srv, "GET", "/api/workforce/approvals", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list approvals: %d (%s)", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []WorkforceTaskDTO `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Items) != 1 || list.Items[0].ID != task.ID {
		t.Fatalf("expected the gated task in the approvals list, got %+v", list.Items)
	}

	// Approve resumes the work.
	rec = do(srv, "POST", "/api/workforce/approvals/"+task.ID, cookies, `{"decision":"approve"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d (%s)", rec.Code, rec.Body.String())
	}
	var approved WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &approved)
	if approved.Status != "in_progress" {
		t.Errorf("expected in_progress after approval, got %s", approved.Status)
	}

	// A replayed decision is refused.
	rec = do(srv, "POST", "/api/workforce/approvals/"+task.ID, cookies, `{"decision":"approve"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a replayed decision, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Rejection returns a gated task to the queue.
	rec = do(srv, "POST", "/api/workforce/tasks", cookies, `{"title":"Rotate DNS keys","summary":"Awaiting sign-off.","assigneeAgentId":"`+agent.ID+`"}`)
	var second WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	_ = do(srv, "PATCH", "/api/workforce/tasks/"+second.ID, cookies, `{"status":"awaiting_approval"}`)
	rec = do(srv, "POST", "/api/workforce/approvals/"+second.ID, cookies, `{"decision":"reject"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: %d (%s)", rec.Code, rec.Body.String())
	}
	var rejected WorkforceTaskDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &rejected)
	if rejected.Status != "todo" {
		t.Errorf("expected todo after rejection, got %s", rejected.Status)
	}
}

func TestOrgIsolation_Workforce(t *testing.T) {
	srv, _ := newWorkforceTestHandler(t)

	cookiesOrg1 := sessionCookies(t, 1, 1)
	cookiesOrg2 := sessionCookies(t, 2, 2)

	// Org 1 creates an agent
	createAgentPayload := `{"name":"Org1 Agent","role":"Specialist","department":"Operations","model":"GPT-4o","budgetLimit":10,"directives":"Strictly org 1 only."}`
	rec := do(srv, "POST", "/api/workforce/agents", cookiesOrg1, createAgentPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent in org 1: %d (%s)", rec.Code, rec.Body.String())
	}
	var org1Agent WorkforceAgentDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &org1Agent)

	// Org 1 creates a task
	createTaskPayload := `{"title":"Org1 Secret Task","summary":"Confidential payload","assigneeAgentId":"` + org1Agent.ID + `"}`
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
