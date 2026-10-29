// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/react";
import AgentHarnessPage from "./index";
import {
  api,
  type HarnessLesson,
  type HarnessLessonsResponse,
  type HarnessSpecDetail,
  type HarnessSpecsListResponse,
  type WorkforceAgentDTO,
  type WorkforceTaskDTO,
  type WorkforceOverviewDTO,
} from "../../api";
import { I18nProvider } from "../../i18n";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import * as sdk from "@octarq/plugin-sdk";

// The page reads the failure's status from the real ApiError class, so the
// mock keeps every other export and replaces the API client only.
vi.mock("../../api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api")>();
  return {
    ...actual,
    api: {
      agentHarnessSpecs: vi.fn(),
      agentHarnessSpec: vi.fn(),
      createAgentHarnessDraft: vi.fn(),
      importAgentHarnessProposal: vi.fn(),
      activateAgentHarnessSpec: vi.fn(),
      discardAgentHarnessSpec: vi.fn(),
      agentHarnessLessons: vi.fn(),
      createAgentHarnessLesson: vi.fn(),
      dismissAgentHarnessLesson: vi.fn(),
      workforceAgents: vi.fn(),
      workforceAgent: vi.fn(),
      createWorkforceAgent: vi.fn(),
      updateWorkforceAgent: vi.fn(),
      pauseWorkforceAgent: vi.fn(),
      resumeWorkforceAgent: vi.fn(),
      triggerWorkforceHeartbeat: vi.fn(),
      deleteWorkforceAgent: vi.fn(),
      workforceTasks: vi.fn(),
      workforceTask: vi.fn(),
      createWorkforceTask: vi.fn(),
      updateWorkforceTask: vi.fn(),
      workforceApprovals: vi.fn(),
      decideWorkforceApproval: vi.fn(),
      workforceOverview: vi.fn(),
      toggleWorkforceAutoHalt: vi.fn(),
    },
  };
});

vi.mock("../../copilot/api", () => ({
  fetchApprovals: vi.fn(),
  decideApproval: vi.fn(),
}));

import { fetchApprovals, decideApproval } from "../../copilot/api";
import { MemoryRouter } from "react-router-dom";

