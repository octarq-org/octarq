package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/internal/agentstore"
	"github.com/octarq-org/octarq/server/internal/models"
)

// The harness admin API: publish, list, inspect and roll back the agent
// harness of a workspace.

func newHarnessTestHandler(t *testing.T) (http.Handler, *Handler) {
	t.Helper()
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)
	return srv, h
}

func publishSpec(t *testing.T, srv http.Handler, cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(srv, "POST", "/api/agent-harness/specs", cookies, body)
}

func TestAgentHarness_ListIsEmptyBeforeAnythingIsPublished(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/agent-harness/specs", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out struct {
		Active   *AgentHarnessVersion  `json:"active"`
		Versions []AgentHarnessVersion `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out.Active != nil {
		t.Errorf("no version was published, yet active = %+v", out.Active)
	}
	if len(out.Versions) != 0 {
		t.Errorf("expected no versions, got %d", len(out.Versions))
	}
}

func TestAgentHarness_PublishThenList(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	body := `{"version":1,"name":"default","preamble":"You are the support agent.","extraRules":["Be brief."],"maxSteps":4}`
	rec := publishSpec(t, srv, cookies, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var doc AgentHarnessSpecDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
	if doc.Fingerprint == "" {
		t.Error("the fingerprint is empty")
	}
	if doc.Spec.Preamble != "You are the support agent." {
		t.Errorf("preamble = %q", doc.Spec.Preamble)
	}

	listRec := do(srv, "GET", "/api/agent-harness/specs", cookies, "")
	var out struct {
		Active   *AgentHarnessVersion  `json:"active"`
		Versions []AgentHarnessVersion `json:"versions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, listRec.Body.String())
	}
	if out.Active == nil {
		t.Fatal("the published version is not active")
	}
	if len(out.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(out.Versions))
	}
}

func TestAgentHarness_PublishRefusesAnInvalidSpec(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := publishSpec(t, srv, cookies, `{"version":99,"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Error("the refusal carries no explanation")
	}
}

func TestAgentHarness_PublishRefusesAnUnknownField(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := publishSpec(t, srv, cookies, `{"version":1,"name":"default","skills":["do-thing"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_GetOneVersion(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := publishSpec(t, srv, cookies, `{"version":1,"name":"default","preamble":"hi"}`)
	var doc AgentHarnessSpecDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	getRec := do(srv, "GET", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10), cookies, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var got AgentHarnessSpecDocument
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Spec.Preamble != "hi" {
		t.Errorf("preamble = %q, want hi", got.Spec.Preamble)
	}
}

func TestAgentHarness_GetUnknownVersionIs404(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "GET", "/api/agent-harness/specs/9999", cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_RollbackMakesTheOldVersionActive(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	first := publishSpec(t, srv, cookies, `{"version":1,"name":"default","preamble":"first"}`)
	var firstDoc AgentHarnessSpecDocument
	if err := json.Unmarshal(first.Body.Bytes(), &firstDoc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	publishSpec(t, srv, cookies, `{"version":1,"name":"default","preamble":"second"}`)

	rollRec := do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(firstDoc.ID), 10)+"/rollback", cookies, "")
	if rollRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rollRec.Code, rollRec.Body.String())
	}

	listRec := do(srv, "GET", "/api/agent-harness/specs", cookies, "")
	var out struct {
		Active   *AgentHarnessVersion  `json:"active"`
		Versions []AgentHarnessVersion `json:"versions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Active == nil || out.Active.ID != firstDoc.ID {
		t.Fatalf("the rolled back version is not active: %+v", out.Active)
	}
}

func TestAgentHarness_RequiresAuthentication(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)

	rec := do(srv, "GET", "/api/agent-harness/specs", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_PublishRequiresAdmin(t *testing.T) {
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	const org = uint(7701)
	seedOrgMember(t, db, org, "owner@harness.test", "owner")
	memberUID := seedOrgMember(t, db, org, "member@harness.test", "member")
	memberSession := sessionCookies(t, memberUID, org)

	rec := publishSpec(t, srv, memberSession, `{"version":1,"name":"default"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a member published a harness: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_PublishedSpecReachesTheChatPath(t *testing.T) {
	llm := &fakeStreamingLLM{texts: []string{"hello"}}
	srv, db, h := newAIStreamTestHandlerFull(t, llm, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	cookies := loginCookies(t, srv)
	rec := publishSpec(t, srv, cookies, `{"version":1,"name":"default","preamble":"You are the published assistant."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	chatCookies := sessionCookies(t, 1, 1)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range chatCookies {
		req.AddCookie(c)
	}
	chatRec := httptest.NewRecorder()
	srv.ServeHTTP(chatRec, req)

	if !strings.Contains(llm.lastReq.System, "You are the published assistant.") {
		t.Errorf("the published preamble did not reach the provider:\n%s", llm.lastReq.System)
	}
}

func TestAgentHarness_BrokenStoredSpecFailsTheChatRequest(t *testing.T) {
	llm := &fakeStreamingLLM{texts: []string{"hello"}}
	srv, db, h := newAIStreamTestHandlerFull(t, llm, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	// A published row that cannot be decoded must fail the request instead
	// of silently running with the built-in default.
	if err := db.Create(&models.AgentHarnessSpec{
		OrgID:       1,
		Name:        "default",
		Version:     1,
		SpecJSON:    `{"version":1,"name":"default","skills":["do-thing"]}`,
		Fingerprint: "abc",
		Status:      models.AgentHarnessStatusPublished,
		CreatedBy:   1,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("expected the request to fail")
	}
	if llm.lastReq.System != "" {
		t.Errorf("a broken spec still reached the provider:\n%s", llm.lastReq.System)
	}
}

func TestAgentHarness_DefaultSpecIsUsedWhenNothingIsPublished(t *testing.T) {
	llm := &fakeStreamingLLM{texts: []string{"hello"}}
	srv, db, h := newAIStreamTestHandlerFull(t, llm, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range sessionCookies(t, 1, 1) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if !strings.Contains(llm.lastReq.System, harness.DefaultPreamble) {
		t.Errorf("the built-in preamble was not used:\n%s", llm.lastReq.System)
	}
}
