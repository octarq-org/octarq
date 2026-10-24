// @vitest-environment happy-dom
import React from "react";
import { render, screen, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach } from "vitest";
import { I18nProvider } from "../../i18n";
import {
  registerCard,
  resolveCard,
  registerSchema,
  schemasFor,
  renderToolCall,
  carriesApprovalID,
  type ToolCardProps,
} from "./registry";

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

function StubCard({ args, toolName }: ToolCardProps) {
  return (
    <div data-testid="stub-card" data-tool={toolName}>
      stub:{JSON.stringify(args)}
    </div>
  );
}

const noop = () => {};

describe("Card Registry", () => {
  afterEach(() => {
    cleanup();
  });

  it("hits registered tool and returns null for unregistered", () => {
    registerCard("custom_test_alpha", StubCard);
    expect(resolveCard("custom_test_alpha")).toBe(StubCard);
    expect(resolveCard("non_existent_tool")).toBeNull();
  });

  it("throws on duplicate registration (Fail-Closed)", () => {
    registerCard("custom_test_dup", StubCard);
    expect(() => registerCard("custom_test_dup", StubCard)).toThrow(/already registered/);
  });

  it("registers and retrieves schemas", () => {
    const testSchema = { type: "object", properties: { key: { type: "string" } } };
    registerSchema("schema_test_tool", testSchema);
    expect(schemasFor("schema_test_tool")).toEqual(testSchema);
    expect(schemasFor("unknown_schema_tool")).toBeNull();
  });

  it("carriesApprovalID detects approval identifiers", () => {
    expect(carriesApprovalID({ approval_id: "appr-123" })).toBe(true);
    expect(carriesApprovalID({ approvalId: "appr-456" })).toBe(true);
    expect(carriesApprovalID({ _approval_id: "appr-789" })).toBe(true);
    expect(carriesApprovalID({ other: "data" })).toBe(false);
    expect(carriesApprovalID(null)).toBe(false);
    expect(carriesApprovalID("not an object")).toBe(false);
  });
});

describe("renderToolCall", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders registered card with validated args", () => {
    registerCard("valid_tool", StubCard);
    registerSchema("valid_tool", {
      type: "object",
      required: ["message"],
      properties: { message: { type: "string" } },
    });

    renderWithI18n(<>{renderToolCall("valid_tool", "c1", { message: "hello" }, noop)}</>);
    expect(screen.getByTestId("stub-card")).toBeDefined();
    expect(screen.getByText(/hello/)).toBeDefined();
  });

  it("renders FallbackCard on unregistered tool", () => {
    renderWithI18n(<>{renderToolCall("ghost_tool", "c1", {}, noop)}</>);
    expect(screen.getByTestId("fallback-card")).toBeDefined();
    expect(screen.getByText(/ghost_tool/)).toBeDefined();
    expect(screen.getByText(/No card is registered/)).toBeDefined();
  });

  it("renders FallbackCard on malformed JSON string", () => {
    renderWithI18n(<>{renderToolCall("any_tool", "c1", "{bad json", noop)}</>);
    expect(screen.getByTestId("fallback-card")).toBeDefined();
    expect(screen.getByText(/Tool arguments are not valid JSON/)).toBeDefined();
  });

  it("renders FallbackCard on schema validation failure", () => {
    registerCard("schema_tool", StubCard);
    registerSchema("schema_tool", {
      type: "object",
      required: ["required_field"],
      properties: { required_field: { type: "string" } },
    });

    renderWithI18n(<>{renderToolCall("schema_tool", "c1", {}, noop)}</>);
    expect(screen.getByTestId("fallback-card")).toBeDefined();
    expect(screen.getByText(/Tool arguments failed validation/)).toBeDefined();
    expect(screen.getByText(/required_field is required/)).toBeDefined();
  });

  it("renders FallbackCard when tool has no schema", () => {
    registerCard("schemaless_tool", StubCard);
    renderWithI18n(<>{renderToolCall("schemaless_tool", "c1", {}, noop)}</>);
    expect(screen.getByTestId("fallback-card")).toBeDefined();
    expect(screen.getByText(/The tool has no schema to validate against/)).toBeDefined();
  });

  it("renders ApprovalCard when args carry approval ID", () => {
    renderWithI18n(
      <>{renderToolCall("any_risky_tool", "c1", { approval_id: "appr-999", summary: "Pending Action" }, noop)}</>,
    );
    expect(screen.getByText(/Approval Required/)).toBeDefined();
    expect(screen.getByText("Pending Action")).toBeDefined();
  });
});
