package lab

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/octarq-org/octarq/server/plugin"
)

type ScriptedCompleter struct {
	mu              sync.Mutex
	responses       []string
	cursor          int
	capturedSystems []string
	callCount       int
}

func NewScriptedCompleter(responses []string) *ScriptedCompleter {
	return &ScriptedCompleter{
		responses: append([]string{}, responses...),
	}
}

func (c *ScriptedCompleter) Complete(_ context.Context, _ string, system string, _ []harness.Message) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.capturedSystems = append(c.capturedSystems, system)
	c.callCount++

	if c.cursor >= len(c.responses) {
		return "", fmt.Errorf("scripted completer: mock responses exhausted (call %d, total %d)", c.callCount, len(c.responses))
	}

	resp := c.responses[c.cursor]
	c.cursor++
	return resp, nil
}

func (c *ScriptedCompleter) CapturedSystem() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.capturedSystems) == 0 {
		return ""
	}
	return c.capturedSystems[len(c.capturedSystems)-1]
}

func (c *ScriptedCompleter) CapturedSystems() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.capturedSystems...)
}

type ExecutedToolCall struct {
	OrgID    uint   `json:"org_id"`
	Name     string `json:"name"`
	ArgsJSON string `json:"args_json"`
}

type MockToolExecutor struct {
	mu    sync.Mutex
	mocks map[string]ToolMockResponse
	calls []ExecutedToolCall
}

func NewMockToolExecutor(mocks map[string]ToolMockResponse) *MockToolExecutor {
	m := make(map[string]ToolMockResponse, len(mocks))
	for k, v := range mocks {
		m[k] = v
	}
	return &MockToolExecutor{mocks: m}
}

func (e *MockToolExecutor) Execute(_ context.Context, orgID uint, name string, argsJSON string) (string, error) {
	e.mu.Lock()
	e.calls = append(e.calls, ExecutedToolCall{OrgID: orgID, Name: name, ArgsJSON: argsJSON})
	e.mu.Unlock()

	mock, exists := e.mocks[name]
	if !exists {
		return "", fmt.Errorf("mock tool %q: no mock response defined", name)
	}

	if mock.Error != "" {
		if mock.ErrorCode != "" {
			return "", plugin.NewAgentError(500, mock.ErrorCode, mock.Error, "", false)
		}
		return "", errors.New(mock.Error)
	}

	if mock.Output != "" {
		return mock.Output, nil
	}

	return "{}", nil
}

func (e *MockToolExecutor) ExecutedCalls() []ExecutedToolCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]ExecutedToolCall{}, e.calls...)
}

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	runes := []rune(text)
	tokens := (len(runes) + 3) / 4
	if tokens == 0 && len(runes) > 0 {
		tokens = 1
	}
	return tokens
}

type TaskResult struct {
	TaskID          string               `json:"task_id"`
	TaskTitle       string               `json:"task_title,omitempty"`
	Passed          bool                 `json:"passed"`
	Status          string               `json:"status"`
	Steps           int                  `json:"steps"`
	ToolErrors      int                  `json:"tool_errors"`
	ToolCalls       []string             `json:"tool_calls"`
	InputTokens     int                  `json:"input_tokens"`
	OutputTokens    int                  `json:"output_tokens"`
	ReasoningTokens int                  `json:"reasoning_tokens"`
	TotalTokens     int                  `json:"total_tokens"`
	CapturedSystem  string               `json:"captured_system,omitempty"`
	FailureReasons  []string             `json:"failure_reasons,omitempty"`
	TraceEvents     []harness.TraceEvent `json:"trace_events,omitempty"`
}

type CandidateResult struct {
	Candidate    Candidate    `json:"candidate"`
	TaskResults  []TaskResult `json:"task_results"`
	SystemPrompt string       `json:"system_prompt"`
	CatalogCount int          `json:"catalog_count"`
}

