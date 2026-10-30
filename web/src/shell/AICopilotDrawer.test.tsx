// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import {
  AICopilotButton,
  AICopilotDrawer,
  useAICopilotStore,
} from "./AICopilotDrawer";
import { RoleProvider } from "./role";
import type { WorkforceAgentDTO } from "../api/types";

const createMockSSEStream = (chunks: string[]) => {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
};

// workforceAgent builds one row of GET /api/workforce/agents.
const workforceAgent = (overrides: Partial<WorkforceAgentDTO> = {}): WorkforceAgentDTO => ({
  id: "agent-1",
  name: "Ops Agent",
  role: "Operator",
  department: "Operations",
  model: "test-model",
  status: "active",
  heartbeat: "1h",
  budgetUsed: 1.5,
  budgetLimit: 10,
  capabilities: [],
  directives: "",
  ...overrides,
});

// agentFetch answers the agent list, the default-agent setting, and the chat
// stream from one fetch mock.
const agentFetch = (
  agents: WorkforceAgentDTO[],
  chat: { ok: true; chunks: string[] } | { ok: false; status: number; body: Record<string, string> },
  opts: { autoHalt?: boolean; defaultAgentId?: string } = {},
) =>
  vi.fn((input: string | URL | Request, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url === "/api/workforce/default-agent") {
      const body = init?.method === "PUT" ? JSON.parse(String(init.body)) : { agentId: opts.defaultAgentId ?? "" };
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        headers: { get: () => "application/json" },
        json: async () => ({ agentId: String(body.agentId ?? "") }),
      } as unknown as Response);
    }
    if (url === "/api/workforce/agents") {
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        headers: { get: () => "application/json" },
        json: async () => ({ items: agents }),
      } as unknown as Response);
    }
    if (url === "/api/workforce/overview") {
      // Only the governance switch matters here: it decides whether a budget
      // limit blocks a turn.
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        headers: { get: () => "application/json" },
        json: async () => ({
          monthlySpend: 0,
          monthlyLimit: 0,
          autoHalt: opts.autoHalt ?? false,
          activeAgents: agents.length,
          runningTasks: 0,
          queuedTasks: 0,
          pendingApprovals: 0,
          spendByAgent: [],
          spendByProvider: [],
        }),
      } as unknown as Response);
    }
    if (!chat.ok) {
      return Promise.resolve({
        ok: false,
        status: chat.status,
        statusText: "Error",
        json: async () => chat.body,
      } as unknown as Response);
    }
    return Promise.resolve({
      ok: true,
      status: 200,
      statusText: "OK",
      body: createMockSSEStream(chat.chunks),
    } as unknown as Response);
  });

const renderAdminDrawer = () => {
  useAICopilotStore.getState().openDrawer();
  return render(
    <I18nProvider>
      <RoleProvider value={{ role: "admin", isInstanceAdmin: false }}>
        <AICopilotDrawer />
      </RoleProvider>
    </I18nProvider>,
  );
};

describe("AICopilotButton", () => {
  beforeEach(() => {
    useAICopilotStore.getState().resetDrawer();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders the trigger button with shortcut indicator and toggles drawer on click", () => {
    render(
      <I18nProvider>
        <AICopilotButton />
      </I18nProvider>,
    );

    const btn = screen.getByTestId("ai-copilot-trigger-btn");
    expect(btn).not.toBeNull();
    expect(btn.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText("⌘J")).not.toBeNull();

    // Click button to open
    fireEvent.click(btn);
    expect(useAICopilotStore.getState().isOpen).toBe(true);
    expect(btn.getAttribute("aria-pressed")).toBe("true");

    // Click button again to close
    fireEvent.click(btn);
    expect(useAICopilotStore.getState().isOpen).toBe(false);
  });
});

