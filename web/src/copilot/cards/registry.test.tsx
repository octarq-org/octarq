// @vitest-environment happy-dom
import React from "react";
import { render, screen, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach } from "vitest";
import {
  registerUIPlugin,
  resetRegistry,
  type ToolCardComponent,
} from "@octarq/plugin-sdk";
import { I18nProvider } from "../../i18n";
import {
  renderToolCall,
  carriesApprovalID,
  type ToolCardProps,
} from "./registry";

function StubCardBody({ args, toolName }: ToolCardProps) {
  return (
    <div data-testid="stub-card" data-tool={toolName}>
      stub:{JSON.stringify(args)}
    </div>
  );
}

const StubCard = StubCardBody as unknown as ToolCardComponent;

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

const noop = () => {};

describe("Card Registry", () => {
  afterEach(() => {
    resetRegistry();
    cleanup();
  });

  it("renders FallbackCard for an unknown tool", () => {
    renderWithI18n(
      <>{renderToolCall("octarq_network__delete_link", "c1", {}, noop)}</>,
    );
    expect(screen.getByTestId("fallback-card")).toBeDefined();
    expect(screen.getByText(/octarq_network__delete_link/)).toBeDefined();
    expect(screen.getByText(/No card is registered/)).toBeDefined();
  });

  it("renders a plugin card registered through the SDK", () => {
    registerUIPlugin({
      name: "test_network",
      routes: [],
      cards: [{ match: "octarq_network__create_link", Component: StubCard }],
    });

    renderWithI18n(
      <>{renderToolCall("octarq_network__create_link", "c1", { slug: "launch" }, noop)}</>,
    );
    expect(screen.getByTestId("stub-card")).toBeDefined();
    expect(screen.getByText(/launch/)).toBeDefined();
  });

  it("resolves a plugin wildcard card", () => {
    registerUIPlugin({
      name: "test_mail",
      routes: [],
      cards: [{ match: "octarq_mail__*", Component: StubCard }],
    });

    renderWithI18n(
      <>{renderToolCall("octarq_mail__send", "c1", { to: "a@example.com" }, noop)}</>,
    );
    expect(screen.getByTestId("stub-card")).toBeDefined();
  });

  it("does not resolve a bare tool name for a namespaced card", () => {
    registerUIPlugin({
      name: "test_network_bare",
      routes: [],
      cards: [{ match: "octarq_network__create_link", Component: StubCard }],
    });

    renderWithI18n(<>{renderToolCall("create_link", "c1", {}, noop)}</>);
    expect(screen.getByTestId("fallback-card")).toBeDefined();
  });

  it("prefers a plugin card over the generic approval card", () => {
    registerUIPlugin({
      name: "test_approval_precedence",
      routes: [],
      cards: [{ match: "octarq_network__create_link", Component: StubCard }],
    });

    renderWithI18n(
      <>{renderToolCall("octarq_network__create_link", "c1", { approval_id: "appr-1" }, noop)}</>,
    );
    expect(screen.getByTestId("stub-card")).toBeDefined();
    expect(screen.queryByText(/Approval Required/)).toBeNull();
  });

  it("renders FallbackCard on malformed JSON string", () => {
    renderWithI18n(<>{renderToolCall("any_tool", "c1", "{bad json", noop)}</>);
    expect(screen.getByTestId("fallback-card")).toBeDefined();
    expect(screen.getByText(/Tool arguments are not valid JSON/)).toBeDefined();
  });

  it("renders ApprovalCard when args carry an approval ID and no card is registered", () => {
    renderWithI18n(
      <>{renderToolCall("any_risky_tool", "c1", { approval_id: "appr-999", summary: "Pending Action" }, noop)}</>,
    );
    expect(screen.getByText(/Approval Required/)).toBeDefined();
    expect(screen.getByText("Pending Action")).toBeDefined();
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
