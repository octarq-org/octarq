package plugin

import (
	"errors"
	"fmt"
)

// AgentError is a structured error designed for Agent-Native and API scenarios.
// It carries HTTP status code mapping, machine-readable business error codes,
// human-friendly error messages, actionable guidance for AI agents (LLMs),
// and explicit retryability indications.
type AgentError struct {
	HTTPCode      int    `json:"http_code"`      // Target HTTP status code (e.g. 400, 401, 402, 403, 404, 409, 422, 429)
	Code          string `json:"code"`           // Stable business error code (e.g. "SLUG_ALREADY_EXISTS", "DOMAIN_NOT_VERIFIED")
	Message       string `json:"message"`        // Human-readable error message
	AgentGuidance string `json:"agent_guidance"` // Actionable guidance for LLM/Agent to fix parameters or take remedial actions
	Retryable     bool   `json:"retryable"`      // True if the operation may succeed on immediate retry without input modifications
}

func (e *AgentError) Error() string {
	if e == nil {
		return ""
	}
	if e.AgentGuidance != "" {
		return fmt.Sprintf("[%s] %s (Agent Guidance: %s)", e.Code, e.Message, e.AgentGuidance)
	}
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return e.Message
}

// HTTPStatus returns the target HTTP status code for the AgentError.
func (e *AgentError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.HTTPCode
}

// Problem represents an RFC 7807 Problem Details object with Agent-Native extensions.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Guidance  string `json:"agent_guidance"`
	Retryable bool   `json:"retryable"`
}

// ToProblem maps the AgentError to an RFC 7807 Problem Details object.
func (e *AgentError) ToProblem(instance string) *Problem {
	if e == nil {
		return nil
	}
	return &Problem{
		Type:      instance,
		Title:     e.Code,
		Detail:    e.Message,
		Status:    e.HTTPCode,
		Code:      e.Code,
		Guidance:  e.AgentGuidance,
		Retryable: e.Retryable,
	}
}

// NewAgentError constructs a new AgentError.
func NewAgentError(httpCode int, code, message, guidance string, retryable bool) *AgentError {
	return &AgentError{
		HTTPCode:      httpCode,
		Code:          code,
		Message:       message,
		AgentGuidance: guidance,
		Retryable:     retryable,
	}
}

// AsAgentError checks if err is or wraps an *AgentError.
func AsAgentError(err error) (*AgentError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *AgentError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// FormatMCPAgentError formats an error (including AgentError) into a descriptive string suitable for MCP tool responses.
func FormatMCPAgentError(err error) string {
	if err == nil {
		return ""
	}
	ae, ok := AsAgentError(err)
	if !ok {
		return err.Error()
	}
	res := fmt.Sprintf("Error [%s]: %s", ae.Code, ae.Message)
	if ae.AgentGuidance != "" {
		res += fmt.Sprintf("\n\n[Agent Action Guidance]: %s", ae.AgentGuidance)
	}
	if ae.Retryable {
		res += "\n[Retryable]: true (You may retry this operation after addressing any transient issues)"
	} else {
		res += "\n[Retryable]: false (Do not retry without modifying parameters or resolving the prerequisite)"
	}
	return res
}