func BuildLabCatalog(tools []harness.ToolDescriptor, filter harness.ToolFilter) harness.Catalog {
	var out []harness.ToolDescriptor
	for _, t := range tools {
		if filter == nil || filter(t) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return harness.Catalog{Tools: out}
}

func ResolveBaseCatalog(ts *TaskSet) []harness.ToolDescriptor {
	if len(ts.Catalog) > 0 {
		return ts.Descriptors()
	}
	seen := make(map[string]bool)
	var tools []harness.ToolDescriptor
	for _, t := range ts.Tasks {
		for name := range t.MockTools {
			if !seen[name] {
				seen[name] = true
				tools = append(tools, harness.ToolDescriptor{
					Name:        name,
					Summary:     fmt.Sprintf("Mock tool %s", name),
					Description: fmt.Sprintf("Auto-discovered mock tool for lab testing: %s", name),
					Method:      "POST",
					Path:        "/lab/" + name,
				})
			}
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

func RunTask(ctx context.Context, task Task, cand Candidate, baseCatalog []harness.ToolDescriptor) (TaskResult, error) {
	catalog := BuildLabCatalog(baseCatalog, cand.Spec.ToolFilter())
	maxSteps, err := cand.Spec.EffectiveMaxSteps(false)
	if err != nil {
		return TaskResult{}, fmt.Errorf("candidate %q: %w", cand.Name, err)
	}
	effectiveSpec := cand.Spec.WithMaxSteps(maxSteps)
	systemPrompt := catalog.SystemPrompt(effectiveSpec)

	completer := NewScriptedCompleter(task.MockResponses)
	executor := NewMockToolExecutor(task.MockTools)
	tracer := harness.NewRecordingTracer()
	guard := harness.NewSpecGuard(cand.Spec)

	runner, err := harness.NewRunner(
		completer,
		executor,
		harness.WithGuard(guard),
		harness.WithTracer(tracer),
		harness.WithProfile(cand.Spec.Profile),
		harness.WithSystem(systemPrompt),
		harness.WithMaxSteps(maxSteps),
	)
	if err != nil {
		return TaskResult{}, fmt.Errorf("candidate %q: %w", cand.Name, err)
	}

	session := &harness.Session{
		ID:    fmt.Sprintf("lab-%s-%s", cand.Name, task.ID),
		OrgID: 1,
	}
	turn := &harness.Turn{
		Input: task.Input,
	}

	_ = runner.Run(ctx, session, turn)

	toolCalls := make([]string, 0, len(turn.Steps))
	toolErrors := 0
	for _, st := range turn.Steps {
		toolCalls = append(toolCalls, st.ToolName)
		if st.Err != nil {
			toolErrors++
		}
	}

	inputTokens := EstimateTokens(systemPrompt)
	outputTokens := 0
	for _, m := range session.History {
		tokens := EstimateTokens(m.Content)
		if m.Role == "assistant" {
			outputTokens += tokens
		} else {
			inputTokens += tokens
		}
	}

	judgement := task.Criteria.Judge(turn, session.History)

	traceEvents := tracer.Events()
	for i := range traceEvents {
		traceEvents[i].DurationMS = 0
	}

	return TaskResult{
		TaskID:         task.ID,
		TaskTitle:      task.Title,
		Passed:         judgement.Passed,
		Status:         turn.Status.String(),
		Steps:          len(turn.Steps),
		ToolErrors:     toolErrors,
		ToolCalls:      toolCalls,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		TotalTokens:    inputTokens + outputTokens,
		CapturedSystem: completer.CapturedSystem(),
		FailureReasons: judgement.Reasons,
		TraceEvents:    traceEvents,
	}, nil
}

func RunCandidate(ctx context.Context, cand Candidate, ts *TaskSet) (CandidateResult, error) {
	baseCatalog := ResolveBaseCatalog(ts)
	catalog := BuildLabCatalog(baseCatalog, cand.Spec.ToolFilter())
	maxSteps, err := cand.Spec.EffectiveMaxSteps(false)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("candidate %q: %w", cand.Name, err)
	}
	effectiveSpec := cand.Spec.WithMaxSteps(maxSteps)
	systemPrompt := catalog.SystemPrompt(effectiveSpec)

	results := make([]TaskResult, 0, len(ts.Tasks))
	for _, t := range ts.Tasks {
		res, err := RunTask(ctx, t, cand, baseCatalog)
		if err != nil {
			return CandidateResult{}, fmt.Errorf("task %q failed replay: %w", t.ID, err)
		}
		results = append(results, res)
	}

	return CandidateResult{
		Candidate:    cand,
		TaskResults:  results,
		SystemPrompt: systemPrompt,
		CatalogCount: len(catalog.Tools),
	}, nil
}
