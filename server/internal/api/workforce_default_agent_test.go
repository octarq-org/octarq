package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

// defaultAgent reads the workspace default over HTTP.
func readDefaultAgent(t *testing.T, srv http.Handler, cookies []*http.Cookie) string {
	t.Helper()
	rec := do(srv, "GET", "/api/workforce/default-agent", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get default agent: %d (%s)", rec.Code, rec.Body.String())
	}
	var body DefaultAgentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode default agent: %v", err)
	}
	return body.AgentID
}

// TestWorkforce_DefaultAgentEndpoints covers the setting that decides which
// agent an unattributed chat turn is billed to.
func TestWorkforce_DefaultAgentEndpoints(t *testing.T) {
	srv, db, _ := newBillingTestHandler(t)
	admin := loginCookies(t, srv)
	agent := createBillingAgent(t, srv, admin, "10")

	// A fresh workspace has no default, and any member may read that.
	if got := readDefaultAgent(t, srv, admin); got != "" {
		t.Fatalf("new workspace default = %q, want empty", got)
	}

	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+agent.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set default: %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got != agent.ID {
		t.Fatalf("default = %q, want %q", got, agent.ID)
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
	if got := readDefaultAgent(t, srv, member); got != agent.ID {
		t.Fatalf("member sees default %q, want %q", got, agent.ID)
	}
	if rec := do(srv, "PUT", "/api/workforce/default-agent", member, `{"agentId":""}`); rec.Code != http.StatusForbidden {
		t.Fatalf("member set default: expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got != agent.ID {
		t.Fatalf("member changed the default to %q", got)
	}

	// An empty value clears it.
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":""}`); rec.Code != http.StatusOK {
		t.Fatalf("clear default: %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got != "" {
		t.Fatalf("default after clear = %q, want empty", got)
	}

	// Deleting the agent clears a default that pointed at it, so no chat turn
	// is refused by a setting no page can repair.
	if rec := do(srv, "PUT", "/api/workforce/default-agent", admin, `{"agentId":"`+agent.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("set default again: %d", rec.Code)
	}
	if rec := do(srv, "DELETE", "/api/workforce/agents/"+agent.ID, admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete agent: %d (%s)", rec.Code, rec.Body.String())
	}
	if got := readDefaultAgent(t, srv, admin); got != "" {
		t.Fatalf("default after deleting its agent = %q, want empty", got)
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
