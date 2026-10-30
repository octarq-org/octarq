package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/apierror"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/llmprovider"
	"gorm.io/gorm"
)

// fakeStreamModel is the model the fake streaming provider reports as its
// default, so a billed stream test prices exactly that identifier.
const fakeStreamModel = "fake-stream-model"

func newBillingTestHandler(t *testing.T) (http.Handler, *gorm.DB, *Handler) {
	t.Helper()
	fake := &fakeStreamingLLM{
		texts: []string{"ok"},
		resp:  llmprovider.Response{InputTokens: 1000, OutputTokens: 500},
	}
	srv, db, h := newAIStreamTestHandlerFull(t, fake, nil)
	h.SetWorkforceStore(agentstore.NewWorkforceStore(db))
	return srv, db, h
}

func createBillingAgent(t *testing.T, srv http.Handler, cookies []*http.Cookie, budgetJSON string) WorkforceAgentDTO {
	t.Helper()
	rec := do(srv, "POST", "/api/workforce/agents", cookies,
		`{"name":"Billing Bot","role":"Operator","department":"Operations","model":"`+fakeStreamModel+`","budgetLimit":`+budgetJSON+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var agent WorkforceAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agent); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	return agent
}

func setModelPrice(t *testing.T, srv http.Handler, cookies []*http.Cookie, model string, in, out float64) {
	t.Helper()
	body := `{"model":"` + model + `","inputPerMTok":` + floatJSON(in) + `,"outputPerMTok":` + floatJSON(out) + `}`
	rec := do(srv, "PUT", "/api/workforce/model-prices", cookies, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("set price %s: %d (%s)", model, rec.Code, rec.Body.String())
	}
}

