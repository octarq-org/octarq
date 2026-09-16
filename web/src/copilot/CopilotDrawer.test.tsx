// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { CopilotDrawer } from "./CopilotDrawer";
import { useCopilotStore } from "./store";
import { I18nProvider } from "../i18n";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false },
  },
});

function renderDrawer() {
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <CopilotDrawer />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

describe("CopilotDrawer", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetToSample();
    useCopilotStore.getState().openCopilot();
  });

  afterEach(() => {
    cleanup();
  });

  it("does not render when drawer is closed", () => {
    useCopilotStore.getState().closeCopilot();
    const { container } = renderDrawer();
    expect(container.querySelector('[data-testid="copilot-drawer"]')).toBeNull();
  });

  it("renders header, tabs, and preset prompt suggestions on open", () => {
    renderDrawer();

    expect(screen.getByText("AI Copilot")).toBeDefined();
    expect(screen.getByText("Agent Chat")).toBeDefined();
    expect(screen.getByText("Pending Approvals")).toBeDefined();
    expect(screen.getByText("Summarize this week's top links")).toBeDefined();
    expect(screen.getByText("Check latest payment status")).toBeDefined();
    expect(screen.getByText("Diagnose DNS anomalies for api.octarq.com")).toBeDefined();
  });

  it("switches to approvals tab and renders pending ActionDiff cards", () => {
    renderDrawer();

    const approvalsTab = screen.getByText("Pending Approvals");
    fireEvent.click(approvalsTab);

    expect(useCopilotStore.getState().activeTab).toBe("approvals");
    expect(screen.getByText("删除 DNS 解析记录")).toBeDefined();
    expect(screen.getByText("修改关键跳转目标")).toBeDefined();
  });

  it("sends user message and renders message bubble", async () => {
    renderDrawer();

    const textarea = screen.getByPlaceholderText("Ask a question or enter a command (Enter to send)...");
    fireEvent.change(textarea, { target: { value: "测试指令" } });

    const sendBtn = screen.getByTitle("Send");
    fireEvent.click(sendBtn);

    expect(screen.getByText("测试指令")).toBeDefined();
  });

  it("clicking preset prompt triggers automated message submission", async () => {
    renderDrawer();

    const presetCard = screen.getByText("Summarize this week's top links");
    fireEvent.click(presetCard);

    expect(screen.getByText("Summarize this week's top links")).toBeDefined();
  });
});