function renderPage(initialUrl = "/workforce?tab=specs") {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 0,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <MemoryRouter initialEntries={[initialUrl]}>
          <AgentHarnessPage />
        </MemoryRouter>
      </I18nProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("AgentHarnessPage", () => {
  const mockSpecsResponse: HarnessSpecsListResponse = {
    active: {
      id: 12,
      name: "default",
      version: 3,
      fingerprint: "9f2c8d1e3a4b5c6d",
      status: "published",
      createdAt: "2026-10-20T01:00:00Z",
      publishedAt: "2026-10-20T01:02:03Z",
      createdBy: "admin@acme.com",
      evidence: null,
    },
    versions: [
      {
        id: 13,
        name: "default",
        version: 4,
        fingerprint: "77aa88bb99cc00dd",
        status: "draft",
        createdAt: "2026-10-20T03:00:00Z",
        publishedAt: null,
        createdBy: "admin@acme.com",
        evidence: { taskSet: "ops-tasks", candidate: "candidate-optimized" },
      },
      {
        id: 12,
        name: "default",
        version: 3,
        fingerprint: "9f2c8d1e3a4b5c6d",
        status: "published",
        createdAt: "2026-10-20T01:00:00Z",
        publishedAt: "2026-10-20T01:02:03Z",
        createdBy: "admin@acme.com",
        evidence: null,
      },
      {
        id: 11,
        name: "default",
        version: 2,
        fingerprint: "41ab8c7d6e5f4a3b",
        status: "archived",
        createdAt: "2026-10-19T19:50:00Z",
        publishedAt: "2026-10-19T20:00:00Z",
        createdBy: "admin@acme.com",
        evidence: null,
      },
      {
        id: 10,
        name: "default",
        version: 1,
        fingerprint: "1234567890abcdef",
        status: "discarded",
        createdAt: "2026-10-19T18:00:00Z",
        publishedAt: null,
        createdBy: "admin@acme.com",
        evidence: null,
      },
    ],
  };

  const mockDetail13: HarnessSpecDetail = {
    id: 13,
    name: "default",
    version: 4,
    fingerprint: "77aa88bb99cc00dd",
    status: "draft",
    createdAt: "2026-10-20T03:00:00Z",
    publishedAt: null,
    createdBy: "admin@acme.com",
    evidence: { taskSet: "ops-tasks", candidate: "candidate-optimized" },
    spec: {
      version: 1,
      name: "default",
      preamble: "You are the Octarq workspace assistant v4 (staged draft).",
      extraRules: ["Rule Draft"],
      toolAllow: ["list_links"],
      maxSteps: 7,
    },
  };

  const mockDetail12: HarnessSpecDetail = {
    id: 12,
    name: "default",
    version: 3,
    fingerprint: "9f2c8d1e3a4b5c6d",
    status: "published",
    createdAt: "2026-10-20T01:00:00Z",
    publishedAt: "2026-10-20T01:02:03Z",
    createdBy: "admin@acme.com",
    evidence: null,
    spec: {
      version: 1,
      name: "default",
      preamble: "You are the Octarq workspace assistant v3.",
      extraRules: ["Always answer in the operator's language."],
      toolAllow: [], // empty = unrestricted
      maxSteps: 6,
    },
  };

  const mockDetail11: HarnessSpecDetail = {
    id: 11,
    name: "default",
    version: 2,
    fingerprint: "41ab8c7d6e5f4a3b",
    status: "archived",
    createdAt: "2026-10-19T19:50:00Z",
    publishedAt: "2026-10-19T20:00:00Z",
    createdBy: "admin@acme.com",
    evidence: null,
    spec: {
      version: 1,
      name: "default",
      preamble: "You are the Octarq workspace assistant v2.",
      extraRules: ["Rule A", "Rule B"],
      toolAllow: ["list_links", "list_domains"],
      maxSteps: 4,
    },
  };

  const mockLessonsResponse: HarnessLessonsResponse = {
    lessons: [
      {
        id: 5,
        scope: "links",
        note: "Always answer in the operator's language.",
        status: "suggested",
        evidence: { taskSet: "ops-tasks" },
        createdBy: "admin@acme.com",
        createdAt: "2026-10-20T03:05:00Z",
        specId: 13,
      },
      {
        id: 4,
        scope: "domains",
        note: "Verify DNS records before activating TLS certificates.",
        status: "adopted",
        evidence: null,
        createdBy: "admin@acme.com",
        createdAt: "2026-10-19T21:00:00Z",
        specId: 12,
      },
    ],
  };

  const mockAgentsResponse: WorkforceAgentDTO[] = [
    {
      id: "agent-01",
      name: "Atlas",
      role: "Lead Orchestrator",
      department: "Executive & Planning",
      model: "claude-3-7-sonnet",
      status: "active",
      heartbeat: "Every 5m",
      budgetUsed: 3.42,
      budgetLimit: 15.0,
      capabilities: ["task-dispatch", "fleet-monitoring", "issue-triage"],
      directives: "Decompose inbound goals into discrete tasks.",
    },
    {
      id: "agent-02",
      name: "Sentinel",
      role: "Security & Compliance Auditor",
      department: "Security & Governance",
      model: "gpt-4o",
      status: "executing",
      heartbeat: "Every 15m",
      budgetUsed: 5.18,
      budgetLimit: 20.0,
      capabilities: ["token-audit", "secret-scanning", "sandbox-enforcement"],
      directives: "Never execute destructive mutations without operator approval.",
    },
  ];

  const mockTasksResponse: WorkforceTaskDTO[] = [
    {
      id: "task-101",
      title: "Verify DNS record propagation across primary edge nodes",
      assigneeAgentId: "agent-01",
      assignee: "Atlas",
      assigneeRole: "Lead Orchestrator",
      priority: "urgent",
      status: "in_progress",
      createdAt: "2026-10-27T08:00:00Z",
      lastHeartbeat: "2m ago",
      summary: "Polling nameservers for recent TXT token verification.",
      context: "Workspace DNS edge verification.",
    },
  ];

  const mockOverviewResponse: WorkforceOverviewDTO = {
    activeAgents: 2,
    runningTasks: 1,
    queuedTasks: 0,
    pendingApprovals: 0,
    monthlySpend: 8.6,
    monthlyLimit: 50.0,
    autoHalt: true,
    spendByAgent: [
      { name: "Atlas", model: "claude-3-7-sonnet", spend: 3.42, pct: 39.8 },
      { name: "Sentinel", model: "gpt-4o", spend: 5.18, pct: 60.2 },
    ],
    spendByProvider: [
      { provider: "Anthropic", spend: 3.42, pct: 39.8 },
      { provider: "OpenAI", spend: 5.18, pct: 60.2 },
    ],
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.workforceAgents).mockResolvedValue(mockAgentsResponse);
    vi.mocked(api.workforceTasks).mockResolvedValue(mockTasksResponse);
    vi.mocked(api.workforceOverview).mockResolvedValue(mockOverviewResponse);
    vi.mocked(api.workforceAgent).mockImplementation(async (id: string) => {
      const found = mockAgentsResponse.find((a) => a.id === id);
      if (!found) throw new Error("Agent not found");
      return found;
    });
    vi.mocked(api.workforceTask).mockImplementation(async (id: string) => {
      const found = mockTasksResponse.find((t) => t.id === id);
      if (!found) throw new Error("Task not found");
      return found;
    });
    vi.mocked(api.pauseWorkforceAgent).mockImplementation(async (id: string) => {
      const found = mockAgentsResponse.find((a) => a.id === id);
      if (!found) throw new Error("Agent not found");
      return { ...found, status: "paused" };
    });
    vi.mocked(api.resumeWorkforceAgent).mockImplementation(async (id: string) => {
      const found = mockAgentsResponse.find((a) => a.id === id);
      if (!found) throw new Error("Agent not found");
      return { ...found, status: "active" };
    });
    vi.mocked(api.updateWorkforceAgent).mockImplementation(async (id: string, input) => {
      const found = mockAgentsResponse.find((a) => a.id === id);
      if (!found) throw new Error("Agent not found");
      return { ...found, ...input } as WorkforceAgentDTO;
    });
    vi.mocked(api.toggleWorkforceAutoHalt).mockImplementation(async (enabled: boolean) => ({
      autoHalt: enabled,
    }));
    vi.mocked(api.workforceApprovals).mockResolvedValue([]);
  });

  // 1. Loading state
  it("renders loading skeleton while fetching specs", async () => {
    vi.mocked(api.agentHarnessSpecs).mockReturnValue(new Promise(() => {}));

    renderPage();

    expect(screen.getByTestId("agent-harness-loading")).toBeDefined();
    expect(screen.queryByTestId("agent-harness-ready")).toBeNull();
    expect(screen.queryByTestId("agent-harness-empty")).toBeNull();
    expect(screen.queryByTestId("agent-harness-error")).toBeNull();
  });

  // 2. Error state & Retry
  it("renders error state with retry button on request failure, never an empty list", async () => {
    vi.mocked(api.agentHarnessSpecs).mockRejectedValue(new Error("Failed to connect to gateway"));

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-error")).toBeDefined();
      expect(screen.getByText("Failed to connect to gateway")).toBeDefined();
    });

    expect(screen.queryByTestId("agent-harness-empty")).toBeNull();
    expect(screen.queryByTestId("agent-harness-ready")).toBeNull();

    // Now test Retry button
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const retryBtn = screen.getByRole("button", { name: /retry|重试/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });
  });

  // 3. Empty state
  it("renders empty state with stage guide when no versions exist", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue({
      active: null,
      versions: [],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-empty")).toBeDefined();
    });

    expect(screen.getByRole("button", { name: /stage first draft|暂存首个草稿/i })).toBeDefined();
    expect(screen.queryByTestId("agent-harness-ready")).toBeNull();
    expect(screen.queryByTestId("agent-harness-error")).toBeNull();
  });

  // 4. Ready state - renders versions list with statuses
  it("renders active version and version history list with draft, published, archived, and discarded badges", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
      // Active version badge
      expect(screen.getAllByText("#3").length).toBeGreaterThan(0);
      // Versions
      expect(screen.getByText("#4")).toBeDefined();
      expect(screen.getByText("#2")).toBeDefined();
      expect(screen.getByText("#1")).toBeDefined();
      // Status badges
      expect(screen.getAllByText(/draft|草稿/i).length).toBeGreaterThan(0);
      expect(screen.getAllByText(/published|已发布/i).length).toBeGreaterThan(0);
      expect(screen.getByText(/archived|已归档/i)).toBeDefined();
      expect(screen.getByText(/discarded|已作废/i)).toBeDefined();
    });
  });

  // 5. Inspect version detail
  it("opens inspector modal with full spec details including unrestricted tools and evidence", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockImplementation((id: number) => {
      if (id === 13) return Promise.resolve(mockDetail13);
      if (id === 12) return Promise.resolve(mockDetail12);
      return Promise.reject(new Error("not found"));
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Click inspect on active version #3
    const inspectBtns = screen.getAllByRole("button", { name: /inspect|查看详情/i });
    fireEvent.click(inspectBtns[0]);

    await waitFor(() => {
      expect(screen.getByText("You are the Octarq workspace assistant v3.")).toBeDefined();
      expect(screen.getByText("Always answer in the operator's language.")).toBeDefined();
      expect(screen.getByText(/every tool the session allows|会话允许的全部工具/i)).toBeDefined();
    });
  });

  // 6. Diff with active version
  it("opens diff modal when clicking diff with active", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockImplementation((id: number) => {
      if (id === 12) return Promise.resolve(mockDetail12);
      if (id === 11) return Promise.resolve(mockDetail11);
      if (id === 13) return Promise.resolve(mockDetail13);
      return Promise.reject(new Error("not found"));
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Click diff button on version 2
    const diffBtns = screen.getAllByRole("button", { name: /diff with active|对比生效版本/i });
    fireEvent.click(diffBtns[2]);

    await waitFor(() => {
      expect(screen.getByText(/Rule A/)).toBeDefined();
      expect(screen.getByText(/Rule B/)).toBeDefined();
    });
  });

  // 7. Draft activation with confirmDialog
  it("prompts confirmDialog before activating a draft and calls activate API", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.activateAgentHarnessSpec).mockResolvedValue({ ok: true });

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(true);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Version 4 is a draft, find its Activate button
    const activateBtns = screen.getAllByRole("button", { name: /^activate$|^激活$/i });
    fireEvent.click(activateBtns[0]);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.activateAgentHarnessSpec).toHaveBeenCalledWith(13);
      expect(api.agentHarnessSpecs).toHaveBeenCalledTimes(2);
    });
  });

  it("does not call activate API if user cancels confirmDialog", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(false);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const activateBtns = screen.getAllByRole("button", { name: /^activate$|^激活$/i });
    fireEvent.click(activateBtns[0]);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.activateAgentHarnessSpec).not.toHaveBeenCalled();
    });
  });

  // 8. Draft discard with confirmDialog
  it("prompts confirmDialog before discarding a draft and calls discard API", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.discardAgentHarnessSpec).mockResolvedValue({ ok: true });

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(true);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const discardBtn = screen.getByRole("button", { name: /^discard$|^作废$/i });
    fireEvent.click(discardBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.discardAgentHarnessSpec).toHaveBeenCalledWith(13);
      expect(api.agentHarnessSpecs).toHaveBeenCalledTimes(2);
    });
  });

  it("does not call discard API if user cancels confirmDialog", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(false);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const discardBtn = screen.getByRole("button", { name: /^discard$|^作废$/i });
    fireEvent.click(discardBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.discardAgentHarnessSpec).not.toHaveBeenCalled();
    });
  });

  // 9. Discarded version does not offer Activate button
  it("does not offer an activate button for discarded versions", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // In mockSpecsResponse: #4 draft (has activate), #3 published (no activate), #2 archived (has activate), #1 discarded (NO activate)
    const activateBtns = screen.getAllByRole("button", { name: /^activate$|^激活$/i });
    expect(activateBtns.length).toBe(2); // only for draft and archived
  });

  // 10. Stage new draft
  it("opens editor modal and stages a draft via createAgentHarnessDraft", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const stagedDraft: HarnessSpecDetail = {
      id: 14,
      name: "default",
      version: 5,
      fingerprint: "feedbeef12345678",
      status: "draft",
      createdAt: "2026-10-20T10:00:00Z",
      publishedAt: null,
      createdBy: "admin@acme.com",
      evidence: null,
      spec: {
        version: 1,
        name: "default",
        preamble: "Staged new draft preamble",
        extraRules: ["Rule Staged"],
        toolAllow: [],
        maxSteps: 6,
      },
    };
    vi.mocked(api.createAgentHarnessDraft).mockResolvedValue(stagedDraft);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const stageBtn = screen.getByRole("button", { name: /stage draft|暂存草稿/i });
    fireEvent.click(stageBtn);

    const preambleInput = screen.getByPlaceholderText(/enter instructions|请输入系统提示词/i);
    fireEvent.change(preambleInput, { target: { value: "Staged new draft preamble" } });

    const submitBtn = screen.getByRole("button", { name: /stage draft|暂存草稿/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.createAgentHarnessDraft).toHaveBeenCalledWith(
        expect.objectContaining({
          version: 1,
          name: "default",
          preamble: "Staged new draft preamble",
        })
      );
    });
  });

  // 11. Import proposal - success
  it("opens proposal modal and submits proposal to create a draft", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const importedProposalDraft: HarnessSpecDetail = {
      id: 15,
      name: "default",
      version: 5,
      fingerprint: "abcd1234efgh5678",
      status: "draft",
      createdAt: "2026-10-20T11:00:00Z",
      publishedAt: null,
      createdBy: "admin@acme.com",
      evidence: { taskSet: "ops-tasks", baseline: "baseline", candidate: "candidate-optimized" },
      spec: {
        version: 1,
        name: "default",
        preamble: "Proposal imported preamble",
        extraRules: [],
        toolAllow: [],
        maxSteps: 5,
      },
    };
    vi.mocked(api.importAgentHarnessProposal).mockResolvedValue(importedProposalDraft);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const importProposalBtn = screen.getByRole("button", { name: /import proposal|导入提案/i });
    fireEvent.click(importProposalBtn);

    // Enter JSON in textarea
    const proposalJson = JSON.stringify({
      name: "default",
      spec: {
        version: 1,
        name: "default",
        preamble: "Proposal imported preamble",
        extraRules: [],
        toolAllow: [],
        maxSteps: 5,
      },
      evidence: { taskSet: "ops-tasks" },
    });

    const textarea = screen.getByPlaceholderText(/paste proposal json here|在此粘贴提案 JSON 内容/i);
    fireEvent.change(textarea, { target: { value: proposalJson } });

    const submitBtn = screen.getByRole("button", { name: /import as draft|导入为草稿/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.importAgentHarnessProposal).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "default",
          evidence: { taskSet: "ops-tasks" },
        })
      );
    });
  });

  // 12. Import proposal - failure displays backend message
  it("displays backend error message when importing proposal fails with 400 validation error", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.importAgentHarnessProposal).mockRejectedValue(
      new Error("candidate failed verification against test suite")
    );

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const importProposalBtn = screen.getByRole("button", { name: /import proposal|导入提案/i });
    fireEvent.click(importProposalBtn);

    const textarea = screen.getByPlaceholderText(/paste proposal json here|在此粘贴提案 JSON 内容/i);
    fireEvent.change(textarea, {
      target: {
        value: JSON.stringify({
          name: "default",
          spec: { version: 1, name: "default", preamble: "test", extraRules: [], toolAllow: [], maxSteps: 4 },
        }),
      },
    });

    const submitBtn = screen.getByRole("button", { name: /import as draft|导入为草稿/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByText("candidate failed verification against test suite")).toBeDefined();
    });
  });

  // 13. Lessons tab - list lessons
  it("switches to lessons ledger tab and renders lessons list with evidence", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.agentHarnessLessons).mockResolvedValue(mockLessonsResponse);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Switch to lessons tab
    const lessonsTabBtn = screen.getByRole("tab", { name: /lessons ledger|经验账本/i });
    fireEvent.click(lessonsTabBtn);

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-lessons-ready")).toBeDefined();
      expect(screen.getByText("Always answer in the operator's language.")).toBeDefined();
      expect(screen.getByText("Verify DNS records before activating TLS certificates.")).toBeDefined();
      expect(screen.getByText(/suggested|建议/i)).toBeDefined();
      expect(screen.getByText(/adopted|已采纳/i)).toBeDefined();
    });
  });

  // 14. Lessons tab - dismiss lesson
  it("prompts confirmDialog before dismissing a lesson and calls dismiss API", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.agentHarnessLessons).mockResolvedValue(mockLessonsResponse);
    vi.mocked(api.dismissAgentHarnessLesson).mockResolvedValue({ ok: true });

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(true);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const lessonsTabBtn = screen.getByRole("tab", { name: /lessons ledger|经验账本/i });
    fireEvent.click(lessonsTabBtn);

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-lessons-ready")).toBeDefined();
    });

    const dismissBtns = screen.getAllByRole("button", { name: /dismiss|忽略/i });
    fireEvent.click(dismissBtns[0]);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.dismissAgentHarnessLesson).toHaveBeenCalledWith(5);
    });
  });

  // 15. Lessons tab - add new lesson
  it("opens add lesson modal and creates a new lesson", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.agentHarnessLessons).mockResolvedValue(mockLessonsResponse);

    const createdLesson: HarnessLesson = {
      id: 6,
      scope: "general",
      note: "Always confirm before executing destructive actions.",
      status: "suggested",
      evidence: null,
      createdBy: "admin@acme.com",
      createdAt: "2026-10-20T12:00:00Z",
      specId: null,
    };
    vi.mocked(api.createAgentHarnessLesson).mockResolvedValue(createdLesson);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const lessonsTabBtn = screen.getByRole("tab", { name: /lessons ledger|经验账本/i });
    fireEvent.click(lessonsTabBtn);

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-lessons-ready")).toBeDefined();
    });

    const addLessonBtn = screen.getByRole("button", { name: /add lesson|新增经验/i });
    fireEvent.click(addLessonBtn);

    const scopeInput = screen.getByPlaceholderText(/links, domains, system/i);
    fireEvent.change(scopeInput, { target: { value: "general" } });

    const noteInput = screen.getByPlaceholderText(/describe the lesson|详细描述学到的经验/i);
    fireEvent.change(noteInput, { target: { value: "Always confirm before executing destructive actions." } });

    const submitBtn = screen.getByRole("button", { name: /save lesson|保存经验/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.createAgentHarnessLesson).toHaveBeenCalledWith(
        expect.objectContaining({
          scope: "general",
          note: "Always confirm before executing destructive actions.",
        })
      );
    });
  });

  // 16. Default activeTab is fleet when no query param is provided
  it("defaults activeTab to fleet when visiting /workforce without tab query param", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    renderPage("/workforce");

    // Fleet view is rendered by default with employee cards
    await waitFor(() => {
      expect(screen.getByText("Lead Orchestrator")).toBeDefined();
    });
    expect(screen.getByTestId("workforce-prototype-callout")).toBeDefined();
    // Specs view is not shown by default
    expect(screen.queryByTestId("agent-harness-ready")).toBeNull();
  });

  // 17. Navigates to tabs based on query parameter
  it("navigates to tabs based on ?tab= query parameter", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(fetchApprovals).mockResolvedValue([]);

    // Tasks tab
    const { unmount: unmount1 } = renderPage("/workforce?tab=tasks");
    await waitFor(() => {
      expect(screen.getByText(/All Tasks/i)).toBeDefined();
    });
    expect(screen.getByTestId("workforce-prototype-callout")).toBeDefined();
    unmount1();

    // Budgets tab
    const { unmount: unmount2 } = renderPage("/workforce?tab=budgets");
    await waitFor(() => {
      expect(screen.getByText(/Workspace Monthly Spend/i)).toBeDefined();
    });
    expect(screen.getByTestId("workforce-prototype-callout")).toBeDefined();
    unmount2();

    // Approvals tab
    const { unmount: unmount3 } = renderPage("/workforce?tab=approvals");
    await waitFor(() => {
      expect(screen.getByTestId("board-approvals-empty")).toBeDefined();
    });
    unmount3();
  });

  // 18. BoardApprovalsView renders fail-closed error state without mock fallbacks
  it("renders fail-closed error callout when fetchApprovals fails, eliminating mock fallback", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(fetchApprovals).mockRejectedValue(new Error("Approvals service connection refused"));

    renderPage("/workforce?tab=approvals");

    await waitFor(() => {
      expect(screen.getByTestId("board-approvals-error")).toBeDefined();
      expect(screen.getByText("Approvals service connection refused")).toBeDefined();
    });

    // Verify mock pending approval item (appr-01 / Revoke 2 stale API access tokens) is NOT rendered
    expect(screen.queryByText(/appr-01/i)).toBeNull();
    expect(screen.queryByText(/Revoke 2 stale API access tokens/i)).toBeNull();
  });

  // 19. BoardApprovalsView does not suppress decision error
  it("displays error toast and retains item when decideApproval fails", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(fetchApprovals).mockResolvedValue([
      {
        id: "appr-real-01",
        approvalId: "1",
        agent: "Security Auditor",
        action: "revoke_keys",
        title: "Revoke production API key",
        target: "Key #1",
        riskLevel: "destructive",
        status: "pending",
        createdAt: new Date().toISOString(),
        reason: "Security audit failed",
        diff: [],
      },
    ]);
    vi.mocked(decideApproval).mockRejectedValue(new Error("Unauthorized approval decision"));

    const toastErrorSpy = vi.spyOn(sdk.toast, "error").mockImplementation(() => "" as any);
    const toastSuccessSpy = vi.spyOn(sdk.toast, "success").mockImplementation(() => "" as any);

    renderPage("/workforce?tab=approvals");

    await waitFor(() => {
      expect(screen.getByText("Revoke production API key")).toBeDefined();
    });

    const approveBtn = screen.getByRole("button", { name: /approve|批准/i });
    fireEvent.click(approveBtn);

    await waitFor(() => {
      expect(toastErrorSpy).toHaveBeenCalledWith("Unauthorized approval decision");
      expect(toastSuccessSpy).not.toHaveBeenCalled();
      // Item must NOT be removed from UI on failure
      expect(screen.getByText("Revoke production API key")).toBeDefined();
    });
  });

  // 20. Tab switching updates the active tab view and switches between views
  it("switches view when clicking tab buttons", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    renderPage("/workforce");

    // Starts on Fleet tab
    await waitFor(() => {
      expect(screen.getByText("Lead Orchestrator")).toBeDefined();
    });

    // Click Tasks tab
    const tasksTabBtn = screen.getByRole("tab", { name: /tasks & issues|任务工单/i });
    fireEvent.click(tasksTabBtn);

    await waitFor(() => {
      expect(screen.getByText(/All Tasks/i)).toBeDefined();
    });

    // Click Budgets tab
    const budgetsTabBtn = screen.getByRole("tab", { name: /budgets & governance|预算开销/i });
    fireEvent.click(budgetsTabBtn);

    await waitFor(() => {
      expect(screen.getByText(/Workspace Monthly Spend/i)).toBeDefined();
    });
  });

  // 21. FleetView renders fail-closed error card when workforceAgents fails
  it("renders fail-closed error state in FleetView when api.workforceAgents fails and recovers on retry", async () => {
    vi.mocked(api.workforceAgents).mockRejectedValue(new Error("Database connection pool exhausted"));

    renderPage("/workforce");

    await waitFor(() => {
      expect(screen.getByTestId("fleet-view-error")).toBeDefined();
      expect(screen.getByText("Database connection pool exhausted")).toBeDefined();
    });

    // Ensure mock employee cards are NOT shown
    expect(screen.queryByText("Lead Orchestrator")).toBeNull();

    // Reset mock and click retry
    vi.mocked(api.workforceAgents).mockResolvedValue(mockAgentsResponse);
    const retryBtn = screen.getByRole("button", { name: /retry|重试/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.queryByTestId("fleet-view-error")).toBeNull();
      expect(screen.getByText("Lead Orchestrator")).toBeDefined();
    });
  });

  // 22. TasksView renders fail-closed error card when workforceTasks fails
  it("renders fail-closed error state in TasksView when api.workforceTasks fails and recovers on retry", async () => {
    vi.mocked(api.workforceTasks).mockRejectedValue(new Error("Failed to query tasks"));

    renderPage("/workforce?tab=tasks");

    await waitFor(() => {
      expect(screen.getByTestId("tasks-view-error")).toBeDefined();
      expect(screen.getByText("Failed to query tasks")).toBeDefined();
    });

    // Ensure tasks are not rendered
    expect(screen.queryByText("Verify DNS record propagation across primary edge nodes")).toBeNull();

    // Reset mock and click retry
    vi.mocked(api.workforceTasks).mockResolvedValue(mockTasksResponse);
    const retryBtn = screen.getByRole("button", { name: /retry|重试/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.queryByTestId("tasks-view-error")).toBeNull();
      expect(screen.getByText("Verify DNS record propagation across primary edge nodes")).toBeDefined();
    });
  });

  // 23. A dispatched ticket names a real agent and stores the stated objective
  it("creates a task for the selected agent and sends its real id", async () => {
    vi.mocked(api.createWorkforceTask).mockResolvedValue({
      id: "task-999",
      title: "Rotate stale DNS keys",
      assigneeAgentId: "agent-01",
      assignee: "Atlas",
      assigneeRole: "Lead Orchestrator",
      priority: "normal",
      status: "todo",
      summary: "Rotate every key older than 90 days.",
      createdAt: "2026-10-28T00:00:00Z",
      lastHeartbeat: "Just now",
    } as WorkforceTaskDTO);

    renderPage("/workforce?tab=tasks");

    await waitFor(() => {
      expect(screen.getByText("Verify DNS record propagation across primary edge nodes")).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "Assign Task" }));
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. Audit TLS certificates/i), {
      target: { value: "Rotate stale DNS keys" },
    });
    fireEvent.change(screen.getByPlaceholderText(/State success criteria/i), {
      target: { value: "Rotate every key older than 90 days." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Dispatch Task" }));

    await waitFor(() => {
      expect(api.createWorkforceTask).toHaveBeenCalledWith({
        title: "Rotate stale DNS keys",
        assigneeAgentId: "agent-01",
        priority: "normal",
        summary: "Rotate every key older than 90 days.",
      });
    });
  });

  // 24. Without a fleet there is nobody to own a ticket, so dispatch is refused
  it("disables dispatch and explains why when the workspace has no agents", async () => {
    vi.mocked(api.workforceAgents).mockResolvedValue([]);

    renderPage("/workforce?tab=tasks");

    await waitFor(() => {
      expect(screen.getByText("Verify DNS record propagation across primary edge nodes")).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "Assign Task" }));

    await waitFor(() => {
      expect(screen.getByTestId("tasks-no-agents")).toBeDefined();
    });
    const dispatch = screen.getByRole("button", { name: "Dispatch Task" }) as HTMLButtonElement;
    expect(dispatch.disabled).toBe(true);
    expect(api.createWorkforceTask).not.toHaveBeenCalled();
  });

  // 25. Optimistic pause toggle with rollback on rejection
  it("rolls back optimistic status toggle in FleetView when pause API call fails", async () => {
    vi.mocked(api.pauseWorkforceAgent).mockRejectedValue(new Error("Permission denied for agent state modification"));
    const toastErrorSpy = vi.spyOn(sdk.toast, "error").mockImplementation(() => "" as any);

    renderPage("/workforce");

    await waitFor(() => {
      expect(screen.getByText("Atlas")).toBeDefined();
    });

    // Find the pause button for Atlas (first employee, currently active)
    const pauseBtn = screen.getAllByTitle(/pause|暂停/i)[0];
    fireEvent.click(pauseBtn);

    await waitFor(() => {
      expect(api.pauseWorkforceAgent).toHaveBeenCalledWith("agent-01");
      expect(toastErrorSpy).toHaveBeenCalledWith("Permission denied for agent state modification");
    });

    // Since it rolled back, the button title should still be "Pause" (status restored to active)
    await waitFor(() => {
      expect(screen.getAllByTitle(/pause|暂停/i).length).toBeGreaterThan(0);
    });
  });

  // 24. Optimistic directives editing with rollback on rejection
  it("rolls back optimistic directives update when updateWorkforceAgent fails", async () => {
    vi.mocked(api.updateWorkforceAgent).mockRejectedValue(new Error("Failed to persist agent directives"));
    const toastErrorSpy = vi.spyOn(sdk.toast, "error").mockImplementation(() => "" as any);

    renderPage("/workforce");

    await waitFor(() => {
      expect(screen.getByText("Atlas")).toBeDefined();
    });

    const tuneBtns = screen.getAllByRole("button", { name: /^tune directives/i });
    fireEvent.click(tuneBtns[0]);

    // Modal opens
    const textarea = await screen.findByDisplayValue("Decompose inbound goals into discrete tasks.");
    fireEvent.change(textarea, { target: { value: "Malicious unapproved directive" } });

    const saveBtn = screen.getByRole("button", { name: /save directives|保存指令/i });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(api.updateWorkforceAgent).toHaveBeenCalledWith("agent-01", {
        directives: "Malicious unapproved directive",
      });
      expect(toastErrorSpy).toHaveBeenCalledWith("Failed to persist agent directives");
    });

    // Rolled back to original directives
    await waitFor(() => {
      expect(screen.getByText("Decompose inbound goals into discrete tasks.")).toBeDefined();
      expect(screen.queryByText("Malicious unapproved directive")).toBeNull();
    });
  });

  // 25. Task approvals are a separate gate from staged tool actions
  it("lists awaiting_approval tasks and resumes them through the decision endpoint", async () => {
    vi.mocked(fetchApprovals).mockResolvedValue([]);
    vi.mocked(api.workforceApprovals).mockResolvedValue([
      {
        id: "AGT-9f2c1a4b7e",
        title: "Rotate production DNS keys",
        assignee: "Sentinel",
        assigneeAgentId: "agent-7c1e9b2f4a8d",
        assigneeRole: "Security Auditor",
        priority: "urgent",
        status: "awaiting_approval",
        createdAt: "2026-10-27T08:00:00Z",
        lastHeartbeat: "1m ago",
        summary: "Prepared a key rotation batch for sign-off.",
      },
    ]);
    vi.mocked(api.decideWorkforceApproval).mockResolvedValue({
      id: "AGT-9f2c1a4b7e",
      title: "Rotate production DNS keys",
      assignee: "Sentinel",
      assigneeAgentId: "agent-7c1e9b2f4a8d",
      assigneeRole: "Security Auditor",
      priority: "urgent",
      status: "in_progress",
      createdAt: "2026-10-27T08:00:00Z",
      lastHeartbeat: "1m ago",
      summary: "Prepared a key rotation batch for sign-off.",
    });

    renderPage("/workforce?tab=approvals");

    await waitFor(() => {
      expect(screen.getByText("Rotate production DNS keys")).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: /approve/i }));

    await waitFor(() => {
      expect(api.decideWorkforceApproval).toHaveBeenCalledWith("AGT-9f2c1a4b7e", "approve");
      expect(screen.queryByText("Rotate production DNS keys")).toBeNull();
    });
  });

  // 26. Task approvals fail closed without hiding the tool approval state
  it("shows a task approval error without mocking a pending decision", async () => {
    vi.mocked(fetchApprovals).mockResolvedValue([]);
    vi.mocked(api.workforceApprovals).mockRejectedValue(new Error("Approval queue unavailable"));

    renderPage("/workforce?tab=approvals");

    await waitFor(() => {
      expect(screen.getByTestId("board-task-approvals-error")).toBeDefined();
      expect(screen.getByText("Approval queue unavailable")).toBeDefined();
    });
    expect(screen.getByTestId("board-approvals-empty")).toBeDefined();
    expect(api.decideWorkforceApproval).not.toHaveBeenCalled();
  });

  // 27. Budgets render only real spend; an empty breakdown shows the empty state
  it("renders the spend empty state instead of fabricated agent rows", async () => {
    vi.mocked(api.workforceOverview).mockResolvedValue({
      activeAgents: 0,
      runningTasks: 0,
      queuedTasks: 0,
      pendingApprovals: 0,
      monthlySpend: 0,
      monthlyLimit: 0,
      autoHalt: true,
      spendByAgent: [],
      spendByProvider: [],
    });

    renderPage("/workforce?tab=budgets");

    await waitFor(() => {
      expect(screen.getAllByText("No agent has accrued spend yet.").length).toBeGreaterThan(0);
    });
    expect(screen.queryByText("Atlas")).toBeNull();
    expect(screen.queryByText("Sentinel")).toBeNull();
  });
});
