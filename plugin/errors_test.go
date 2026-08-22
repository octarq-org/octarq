package plugin

import (
	"fmt"
	"testing"
)

func TestAgentError_FormatAndUnwrap(t *testing.T) {
	ae := NewAgentError(
		409,
		"SLUG_ALREADY_EXISTS",
		"The slug is already in use.",
		"Please specify a different slug or omit the field.",
		false,
	)

	if ae.HTTPCode != 409 {
		t.Errorf("expected HTTPCode 409, got %d", ae.HTTPCode)
	}
	if ae.Code != "SLUG_ALREADY_EXISTS" {
		t.Errorf("expected Code SLUG_ALREADY_EXISTS, got %s", ae.Code)
	}

	errStr := ae.Error()
	if errStr != "[SLUG_ALREADY_EXISTS] The slug is already in use. (Agent Guidance: Please specify a different slug or omit the field.)" {
		t.Errorf("unexpected Error() format: %s", errStr)
	}

	wrapped := fmt.Errorf("outer wrap: %w", ae)
	unwrapped, ok := AsAgentError(wrapped)
	if !ok || unwrapped == nil {
		t.Fatalf("expected AsAgentError to succeed on wrapped error")
	}
	if unwrapped.Code != "SLUG_ALREADY_EXISTS" {
		t.Errorf("expected unwrapped Code SLUG_ALREADY_EXISTS, got %s", unwrapped.Code)
	}

	mcpMsg := FormatMCPAgentError(wrapped)
	if mcpMsg == "" {
		t.Fatalf("expected formatted MCP error message")
	}
	if !testing.Short() {
		t.Logf("Formatted MCP Error:\n%s", mcpMsg)
	}
}

func TestAgentError_NilAndPlain(t *testing.T) {
	if _, ok := AsAgentError(nil); ok {
		t.Errorf("expected AsAgentError(nil) to be false")
	}
	plain := fmt.Errorf("standard error")
	if _, ok := AsAgentError(plain); ok {
		t.Errorf("expected AsAgentError(plain) to be false")
	}
	if FormatMCPAgentError(plain) != "standard error" {
		t.Errorf("expected standard error text, got %s", FormatMCPAgentError(plain))
	}
	if FormatMCPAgentError(nil) != "" {
		t.Errorf("expected empty string for nil error")
	}
}
