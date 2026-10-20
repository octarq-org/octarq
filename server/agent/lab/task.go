package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/octarq-org/octarq/server/agent/harness"
)

// TaskSetVersion is the current schema version for task set files.
const TaskSetVersion = 1

// ToolMockResponse defines the predetermined output or error for a tool call.
type ToolMockResponse struct {
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// UnmarshalJSON permits either a string (shorthand for output) or an object.
func (r *ToolMockResponse) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		r.Output = str
		return nil
	}
	type rawToolMock ToolMockResponse
	var raw rawToolMock
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = ToolMockResponse(raw)
	return nil
}

// TaskCriteria defines deterministic verification rules for a task run.
type TaskCriteria struct {
	ExpectedStatus string   `json:"expected_status,omitempty"`
	ExpectedTools  []string `json:"expected_tools,omitempty"`
	ForbiddenTools []string `json:"forbidden_tools,omitempty"`
	FinalContains  string   `json:"final_contains,omitempty"`
	MaxSteps       int      `json:"max_steps,omitempty"`
	MaxToolErrors  *int     `json:"max_tool_errors,omitempty"`
}

// Judgement is the result of applying TaskCriteria to an executed turn.
type Judgement struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons,omitempty"`
}

// Judge evaluates the turn and conversation history against the criteria.
func (c TaskCriteria) Judge(turn *harness.Turn, history []harness.Message) Judgement {
	var reasons []string

	if c.ExpectedStatus != "" && turn.Status.String() != c.ExpectedStatus {
		reasons = append(reasons, fmt.Sprintf("status mismatch: expected %q, got %q", c.ExpectedStatus, turn.Status.String()))
	}

	toolCallsMap := make(map[string]int)
	toolErrorCount := 0
	for _, st := range turn.Steps {
		toolCallsMap[st.ToolName]++
		if st.Err != nil {
			toolErrorCount++
		}
	}

	for _, expected := range c.ExpectedTools {
		if toolCallsMap[expected] == 0 {
			reasons = append(reasons, fmt.Sprintf("missing expected tool call: %q", expected))
		}
	}

	for _, forbidden := range c.ForbiddenTools {
		if count := toolCallsMap[forbidden]; count > 0 {
			reasons = append(reasons, fmt.Sprintf("called forbidden tool: %q (%d time(s))", forbidden, count))
		}
	}

	if c.MaxSteps > 0 && len(turn.Steps) > c.MaxSteps {
		reasons = append(reasons, fmt.Sprintf("step count exceeded: max %d, took %d", c.MaxSteps, len(turn.Steps)))
	}

	if c.MaxToolErrors != nil && toolErrorCount > *c.MaxToolErrors {
		reasons = append(reasons, fmt.Sprintf("tool error count exceeded: max %d, encountered %d", *c.MaxToolErrors, toolErrorCount))
	}

	if c.FinalContains != "" {
		lastAssistant := ""
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == "assistant" {
				lastAssistant = history[i].Content
				break
			}
		}
		if !strings.Contains(lastAssistant, c.FinalContains) {
			reasons = append(reasons, fmt.Sprintf("final assistant response missing expected substring: %q", c.FinalContains))
		}
	}

	return Judgement{
		Passed:  len(reasons) == 0,
		Reasons: reasons,
	}
}

// ParameterDef is the declarative parameter shape in task set JSON.
type ParameterDef struct {
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ToDescriptor converts ParameterDef to harness.ParameterDescriptor.
func (p ParameterDef) ToDescriptor() harness.ParameterDescriptor {
	return harness.ParameterDescriptor{
		Name:        p.Name,
		Type:        p.Type,
		Required:    p.Required,
		Description: p.Description,
		Enum:        append([]string{}, p.Enum...),
	}
}

// ToolDef is the declarative tool shape in task set JSON.
type ToolDef struct {
	Name             string         `json:"name"`
	Summary          string         `json:"summary,omitempty"`
	Description      string         `json:"description,omitempty"`
	Method           string         `json:"method,omitempty"`
	Path             string         `json:"path,omitempty"`
	Risk             string         `json:"risk,omitempty"`
	RequiresApproval bool           `json:"requires_approval,omitempty"`
	Roles            []string       `json:"roles,omitempty"`
	Parameters       []ParameterDef `json:"parameters,omitempty"`
}

// ToDescriptor converts ToolDef to harness.ToolDescriptor.
func (t ToolDef) ToDescriptor() harness.ToolDescriptor {
	var params []harness.ParameterDescriptor
	for _, p := range t.Parameters {
		params = append(params, p.ToDescriptor())
	}
	return harness.ToolDescriptor{
		Name:             t.Name,
		Summary:          t.Summary,
		Description:      t.Description,
		Method:           t.Method,
		Path:             t.Path,
		Risk:             t.Risk,
		RequiresApproval: t.RequiresApproval,
		Roles:            append([]string{}, t.Roles...),
		Parameters:       params,
	}
}

// Task is a single reproducible evaluation case.
type Task struct {
	ID            string                      `json:"id"`
	Title         string                      `json:"title,omitempty"`
	Input         string                      `json:"input"`
	MockResponses []string                    `json:"mock_responses"`
	MockTools     map[string]ToolMockResponse `json:"mock_tools,omitempty"`
	Criteria      TaskCriteria                `json:"criteria"`
}

// TaskSet is a collection of tasks and shared tool descriptors.
type TaskSet struct {
	Version     int       `json:"version"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Catalog     []ToolDef `json:"catalog,omitempty"`
	Tasks       []Task    `json:"tasks"`
}

// Descriptors converts Catalog ToolDefs to harness ToolDescriptors.
func (ts *TaskSet) Descriptors() []harness.ToolDescriptor {
	if ts == nil {
		return nil
	}
	var out []harness.ToolDescriptor
	for _, td := range ts.Catalog {
		out = append(out, td.ToDescriptor())
	}
	return out
}

// LoadTaskSet decodes and validates a task set file.
func LoadTaskSet(path string) (*TaskSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task set file %q: %w", path, err)
	}

	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()

	var ts TaskSet
	if err := dec.Decode(&ts); err != nil {
		return nil, fmt.Errorf("decode task set %q: %w", path, err)
	}

	if err := ValidateTaskSet(&ts); err != nil {
		return nil, fmt.Errorf("invalid task set %q: %w", path, err)
	}

	return &ts, nil
}

// ValidateTaskSet enforces integrity constraints on the task set.
func ValidateTaskSet(ts *TaskSet) error {
	if ts == nil {
		return errors.New("task set is nil")
	}
	if ts.Version != TaskSetVersion {
		return fmt.Errorf("task set version must be %d, got %d", TaskSetVersion, ts.Version)
	}
	if strings.TrimSpace(ts.Name) == "" {
		return errors.New("task set name must not be empty")
	}
	if len(ts.Tasks) == 0 {
		return errors.New("task set must contain at least one task")
	}

	seenIDs := make(map[string]bool, len(ts.Tasks))
	for i, t := range ts.Tasks {
		id := strings.TrimSpace(t.ID)
		if id == "" {
			return fmt.Errorf("tasks[%d]: id must not be empty", i)
		}
		if seenIDs[id] {
			return fmt.Errorf("tasks[%d]: duplicate task id %q", i, id)
		}
		seenIDs[id] = true

		if strings.TrimSpace(t.Input) == "" {
			return fmt.Errorf("task %q: input must not be empty", id)
		}
		if len(t.MockResponses) == 0 {
			return fmt.Errorf("task %q: mock_responses must contain at least one reply", id)
		}
	}

	return nil
}