func floatJSON(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func readAgent(t *testing.T, srv http.Handler, cookies []*http.Cookie, id string) WorkforceAgentDTO {
	t.Helper()
	rec := do(srv, "GET", "/api/workforce/agents/"+id, cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var agent WorkforceAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &agent); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	return agent
}

func streamAgentTurn(t *testing.T, srv http.Handler, cookies []*http.Cookie, agentID, model string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"messages":[{"role":"user","content":"hello"}]`
	if agentID != "" {
		body += `,"agentId":"` + agentID + `"`
	}
	if model != "" {
		body += `,"model":"` + model + `"`
	}
	body += `}`
	return do(srv, "POST", "/api/ai/chat/stream", cookies, body)
}

// TestWorkforceBilling_ModelPriceEndpoints covers the price CRUD over HTTP,
// including the fail-closed validations.
func TestWorkforceBilling_ModelPriceEndpoints(t *testing.T) {
	srv, _, _ := newBillingTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/workforce/model-prices", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list prices: %d (%s)", rec.Code, rec.Body.String())
	}
	var listed struct {
		Items []WorkforceModelPriceDTO `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode prices: %v", err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("a new workspace must have no prices, got %d", len(listed.Items))
	}

	setModelPrice(t, srv, cookies, "claude-opus-4-8", 3, 15)
	// The upsert replaces the values instead of adding a second row.
	setModelPrice(t, srv, cookies, "claude-opus-4-8", 4, 20)

	rec = do(srv, "GET", "/api/workforce/model-prices", cookies, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode prices: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].InputPerMTok != 4 || listed.Items[0].OutputPerMTok != 20 {
		t.Fatalf("unexpected prices after upsert: %+v", listed.Items)
	}

	for _, body := range []string{
		`{"model":"","inputPerMTok":1,"outputPerMTok":1}`,
		`{"model":"m","inputPerMTok":-1,"outputPerMTok":1}`,
		`{"model":"m","inputPerMTok":1,"outputPerMTok":-0.01}`,
	} {
		if rec := do(srv, "PUT", "/api/workforce/model-prices", cookies, body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: expected 400, got %d (%s)", body, rec.Code, rec.Body.String())
		}
	}

	if rec := do(srv, "DELETE", "/api/workforce/model-prices", cookies, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete without model: expected 400, got %d", rec.Code)
	}
	if rec := do(srv, "DELETE", "/api/workforce/model-prices?model=claude-opus-4-8", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete price: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "DELETE", "/api/workforce/model-prices?model=claude-opus-4-8", cookies, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: expected 404, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestWorkforceBilling_RecordAgentSpendEndpoint covers the ingestion path an
// external agent runtime uses to report its usage.
func TestWorkforceBilling_RecordAgentSpendEndpoint(t *testing.T) {
	srv, _, _ := newBillingTestHandler(t)
	cookies := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, cookies, "10")
	setModelPrice(t, srv, cookies, fakeStreamModel, 1, 1)

	rec := do(srv, "POST", "/api/workforce/agents/"+agent.ID+"/spend", cookies,
		`{"model":"`+fakeStreamModel+`","inputTokens":1000000,"outputTokens":0}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("record spend: %d (%s)", rec.Code, rec.Body.String())
	}
	var recorded struct {
		Event WorkforceSpendEventDTO `json:"event"`
		Agent WorkforceAgentDTO      `json:"agent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &recorded); err != nil {
		t.Fatalf("decode spend: %v", err)
	}
	if recorded.Event.CostUSD != 1 {
		t.Fatalf("cost = %v, want 1", recorded.Event.CostUSD)
	}
	if recorded.Event.Source != models.WorkforceSpendSourceRuntime {
		t.Fatalf("source = %q, want runtime", recorded.Event.Source)
	}
	if recorded.Agent.BudgetUsed != 1 {
		t.Fatalf("budgetUsed = %v, want 1", recorded.Agent.BudgetUsed)
	}

	rec = do(srv, "GET", "/api/workforce/agents/"+agent.ID+"/spend", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list spend: %d (%s)", rec.Code, rec.Body.String())
	}
	var ledger struct {
		Items []WorkforceSpendEventDTO `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ledger); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	if len(ledger.Items) != 1 {
		t.Fatalf("expected one ledger row, got %d", len(ledger.Items))
	}

	// Fail-closed inputs.
	badBodies := []string{
		`{"model":"","inputTokens":1,"outputTokens":0}`,
		`{"model":"` + fakeStreamModel + `","inputTokens":-1,"outputTokens":0}`,
		`{"model":"` + fakeStreamModel + `","inputTokens":0,"outputTokens":-1}`,
		`{"model":"` + fakeStreamModel + `","inputTokens":1,"outputTokens":1,"reasoningTokens":2}`,
		`{"model":"unpriced","inputTokens":1,"outputTokens":0}`,
	}
	for _, body := range badBodies {
		if rec := do(srv, "POST", "/api/workforce/agents/"+agent.ID+"/spend", cookies, body); rec.Code != http.StatusBadRequest {
			t.Errorf("spend %s: expected 400, got %d (%s)", body, rec.Code, rec.Body.String())
		}
	}
	if rec := do(srv, "POST", "/api/workforce/agents/agent-missing/spend", cookies,
		`{"model":"`+fakeStreamModel+`","inputTokens":1,"outputTokens":0}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown agent spend: expected 404, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "GET", "/api/workforce/agents/agent-missing/spend", cookies, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown agent ledger: expected 404, got %d", rec.Code)
	}
	if rec := do(srv, "GET", "/api/workforce/agents/"+agent.ID+"/spend?limit=501", cookies, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit above the ceiling: expected 400, got %d", rec.Code)
	}
}

// TestWorkforceBilling_ChatStreamBillsTheAgent pins the end-to-end attribution:
// a stream turn with an agentId lands in the ledger and in the agent counter.
func TestWorkforceBilling_ChatStreamBillsTheAgent(t *testing.T) {
	srv, db, _ := newBillingTestHandler(t)
	cookies := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, cookies, "10")
	setModelPrice(t, srv, cookies, fakeStreamModel, 1, 1)

	rec := streamAgentTurn(t, srv, cookies, agent.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stream: %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("stream did not finish: %s", rec.Body.String())
	}

	// 1000 input + 500 output tokens at $1/M each.
	updated := readAgent(t, srv, cookies, agent.ID)
	if updated.BudgetUsed != 0.0015 {
		t.Fatalf("budgetUsed = %v, want 0.0015", updated.BudgetUsed)
	}

	var event models.WorkforceSpendEvent
	if err := db.Where("owner_id = ? AND agent_id = ?", agent.OrgID, agent.ID).First(&event).Error; err != nil {
		t.Fatalf("ledger row missing: %v", err)
	}
	if event.Source != models.WorkforceSpendSourceChat {
		t.Fatalf("source = %q, want chat", event.Source)
	}
	if !strings.HasPrefix(event.SessionID, "sess-") {
		t.Fatalf("session id not recorded: %q", event.SessionID)
	}
	if event.InputTokens != 1000 || event.OutputTokens != 500 {
		t.Fatalf("ledger tokens = %d/%d, want 1000/500", event.InputTokens, event.OutputTokens)
	}
}

