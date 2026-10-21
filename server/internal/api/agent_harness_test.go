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

// The harness admin API: stage a version, read it, activate it, throw a draft
// away, import a lab proposal and keep the record of what was learned.

func newHarnessTestHandler(t *testing.T) (http.Handler, *Handler) {
	t.Helper()
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)
	return srv, h
}

func stageSpec(t *testing.T, srv http.Handler, cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(srv, "POST", "/api/agent-harness/drafts", cookies, body)
}

func harnessVersionDoc(t *testing.T, rec *httptest.ResponseRecorder) AgentHarnessSpecDocument {
	t.Helper()
	var doc AgentHarnessSpecDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return doc
}

func TestAgentHarness_ListIsEmptyBeforeAnythingIsStaged(t *testing.T) {
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
		t.Errorf("nothing was activated, yet active = %+v", out.Active)
	}
	if len(out.Versions) != 0 {
		t.Errorf("expected no versions, got %d", len(out.Versions))
	}
}

func TestAgentHarness_StagingDoesNotMakeTheVersionLive(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"You are the support agent."}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	doc := harnessVersionDoc(t, rec)
	if doc.Status != "draft" {
		t.Errorf("status = %q, want draft", doc.Status)
	}
	if doc.PublishedAt != nil {
		t.Error("a draft has no publish time")
	}

	listRec := do(srv, "GET", "/api/agent-harness/specs", cookies, "")
	var out struct {
		Active   *AgentHarnessVersion  `json:"active"`
		Versions []AgentHarnessVersion `json:"versions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Active != nil {
		t.Errorf("a staged draft became live: %+v", out.Active)
	}
	if len(out.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(out.Versions))
	}
}

func TestAgentHarness_ActivateMakesTheVersionLive(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	staged := stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"first"}}`)
	doc := harnessVersionDoc(t, staged)

	rec := do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10)+"/activate", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	listRec := do(srv, "GET", "/api/agent-harness/specs", cookies, "")
	var out struct {
		Active   *AgentHarnessVersion  `json:"active"`
		Versions []AgentHarnessVersion `json:"versions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Active == nil {
		t.Fatal("the activated version is not live")
	}
	if out.Active.ID != doc.ID {
		t.Errorf("active = %d, want %d", out.Active.ID, doc.ID)
	}
	if out.Active.PublishedAt == nil {
		t.Error("the live version has no publish time")
	}
}

func TestAgentHarness_ActivatingOneVersionArchivesTheOther(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	first := harnessVersionDoc(t, stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"first"}}`))
	do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(first.ID), 10)+"/activate", cookies, "")

	second := harnessVersionDoc(t, stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"second"}}`))
	do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(second.ID), 10)+"/activate", cookies, "")

	getRec := do(srv, "GET", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(first.ID), 10), cookies, "")
	got := harnessVersionDoc(t, getRec)
	if got.Status != "archived" {
		t.Errorf("the replaced version is %q, want archived", got.Status)
	}
}

func TestAgentHarness_DiscardThrowsADraftAway(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	doc := harnessVersionDoc(t, stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default"}}`))
	rec := do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10)+"/discard", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	getRec := do(srv, "GET", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10), cookies, "")
	got := harnessVersionDoc(t, getRec)
	if got.Status != "discarded" {
		t.Errorf("status = %q, want discarded", got.Status)
	}
}