describe("AICopilotDrawer", () => {
  beforeEach(() => {
    useAICopilotStore.getState().resetDrawer();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("1. does not render when closed, renders accessible dialog when open", () => {
    const { rerender } = render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(screen.queryByRole("dialog")).toBeNull();

    // Open drawer
    useAICopilotStore.getState().openDrawer();
    rerender(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const dialog = screen.getByRole("dialog");
    expect(dialog).not.toBeNull();
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(screen.getAllByText("AI Copilot").length).toBeGreaterThan(0);
  });

  it("2. renders empty state with 3 quick prompt cards", () => {
    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(screen.getByTestId("copilot-empty-state")).not.toBeNull();

    // Verify presence of prompt cards
    expect(screen.getByText(/Analyze past 7 days link traffic trends/)).not.toBeNull();
    expect(screen.getByText(/Check current DNS records/)).not.toBeNull();
    expect(screen.getByText(/Summarize recent unread emails/)).not.toBeNull();
  });

  it("3. clicking a prompt card immediately sends the message and streams response", async () => {
    const streamChunks = [
      'event: text\ndata: {"delta": "DNS records are currently healthy across all edge locations."}\n\n',
      'event: done\ndata: {}\n\n',
    ];

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: createMockSSEStream(streamChunks),
    });
    global.fetch = mockFetch;

    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const dnsPill = screen.getByText(/Check current DNS records/);
    fireEvent.click(dnsPill);

    // Verify fetch request
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/ai/chat/stream",
        expect.objectContaining({
          method: "POST",
          headers: { "Content-Type": "application/json" },
        }),
      );
    });

    // Check assistant streamed content rendered
    await waitFor(() => {
      expect(
        screen.getByText("DNS records are currently healthy across all edge locations."),
      ).not.toBeNull();
    });
  });

  it("4. supports manual input, Enter to send, Shift+Enter for newline", async () => {
    const streamChunks = [
      'event: thinking\ndata: {"delta": "Analyzing query...", "tokens": 24}\n\n',
      'event: text\ndata: {"delta": "Here is the response."}\n\n',
      'event: done\ndata: {}\n\n',
    ];

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: createMockSSEStream(streamChunks),
    });
    global.fetch = mockFetch;

    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);

    // Shift+Enter should NOT send
    fireEvent.change(textarea, { target: { value: "Line 1" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });
    expect(mockFetch.mock.calls.filter(([url]) => url === "/api/ai/chat/stream")).toHaveLength(0);

    // Plain Enter sends
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalled();
    });

    // User message and assistant response
    await waitFor(() => {
      expect(screen.getByText("Line 1")).not.toBeNull();
      expect(screen.getByText("Here is the response.")).not.toBeNull();
      expect(screen.getByText("(24 tokens)")).not.toBeNull();
    });
  });

  it("5. closes on Escape key, backdrop click, and close button click", () => {
    useAICopilotStore.getState().openDrawer();

    const { rerender } = render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    // 1. Close button
    const closeBtn = screen.getByRole("button", { name: "Close" });
    fireEvent.click(closeBtn);
    expect(useAICopilotStore.getState().isOpen).toBe(false);

    // 2. Backdrop click
    useAICopilotStore.getState().openDrawer();
    rerender(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );
    const backdrop = screen.getByTestId("copilot-backdrop");
    fireEvent.click(backdrop);
    expect(useAICopilotStore.getState().isOpen).toBe(false);

    // 3. Escape key in textarea
    useAICopilotStore.getState().openDrawer();
    rerender(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );
    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(useAICopilotStore.getState().isOpen).toBe(false);
  });

  it("6. responds to octarq:open-copilot event and Mod+J / Mod+/ global shortcuts", () => {
    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(useAICopilotStore.getState().isOpen).toBe(false);

    // Custom event opens drawer
    window.dispatchEvent(new CustomEvent("octarq:open-copilot"));
    expect(useAICopilotStore.getState().isOpen).toBe(true);

    // Mod+J toggles drawer
    fireEvent.keyDown(window, { key: "j", metaKey: true });
    expect(useAICopilotStore.getState().isOpen).toBe(false);

    fireEvent.keyDown(window, { key: "j", metaKey: true });
    expect(useAICopilotStore.getState().isOpen).toBe(true);

    // Mod+/ toggles drawer
    fireEvent.keyDown(window, { key: "/", metaKey: true });
    expect(useAICopilotStore.getState().isOpen).toBe(false);
  });

  it("7. handles stream error gracefully with role='alert'", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      statusText: "Service Unavailable",
    });
    global.fetch = mockFetch;

    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.change(textarea, { target: { value: "Cause error" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      const alert = screen.getByRole("alert");
      expect(alert).not.toBeNull();
      expect(screen.getByText(/503/)).not.toBeNull();
    });
  });

  it("8. clear chat clears all history and resets to empty state", () => {
    useAICopilotStore.getState().openDrawer();
    useAICopilotStore.getState().addMessage({
      id: "m-1",
      role: "user",
      content: "Hello AI",
    });

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(screen.getByText("Hello AI")).not.toBeNull();

    const clearBtn = screen.getByRole("button", { name: "Clear chat" });
    fireEvent.click(clearBtn);

    expect(useAICopilotStore.getState().messages).toHaveLength(0);
    expect(screen.getByTestId("copilot-empty-state")).not.toBeNull();
  });

  it("9. does not offer attribution to a non-admin", () => {
    const mockFetch = agentFetch([workforceAgent()], { ok: true, chunks: [] });
    global.fetch = mockFetch as unknown as typeof fetch;

    useAICopilotStore.getState().openDrawer();
    render(
      <I18nProvider>
        <RoleProvider value={{ role: "member", isInstanceAdmin: false }}>
          <AICopilotDrawer />
        </RoleProvider>
      </I18nProvider>,
    );

    expect(screen.queryByLabelText("Agent to bill")).toBeNull();
    // A member has no attribution control. Their turns bill the workspace
    // default, which the server applies without the client asking.
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("10. saves the workspace default and leaves the turn to the server", async () => {
    const mockFetch = agentFetch([workforceAgent()], {
      ok: true,
      chunks: ['event: text\ndata: {"delta": "Billed reply"}\n\n', "event: done\ndata: {}\n\n"],
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    renderAdminDrawer();

    const select = await screen.findByLabelText("Agent to bill");
    fireEvent.change(select, { target: { value: "agent-1" } });
    expect(screen.getByText("$1.50 of $10.00")).not.toBeNull();

    // The picker writes the workspace setting, not a per-session value.
    await waitFor(() => {
      const put = mockFetch.mock.calls.find(
        ([url, init]) => url === "/api/workforce/default-agent" && init?.method === "PUT",
      );
      expect(put).toBeDefined();
      expect(JSON.parse(String(put![1]!.body))).toEqual({ agentId: "agent-1" });
    });

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.change(textarea, { target: { value: "Run the report" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      expect(screen.getByText("Billed reply")).not.toBeNull();
    });

    const chatCall = mockFetch.mock.calls.find(([url]) => url === "/api/ai/chat/stream") as unknown as
      | [string, RequestInit]
      | undefined;
    expect(chatCall).toBeDefined();
    // The server applies the default, so the turn names no agent. That keeps a
    // member's turn eligible, which an explicit agentId would refuse.
    expect(JSON.parse(String(chatCall![1].body))).toEqual({
      messages: [{ role: "user", content: "Run the report" }],
    });

    // The list is read again after the turn, so the spend shown is current.
    await waitFor(() => {
      const listCalls = mockFetch.mock.calls.filter(([url]) => url === "/api/workforce/agents");
      expect(listCalls.length).toBe(2);
    });
  });

  it("11. warns that a paused agent refuses the next turn", async () => {
    global.fetch = agentFetch([workforceAgent({ status: "paused" })], { ok: true, chunks: [] }) as unknown as typeof fetch;

    renderAdminDrawer();

    const select = await screen.findByLabelText("Agent to bill");
    fireEvent.change(select, { target: { value: "agent-1" } });

    expect(screen.getByText("This agent is paused. Resume it in AI Workforce.")).not.toBeNull();
  });

  it("12. shows the next action when the server refuses an exhausted budget", async () => {
    global.fetch = agentFetch([workforceAgent()], {
      ok: false,
      status: 409,
      body: {
        code: "workforce_budget_exceeded",
        message: "This agent reached its monthly budget limit. Raise the limit before you run a turn.",
      },
    }) as unknown as typeof fetch;

    renderAdminDrawer();

    const select = await screen.findByLabelText("Agent to bill");
    fireEvent.change(select, { target: { value: "agent-1" } });

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.change(textarea, { target: { value: "Spend too much" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      expect(screen.getByRole("alert")).not.toBeNull();
    });
    expect(
      screen.getByText("This agent reached its monthly budget limit. Raise the limit in AI Workforce."),
    ).not.toBeNull();
  });

  it("13. maps a forbidden attribution to the admin-only rule", async () => {
    global.fetch = agentFetch([workforceAgent()], {
      ok: false,
      status: 403,
      body: { code: "forbidden", message: "forbidden" },
    }) as unknown as typeof fetch;

    renderAdminDrawer();

    const select = await screen.findByLabelText("Agent to bill");
    fireEvent.change(select, { target: { value: "agent-1" } });

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.change(textarea, { target: { value: "Bill this" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      expect(screen.getByText("Only an admin can bill a turn to an agent.")).not.toBeNull();
    });
  });

  it("15. warns about an exhausted budget when auto-halt makes the limit blocking", async () => {
    global.fetch = agentFetch(
      [workforceAgent({ budgetUsed: 10, budgetLimit: 10 })],
      { ok: true, chunks: [] },
      { autoHalt: true },
    ) as unknown as typeof fetch;

    renderAdminDrawer();

    const select = await screen.findByLabelText("Agent to bill");
    fireEvent.change(select, { target: { value: "agent-1" } });

    await waitFor(() => {
      expect(
        screen.getByText("This agent reached its monthly budget limit. Raise the limit in AI Workforce."),
      ).not.toBeNull();
    });
  });

  it("16. stays quiet about an exhausted budget while auto-halt is off", async () => {
    global.fetch = agentFetch(
      [workforceAgent({ budgetUsed: 10, budgetLimit: 10 })],
      { ok: true, chunks: [] },
      { autoHalt: false },
    ) as unknown as typeof fetch;

    renderAdminDrawer();

    const select = await screen.findByLabelText("Agent to bill");
    fireEvent.change(select, { target: { value: "agent-1" } });
    // The agent list has loaded, so the absence below is the picker's choice,
    // not a pending request.
    await waitFor(() => {
      expect((select as HTMLSelectElement).value).toBe("agent-1");
    });

    expect(
      screen.queryByText("This agent reached its monthly budget limit. Raise the limit in AI Workforce."),
    ).toBeNull();
  });
});