// wantErrCode pins the machine-readable code the client branches on. The
// prose message may change; the code is the contract.
func wantErrCode(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, rec.Body.String())
	}
	if body.Code != code {
		t.Fatalf("error code: got %q, want %q (%s)", body.Code, code, rec.Body.String())
	}
}

// TestWorkforceBilling_ChatStreamRefusals pins that the gate runs before the
// stream opens, so every refusal is a real HTTP status.
func TestWorkforceBilling_ChatStreamRefusals(t *testing.T) {
	srv, db, _ := newBillingTestHandler(t)
	cookies := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, cookies, "10")

	// Attribution spends workspace money, so a member cannot do it.
	if err := db.Create(&models.OrgMember{OrgID: agent.OrgID, UserID: 2, Role: "member"}).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
	memberCookies := sessionCookies(t, 2, agent.OrgID)
	if rec := streamAgentTurn(t, srv, memberCookies, agent.ID, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("member attribution: expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	// A model without a price cannot be billed.
	if rec := streamAgentTurn(t, srv, cookies, agent.ID, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unpriced model: expected 400, got %d (%s)", rec.Code, rec.Body.String())
	} else {
		wantErrCode(t, rec, apierror.CodeWorkforceModelPriceAbsent)
	}

	setModelPrice(t, srv, cookies, fakeStreamModel, 1, 1)

	// An unknown agent is a 404.
	if rec := streamAgentTurn(t, srv, cookies, "agent-missing", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown agent: expected 404, got %d", rec.Code)
	} else {
		wantErrCode(t, rec, apierror.CodeWorkforceAgentNotFound)
	}

	// A paused agent is refused even when its budget is untouched.
	if rec := do(srv, "POST", "/api/workforce/agents/"+agent.ID+"/pause", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("pause: %d", rec.Code)
	}
	if rec := streamAgentTurn(t, srv, cookies, agent.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("paused agent: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	} else {
		wantErrCode(t, rec, apierror.CodeWorkforceAgentPaused)
	}

	// An agent over its limit with auto-halt on is refused. Spend past the
	// limit with auto-halt off, then switch auto-halt on; the agent stays
	// active, so this exercises the budget check rather than the pause check.
	if rec := do(srv, "POST", "/api/workforce/budgets/auto-halt", cookies, `{"autoHalt":false}`); rec.Code != http.StatusOK {
		t.Fatalf("auto-halt off: %d", rec.Code)
	}
	if rec := do(srv, "POST", "/api/workforce/agents/"+agent.ID+"/resume", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("resume: %d", rec.Code)
	}
	if rec := do(srv, "POST", "/api/workforce/agents/"+agent.ID+"/spend", cookies,
		`{"model":"`+fakeStreamModel+`","inputTokens":20000000,"outputTokens":0}`); rec.Code != http.StatusCreated {
		t.Fatalf("spend over limit: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "POST", "/api/workforce/budgets/auto-halt", cookies, `{"autoHalt":true}`); rec.Code != http.StatusOK {
		t.Fatalf("auto-halt on: %d", rec.Code)
	}
	if rec := streamAgentTurn(t, srv, cookies, agent.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("exhausted budget: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	} else {
		wantErrCode(t, rec, apierror.CodeWorkforceBudgetExceeded)
	}
}

// TestWorkforceBilling_SpendSurvivesACancelledRequest pins the write that
// follows the stream. A client that closes the tab cancels the request
// context; the turn already ran and the provider billed it, so the ledger row
// must still be written.
func TestWorkforceBilling_SpendSurvivesACancelledRequest(t *testing.T) {
	srv, db, h := newBillingTestHandler(t)
	cookies := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, cookies, "10")
	setModelPrice(t, srv, cookies, fakeStreamModel, 3, 15)

	var row models.WorkforceAgent
	if err := db.Where("id = ?", agent.ID).First(&row).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := recordTurnSpend(ctx, h.getWorkforceStore(), row.OrgID, agentstore.SpendInput{
		AgentID:         agent.ID,
		Model:           fakeStreamModel,
		InputTokens:     1_000,
		OutputTokens:    500,
		ReasoningTokens: 100,
		Source:          models.WorkforceSpendSourceChat,
		SessionID:       "sess-cancelled",
	}); err != nil {
		t.Fatalf("recordTurnSpend on a cancelled request: %v", err)
	}

	var count int64
	if err := db.Model(&models.WorkforceSpendEvent{}).Where("agent_id = ?", agent.ID).Count(&count).Error; err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if count != 1 {
		t.Fatalf("ledger rows = %d, want 1", count)
	}
}
