package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

// defaultAgent reads the workspace default over HTTP.
func readDefaultAgent(t *testing.T, srv http.Handler, cookies []*http.Cookie) DefaultAgentDTO {
	t.Helper()
	rec := do(srv, "GET", "/api/workforce/default-agent", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get default agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var body DefaultAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode default agent: %v", err)
	}
	return body
}

// TestWorkforce_DefaultAgentEndpoints covers the setting that decides which
// agent an unattributed chat turn is billed to.
func TestWorkforce_DefaultAgentEndpoints(t *testing.T) {
	srv, db, _ := newBillingTestHandler(t)
	admin := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, admin, "10")

	// A fresh workspace has no default, and any member may read that.
	if got := readDefaultAgent(t, srv, admin); got.AgentID != "" {
		t.Fatalf("new workspace default = %q, want empty", got.AgentID)
	}

	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+agent.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set default: %d (%s)", rec.Code, rec.Body.String())
	}
	// The name travels with the id: a member cannot list agents, but still has
	// to see whose budget their turns spend.
	if got := readDefaultAgent(t, srv, admin); got.AgentID != agent.ID || got.Name != "Billing Bot" {
		t.Fatalf("default = %+v, want %q named %q", got, agent.ID, "Billing Bot")
	}

	// Fail-closed inputs: a missing field is not an empty default, and an
	// unknown agent is refused.
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing agentId: expected 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"agent-missing"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown agent: expected 404, got %d (%s)", rec.Code, rec.Body.String())
	}

	// A member reads the default but cannot change it: the setting decides
	// whose budget every member spends.
	if err := db.Create(&models.OrgMember{OrgID: agent.OrgID, UserID: 2, Role: "member"}).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
	member := sessionCookies(t, 2, agent.OrgID)
	if got := readDefaultAgent(t, srv, member); got.AgentID != agent.ID || got.Name != "Billing Bot" {
		t.Fatalf("member sees default %+v, want %q named %q", got, agent.ID, "Billing Bot")
	}
	if rec := do(srv, "PUT", "/api/workforce/default-agent", member, `{"agentId":""}`); rec.Code != http.StatusForbidden {
		t.Fatalf("member set default: expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got.AgentID != agent.ID {
		t.Fatalf("member changed the default to %q", got.AgentID)
	}

	// The PUT answer carries the name too, so an admin surface repaints from
	// the response instead of refetching.
	rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+agent.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set default: %d (%s)", rec.Code, rec.Body.String())
	}
	var putBody DefaultAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &putBody); err != nil {
		t.Fatalf("decode set response: %v", err)
	}
	if putBody.AgentID != agent.ID || putBody.Name != "Billing Bot" {
		t.Fatalf("set response = %+v, want %q named %q", putBody, agent.ID, "Billing Bot")
	}

	// An empty value clears it.
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":""}`); rec.Code != http.StatusOK {
		t.Fatalf("clear default: %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got.AgentID != "" || got.Name != "" {
		t.Fatalf("default after clear = %+v, want empty", got)
	}

	// Deleting the agent clears a default that pointed at it, so no chat turn
	// is refused by a setting no page can repair.
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+agent.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set default again: %d", rec.Code)
	}
	if rec := do(srv, "DELETE", "/api/workforce/agents/"+agent.ID, admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete agent: %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got.AgentID != "" {
		t.Fatalf("default after deleting its agent = %q, want empty", got.AgentID)
	}

	// A setting that outlives its agent (a row removed behind the store) still
	// answers 200 with the bare id. The chat refuses that turn and names the
	// cause; a 500 here would make the whole drawer unopenable instead.
	orphan := createBillingAgent(t, srv, admin, "10")
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+orphan.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set orphan default: %d (%s)", rec.Code, rec.Body.String())
	}
	if err := db.Where("id = ?", orphan.ID).Delete(&models.WorkforceAgent{}).Error; err != nil {
		t.Fatalf("remove agent row: %v", err)
	}
	if got := readDefaultAgent(t, srv, admin); got.AgentID != orphan.ID || got.Name != "" {
		t.Fatalf("orphan default = %+v, want id %q and no name", got, orphan.ID)
	}
}

// TestWorkforceBilling_ChatStreamBillsTheDefaultAgent pins the case that made
// a setting necessary: a member chats from a surface with no agent picker, and
// the turn is charged to the agent the admin chose.
func TestWorkforceBilling_ChatStreamBillsTheDefaultAgent(t *testing.T) {
	srv, db, _ := newBillingTestHandler(t)
	admin := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, admin, "10")
	setModelPrice(t, srv, admin, fakeStreamModel, 1, 1)

	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+agent.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set default: %d (%s)", rec.Code, rec.Body.String())
	}

	// A member names no agent. The default answers for them.
	if err := db.Create(&models.OrgMember{OrgID: agent.OrgID, UserID: 2, Role: "member"}).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
	member := sessionCookies(t, 2, agent.OrgID)

	rec := streamAgentTurn(t, srv, member, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("member stream: %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("stream did not finish: %s", rec.Body.String())
	}
	updated := readAgent(t, srv, admin, agent.ID)
	if updated.BudgetUsed != 0.0015 {
		t.Fatalf("budgetUsed = %v, want 0.0015 billed to the default agent", updated.BudgetUsed)
	}

	// Naming another agent is still an admin action.
	if rec := streamAgentTurn(t, srv, member, agent.ID, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("member naming an agent: expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}
}