func TestAgentHarness_DiscardRefusesALiveVersion(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	doc := harnessVersionDoc(t, stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default"}}`))
	do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10)+"/activate", cookies, "")

	rec := do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10)+"/discard", cookies, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_StagingRefusesAnInvalidSpec(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := stageSpec(t, srv, cookies, `{"spec":{"version":99,"name":""}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Error("the refusal carries no explanation")
	}
}

func TestAgentHarness_StagingRefusesAnUnknownField(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","skills":["do-thing"]}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_ImportAProposalStagesADraftWithItsEvidence(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	body := `{"spec":{"version":1,"name":"default","preamble":"You are the tuned agent."},` +
		`"evidence":{"taskSet":"ops-tasks","candidate":"candidate-optimized"}}`
	rec := do(srv, "POST", "/api/agent-harness/proposals", cookies, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	doc := harnessVersionDoc(t, rec)
	if doc.Status != "draft" {
		t.Errorf("status = %q, want draft — a proposal must not go live by itself", doc.Status)
	}
	if doc.Evidence == nil {
		t.Fatal("the evidence was dropped")
	}
	var evidence map[string]any
	if err := json.Unmarshal(doc.Evidence, &evidence); err != nil {
		t.Fatalf("the evidence is not JSON: %v", err)
	}
	if evidence["candidate"] != "candidate-optimized" {
		t.Errorf("evidence = %v", evidence)
	}

	// The proposal is also a lesson.
	lessonsRec := do(srv, "GET", "/api/agent-harness/lessons", cookies, "")
	var lessons struct {
		Lessons []AgentHarnessLesson `json:"lessons"`
	}
	if err := json.Unmarshal(lessonsRec.Body.Bytes(), &lessons); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(lessons.Lessons) != 1 {
		t.Fatalf("expected the import to record a lesson, got %d", len(lessons.Lessons))
	}
	if lessons.Lessons[0].Status != "suggested" {
		t.Errorf("lesson status = %q, want suggested", lessons.Lessons[0].Status)
	}
	if lessons.Lessons[0].SpecID != doc.ID {
		t.Errorf("lesson spec id = %d, want %d", lessons.Lessons[0].SpecID, doc.ID)
	}
}

func TestAgentHarness_ImportAProposalRefusesAnInvalidSpec(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/agent-harness/proposals", cookies, `{"spec":{"version":99,"name":""}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_LessonsCanBeRecordedAndDismissed(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/agent-harness/lessons", cookies, `{"scope":"links","note":"Answer in the operator's language."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var lesson AgentHarnessLesson
	if err := json.Unmarshal(rec.Body.Bytes(), &lesson); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if lesson.Scope != "links" || lesson.Note == "" {
		t.Errorf("lesson = %+v", lesson)
	}

	listRec := do(srv, "GET", "/api/agent-harness/lessons", cookies, "")
	var lessons struct {
		Lessons []AgentHarnessLesson `json:"lessons"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &lessons); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(lessons.Lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(lessons.Lessons))
	}

	dismissRec := do(srv, "POST", "/api/agent-harness/lessons/"+strconv.FormatUint(uint64(lesson.ID), 10)+"/dismiss", cookies, "")
	if dismissRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", dismissRec.Code, dismissRec.Body.String())
	}

	listRec = do(srv, "GET", "/api/agent-harness/lessons", cookies, "")
	if err := json.Unmarshal(listRec.Body.Bytes(), &lessons); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(lessons.Lessons) != 1 {
		t.Fatalf("the dismissed lesson disappeared: %d", len(lessons.Lessons))
	}
	if lessons.Lessons[0].Status != "dismissed" {
		t.Errorf("status = %q, want dismissed", lessons.Lessons[0].Status)
	}
}

func TestAgentHarness_LessonWithoutScopeOrNoteIsRefused(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	if rec := do(srv, "POST", "/api/agent-harness/lessons", cookies, `{"scope":"","note":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a lesson without a scope, got %d", rec.Code)
	}
	if rec := do(srv, "POST", "/api/agent-harness/lessons", cookies, `{"scope":"links","note":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a lesson without a note, got %d", rec.Code)
	}
}

func TestAgentHarness_GetOneVersion(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)
	cookies := loginCookies(t, srv)

	doc := harnessVersionDoc(t, stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"hi"}}`))
	getRec := do(srv, "GET", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10), cookies, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	got := harnessVersionDoc(t, getRec)
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

func TestAgentHarness_RequiresAuthentication(t *testing.T) {
	srv, _ := newHarnessTestHandler(t)

	rec := do(srv, "GET", "/api/agent-harness/specs", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_StagingRequiresAdmin(t *testing.T) {
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	const org = uint(7701)
	seedOrgMember(t, db, org, "owner@harness.test", "owner")
	memberUID := seedOrgMember(t, db, org, "member@harness.test", "member")
	memberSession := sessionCookies(t, memberUID, org)

	rec := stageSpec(t, srv, memberSession, `{"spec":{"version":1,"name":"default"}}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a member staged a harness: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestAgentHarness_TheActivatedVersionReachesTheChatPath(t *testing.T) {
	llm := &fakeStreamingLLM{texts: []string{"hello"}}
	srv, db, h := newAIStreamTestHandlerFull(t, llm, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	cookies := loginCookies(t, srv)
	doc := harnessVersionDoc(t, stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"You are the live assistant."}}`))
	if rec := do(srv, "POST", "/api/agent-harness/specs/"+strconv.FormatUint(uint64(doc.ID), 10)+"/activate", cookies, ""); rec.Code != http.StatusOK {
		t.Fatalf("activate: %d %s", rec.Code, rec.Body.String())
	}

	chatCookies := sessionCookies(t, 1, 1)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range chatCookies {
		req.AddCookie(c)
	}
	chatRec := httptest.NewRecorder()
	srv.ServeHTTP(chatRec, req)

	if !strings.Contains(llm.lastReq.System, "You are the live assistant.") {
		t.Errorf("the activated preamble did not reach the provider:\n%s", llm.lastReq.System)
	}
}

// A staged draft is not live: the chat path must keep using the built-in
// default until somebody activates it.
func TestAgentHarness_AStagedDraftDoesNotReachTheChatPath(t *testing.T) {
	llm := &fakeStreamingLLM{texts: []string{"hello"}}
	srv, db, h := newAIStreamTestHandlerFull(t, llm, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	cookies := loginCookies(t, srv)
	stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default","preamble":"You are the draft assistant."}}`)

	chatCookies := sessionCookies(t, 1, 1)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range chatCookies {
		req.AddCookie(c)
	}
	srv.ServeHTTP(httptest.NewRecorder(), req)

	if strings.Contains(llm.lastReq.System, "You are the draft assistant.") {
		t.Errorf("a draft reached the provider:\n%s", llm.lastReq.System)
	}
	if !strings.Contains(llm.lastReq.System, harness.DefaultPreamble) {
		t.Errorf("the built-in preamble was not used:\n%s", llm.lastReq.System)
	}
}

func TestAgentHarness_BrokenStoredSpecFailsTheChatRequest(t *testing.T) {
	llm := &fakeStreamingLLM{texts: []string{"hello"}}
	srv, db, h := newAIStreamTestHandlerFull(t, llm, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	// A live version that cannot be decoded must fail the request instead
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

func TestAgentHarness_DefaultSpecIsUsedWhenNothingIsLive(t *testing.T) {
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
	srv.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(llm.lastReq.System, harness.DefaultPreamble) {
		t.Errorf("the built-in preamble was not used:\n%s", llm.lastReq.System)
	}
}

func TestAgentHarness_TheAuthorIsNamedNotNumbered(t *testing.T) {
	srv, db, h := newAIStreamTestHandlerFull(t, nil, nil)
	store := agentstore.NewHarnessStore(db)
	h.SetHarnessStore(store)
	h.SetHarnessSpecProvider(store.Spec)

	const org = uint(7802)
	ownerUID := seedOrgMember(t, db, org, "owner@harness.test", "owner")
	cookies := sessionCookies(t, ownerUID, org)

	rec := stageSpec(t, srv, cookies, `{"spec":{"version":1,"name":"default"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	doc := harnessVersionDoc(t, rec)
	if doc.CreatedBy == "" {
		t.Error("the author is empty")
	}
	if !strings.Contains(doc.CreatedBy, "owner@harness.test") {
		t.Errorf("author = %q, want the publishing account", doc.CreatedBy)
	}
}
