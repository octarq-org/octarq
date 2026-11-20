package plugin

import (
	"errors"
	"fmt"
)

type AgentError struct {
	HTTPCode      int    `json:"http_code"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	AgentGuidance string `json:"agent_guidance"`
	Retryable     bool   `json:"retryable"`
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

func (e *AgentError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.HTTPCode
}

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Guidance  string `json:"agent_guidance"`
	Retryable bool   `json:"retryable"`
}

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

func NewAgentError(httpCode int, code, message, guidance string, retryable bool) *AgentError {
	return &AgentError{
		HTTPCode:      httpCode,
		Code:          code,
		Message:       message,
		AgentGuidance: guidance,
		Retryable:     retryable,
	}
}

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
