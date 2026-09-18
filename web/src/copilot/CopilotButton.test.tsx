// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { CopilotButton } from "./CopilotButton";
import { useCopilotStore } from "./store";
import { I18nProvider } from "../i18n";

function renderButton() {
  return render(
    <I18nProvider>
      <CopilotButton />
    </I18nProvider>,
  );
}

describe("CopilotButton", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetCopilot();
    useCopilotStore.getState().closeCopilot();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders button with trigger text and shortcut badge", () => {
    renderButton();

    expect(screen.getByTestId("copilot-trigger-btn")).toBeDefined();
    expect(screen.getByText("Copilot")).toBeDefined();
    expect(screen.getByTitle("AI Copilot (⌘+J)")).toBeDefined();
  });

  it("toggles copilot drawer state on click", () => {
    renderButton();

    const btn = screen.getByTestId("copilot-trigger-btn");
    expect(useCopilotStore.getState().isOpen).toBe(false);

    fireEvent.click(btn);
    expect(useCopilotStore.getState().isOpen).toBe(true);

    fireEvent.click(btn);
    expect(useCopilotStore.getState().isOpen).toBe(false);
  });

  it("displays pending count badge when approvals are pending", () => {
    useCopilotStore.getState().addActionDiff({
      id: "act-1",
      agent: "AI Agent",
      action: "a",
      title: "A",
      target: "t",
      riskLevel: "write",
      status: "pending",
      createdAt: new Date().toISOString(),
      diff: [],
    });
    useCopilotStore.getState().addActionDiff({
      id: "act-2",
      agent: "AI Agent",
      action: "b",
      title: "B",
      target: "t",
      riskLevel: "read",
      status: "pending",
      createdAt: new Date().toISOString(),
      diff: [],
    });
    renderButton();

    expect(screen.getByText("2")).toBeDefined();
  });

  it("shows no badge when the approval queue is empty", () => {
    renderButton();

    expect(screen.queryByText("2")).toBeNull();
  });
});
