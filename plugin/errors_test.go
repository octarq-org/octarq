package plugin

import (
	"encoding/json"
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
		return
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

func TestAgentError_ToProblem(t *testing.T) {
	// 1. 409 SLUG_EXISTS with non-empty instance URI
	ae409 := NewAgentError(
		409,
		"SLUG_EXISTS",
		"The slug is already in use.",
		"Please specify a different slug or omit the field.",
		false,
	)

	prob409 := ae409.ToProblem("/api/links/my-slug")
	if prob409 == nil { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
		t.Fatalf("expected non-nil Problem for 409 error")
		return
	}
	if prob409.Type != "/api/links/my-slug" { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
		t.Errorf("expected Type '/api/links/my-slug', got '%s'", prob409.Type)
	}
	if prob409.Title != "SLUG_EXISTS" {
		t.Errorf("expected Title 'SLUG_EXISTS', got '%s'", prob409.Title)
	}
	if prob409.Detail != "The slug is already in use." {
		t.Errorf("expected Detail 'The slug is already in use.', got '%s'", prob409.Detail)
	}
	if prob409.Status != 409 {
		t.Errorf("expected Status 409, got %d", prob409.Status)
	}
	if prob409.Code != "SLUG_EXISTS" {
		t.Errorf("expected Code 'SLUG_EXISTS', got '%s'", prob409.Code)
	}
	if prob409.Guidance != "Please specify a different slug or omit the field." {
		t.Errorf("expected Guidance 'Please specify a different slug or omit the field.', got '%s'", prob409.Guidance)
	}
	if prob409.Retryable {
		t.Errorf("expected Retryable false, got true")
	}

	// Verify JSON serialization
	b409, err := json.Marshal(prob409)
	if err != nil {
		t.Fatalf("failed to marshal 409 Problem: %v", err)
	}
	var unmarshaled409 Problem
	if err := json.Unmarshal(b409, &unmarshaled409); err != nil {
		t.Fatalf("failed to unmarshal 409 Problem: %v", err)
	}
	if unmarshaled409 != *prob409 {
		t.Errorf("unmarshaled problem mismatch: %+v != %+v", unmarshaled409, *prob409)
	}

	// 2. 429 RATE_LIMIT_EXCEEDED with empty instance parameter
	ae429 := NewAgentError(
		429,
		"RATE_LIMIT_EXCEEDED",
		"Too many requests received.",
		"Wait 60 seconds before retrying.",
		true,
	)

	prob429 := ae429.ToProblem("")
	if prob429 == nil { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
		t.Fatalf("expected non-nil Problem for 429 error")
		return
	}
	if prob429.Type != "" { //nolint:staticcheck // SA5011 false positive: t.Fatalf above is noreturn
		t.Errorf("expected empty Type for empty instance, got '%s'", prob429.Type)
	}
	if prob429.Title != "RATE_LIMIT_EXCEEDED" {
		t.Errorf("expected Title 'RATE_LIMIT_EXCEEDED', got '%s'", prob429.Title)
	}
	if prob429.Detail != "Too many requests received." {
		t.Errorf("expected Detail 'Too many requests received.', got '%s'", prob429.Detail)
	}
	if prob429.Status != 429 {
		t.Errorf("expected Status 429, got %d", prob429.Status)
	}
	if prob429.Code != "RATE_LIMIT_EXCEEDED" {
		t.Errorf("expected Code 'RATE_LIMIT_EXCEEDED', got '%s'", prob429.Code)
	}
	if prob429.Guidance != "Wait 60 seconds before retrying." {
		t.Errorf("expected Guidance 'Wait 60 seconds before retrying.', got '%s'", prob429.Guidance)
	}
	if !prob429.Retryable {
		t.Errorf("expected Retryable true, got false")
	}

	// Verify JSON serialization
	b429, err := json.Marshal(prob429)
	if err != nil {
		t.Fatalf("failed to marshal 429 Problem: %v", err)
	}
	var unmarshaled429 Problem
	if err := json.Unmarshal(b429, &unmarshaled429); err != nil {
		t.Fatalf("failed to unmarshal 429 Problem: %v", err)
	}
	if unmarshaled429 != *prob429 {
		t.Errorf("unmarshaled problem mismatch: %+v != %+v", unmarshaled429, *prob429)
	}

	// 3. Nil receiver safety
	var nilAE *AgentError
	if nilAE.ToProblem("instance") != nil {
		t.Errorf("expected nil Problem for nil AgentError")
	}
}

func TestAgentError_HTTPStatus(t *testing.T) {
	ae409 := NewAgentError(409, "SLUG_EXISTS", "Slug exists", "", false)
	if ae409.HTTPStatus() != 409 {
		t.Errorf("expected HTTPStatus 409, got %d", ae409.HTTPStatus())
	}

	ae429 := NewAgentError(429, "RATE_LIMIT_EXCEEDED", "Rate limited", "", true)
	if ae429.HTTPStatus() != 429 {
		t.Errorf("expected HTTPStatus 429, got %d", ae429.HTTPStatus())
	}

	var nilAE *AgentError
	if nilAE.HTTPStatus() != 0 {
		t.Errorf("expected HTTPStatus 0 for nil AgentError, got %d", nilAE.HTTPStatus())
	}
}
