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
    useCopilotStore.getState().resetToSample();
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
    renderButton();

    // Default sample has 2 pending approvals
    expect(screen.getByText("2")).toBeDefined();
  });
});
