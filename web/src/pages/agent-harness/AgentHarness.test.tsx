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
    },
  };
});

function renderPage() {
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
        <AgentHarnessPage />
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

  beforeEach(() => {
    vi.clearAllMocks();
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
    const lessonsTabBtn = screen.getByRole("button", { name: /lessons ledger|经验账本/i });
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

    const lessonsTabBtn = screen.getByRole("button", { name: /lessons ledger|经验账本/i });
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

    const lessonsTabBtn = screen.getByRole("button", { name: /lessons ledger|经验账本/i });
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
});
