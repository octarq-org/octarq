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

function seedApprovals() {
  useCopilotStore.getState().addActionDiff({
    id: "act-dns-001",
    agent: "Claude Code (MCP)",
    action: "delete_dns_record",
    title: "删除 DNS 解析记录",
    target: "api.octarq.org",
    riskLevel: "destructive",
    status: "pending",
    createdAt: new Date().toISOString(),
    diff: [],
  });
  useCopilotStore.getState().addActionDiff({
    id: "act-link-002",
    agent: "Octarq Copilot",
    action: "update_redirect_target",
    title: "修改关键跳转目标",
    target: "短链: /black-friday",
    riskLevel: "write",
    status: "pending",
    createdAt: new Date().toISOString(),
    diff: [],
  });
}

describe("CopilotDrawer", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetCopilot();
    useCopilotStore.getState().openCopilot();
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("network offline");
    }));
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
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

  it("shows the unconfigured guide card instead of fake replies when LLM is missing", async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation(async (url: string) => {
      if (String(url).includes("/api/ai/assist/status")) {
        return { ok: true, json: async () => ({ configured: false, provider: "" }) };
      }
      throw new Error("network offline");
    });
    renderDrawer();

    await waitFor(() => {
      expect(screen.getByTestId("copilot-unconfigured-card")).toBeDefined();
    });
    expect(screen.getByText("LLM not configured — open Settings")).toBeDefined();
  });

  it("switches to approvals tab and renders backend-synced ActionDiff cards", () => {
    seedApprovals();
    renderDrawer();

    const approvalsTab = screen.getByText("Pending Approvals");
    fireEvent.click(approvalsTab);

    expect(useCopilotStore.getState().activeTab).toBe("approvals");
    expect(screen.getByText("删除 DNS 解析记录")).toBeDefined();
    expect(screen.getByText("修改关键跳转目标")).toBeDefined();
  });

  it("renders the empty approvals state when the queue has no cards", async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation(async (url: string) => {
      if (String(url).includes("/api/ai/assist/status")) {
        return { ok: true, json: async () => ({ configured: true, provider: "test" }) };
      }
      return {
        ok: true,
        json: async () => [],
      };
    });
    renderDrawer();

    fireEvent.click(screen.getByText("Pending Approvals"));

    await waitFor(() => {
      expect(screen.getByText("No pending approvals")).toBeDefined();
    });
  });

  it("sends user message and renders message bubble", async () => {
    renderDrawer();

    const textarea = screen.getByPlaceholderText("Ask a question or enter a command (Enter to send)...");
    fireEvent.change(textarea, { target: { value: "测试指令" } });

    const sendBtn = screen.getByTitle("Send");
    fireEvent.click(sendBtn);

    expect(screen.getByText("测试指令")).toBeDefined();
  });

  it("surfaces the backend failure verbatim instead of a simulated reply", async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation(async (url: string) => {
      if (String(url).includes("/api/ai/chat/stream")) {
        return {
          ok: false,
          status: 400,
          text: async () => JSON.stringify({ message: "AI is not configured" }),
        };
      }
      throw new Error("network offline");
    });
    renderDrawer();

    const textarea = screen.getByPlaceholderText("Ask a question or enter a command (Enter to send)...");
    fireEvent.change(textarea, { target: { value: "汇总本周点击最高的短链" } });
    fireEvent.click(screen.getByTitle("Send"));

    await waitFor(() => {
      expect(screen.getByText(/AI is not configured/)).toBeDefined();
    });
    expect(screen.queryByText(/本周热门短链排行/)).toBeNull();
  });

  it("clicking preset prompt triggers automated message submission", async () => {
    renderDrawer();

    const presetCard = screen.getByText("Summarize this week's top links");
    fireEvent.click(presetCard);

    expect(screen.getByText("Summarize this week's top links")).toBeDefined();
  });
});
