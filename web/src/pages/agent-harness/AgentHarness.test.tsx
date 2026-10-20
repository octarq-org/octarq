// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/react";
import AgentHarnessPage from "./index";
import { api, type HarnessSpecDetail, type HarnessSpecsListResponse } from "../../api";
import { I18nProvider } from "../../i18n";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import * as sdk from "@octarq/plugin-sdk";

vi.mock("../../api", () => ({
  api: {
    agentHarnessSpecs: vi.fn(),
    agentHarnessSpec: vi.fn(),
    publishAgentHarnessSpec: vi.fn(),
    rollbackAgentHarnessSpec: vi.fn(),
  },
}));

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
      publishedAt: "2026-10-20T01:02:03Z",
      createdBy: "admin",
    },
    versions: [
      {
        id: 12,
        name: "default",
        version: 3,
        fingerprint: "9f2c8d1e3a4b5c6d",
        publishedAt: "2026-10-20T01:02:03Z",
        createdBy: "admin",
        status: "published",
      },
      {
        id: 11,
        name: "default",
        version: 2,
        fingerprint: "41ab8c7d6e5f4a3b",
        publishedAt: "2026-10-19T20:00:00Z",
        createdBy: "admin",
        status: "archived",
      },
    ],
  };

  const mockDetail12: HarnessSpecDetail = {
    id: 12,
    name: "default",
    version: 3,
    fingerprint: "9f2c8d1e3a4b5c6d",
    status: "published",
    createdAt: "2026-10-20T01:00:00Z",
    publishedAt: "2026-10-20T01:02:03Z",
    createdBy: "admin",
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
    createdBy: "admin",
    spec: {
      version: 1,
      name: "default",
      preamble: "You are the Octarq workspace assistant v2.",
      extraRules: ["Rule A", "Rule B"],
      toolAllow: ["list_links", "list_domains"],
      maxSteps: 4,
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  // 1. Loading state
  it("renders loading skeleton while fetching specs", async () => {
    // Hang promise to keep loading state
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
  it("renders empty state with publish guide when no versions exist", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue({
      active: null,
      versions: [],
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-empty")).toBeDefined();
    });

    expect(screen.getByRole("button", { name: /publish first version|发布第一个版本/i })).toBeDefined();
    expect(screen.queryByTestId("agent-harness-ready")).toBeNull();
    expect(screen.queryByTestId("agent-harness-error")).toBeNull();
  });

  // 4. Ready state - renders versions list
  it("renders active version and version history list", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
      // Active version badge and table row
      expect(screen.getAllByText("#3").length).toBeGreaterThan(0);
      // Archived version in table
      expect(screen.getByText("#2")).toBeDefined();
      // Authors
      const admins = screen.getAllByText("admin");
      expect(admins.length).toBeGreaterThan(0);
    });
  });

  // 5. Inspect version detail
  it("opens inspector modal with full spec details including unrestricted tools", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Click inspect on active version
    const inspectBtns = screen.getAllByRole("button", { name: /inspect|查看详情/i });
    fireEvent.click(inspectBtns[0]);

    await waitFor(() => {
      // Preamble shown
      expect(screen.getByText("You are the Octarq workspace assistant v3.")).toBeDefined();
      // Rule shown
      expect(screen.getByText("Always answer in the operator's language.")).toBeDefined();
      // Unrestricted tools distinction
      expect(screen.getByText(/unrestricted|不限制/i)).toBeDefined();
    });
  });

  // 6. Diff with active version
  it("opens diff modal when clicking diff with active", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockImplementation((id: number) => {
      if (id === 12) return Promise.resolve(mockDetail12);
      if (id === 11) return Promise.resolve(mockDetail11);
      return Promise.reject(new Error("not found"));
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Click diff button on version 2 (second row)
    const diffBtns = screen.getAllByRole("button", { name: /diff with active|对比生效版本/i });
    fireEvent.click(diffBtns[1]);

    await waitFor(() => {
      // Diff shows rules added/removed or preamble comparison
      expect(screen.getByText(/Rule A/)).toBeDefined();
      expect(screen.getByText(/Rule B/)).toBeDefined();
    });
  });

  // 7. Rollback confirmation and execution
  it("prompts confirmDialog before rolling back to an older version", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.rollbackAgentHarnessSpec).mockResolvedValue({ ok: true });

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(true);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    // Click rollback button on version 2
    const rollbackBtn = screen.getByRole("button", { name: /rollback|回滚/i });
    fireEvent.click(rollbackBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.rollbackAgentHarnessSpec).toHaveBeenCalledWith(11);
      expect(api.agentHarnessSpecs).toHaveBeenCalledTimes(2); // reloads after rollback
    });
  });

  it("does not call rollback API if user cancels confirmDialog", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const confirmSpy = vi.spyOn(sdk, "confirmDialog").mockResolvedValue(false);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const rollbackBtn = screen.getByRole("button", { name: /rollback|回滚/i });
    fireEvent.click(rollbackBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledTimes(1);
      expect(api.rollbackAgentHarnessSpec).not.toHaveBeenCalled();
    });
  });

  // 8. Publish new version
  it("opens publish modal and submits new version with version 1", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);

    const newCreated: HarnessSpecDetail = {
      id: 13,
      name: "default",
      version: 4,
      fingerprint: "1122334455667788",
      status: "published",
      createdAt: "2026-10-20T09:00:00Z",
      publishedAt: "2026-10-20T09:00:00Z",
      createdBy: "admin",
      spec: {
        version: 1,
        name: "default",
        preamble: "New published agent preamble",
        extraRules: ["Rule New"],
        toolAllow: [],
        maxSteps: 8,
      },
    };
    vi.mocked(api.publishAgentHarnessSpec).mockResolvedValue(newCreated);

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const publishBtn = screen.getByRole("button", { name: /\+ (publish new version|发布新版本)/i });
    fireEvent.click(publishBtn);

    // Form inputs
    const preambleInput = screen.getByPlaceholderText(/enter instructions|请输入系统提示词/i);
    fireEvent.change(preambleInput, { target: { value: "New published agent preamble" } });

    const submitBtn = screen.getByRole("button", { name: /publish version|确认发布/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.publishAgentHarnessSpec).toHaveBeenCalledWith(
        expect.objectContaining({
          version: 1,
          name: "default",
          preamble: "New published agent preamble",
        })
      );
    });
  });

  it("displays backend error message when publish fails", async () => {
    vi.mocked(api.agentHarnessSpecs).mockResolvedValue(mockSpecsResponse);
    vi.mocked(api.agentHarnessSpec).mockResolvedValue(mockDetail12);
    vi.mocked(api.publishAgentHarnessSpec).mockRejectedValue(new Error("invalid preamble syntax"));

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId("agent-harness-ready")).toBeDefined();
    });

    const publishBtn = screen.getByRole("button", { name: /\+ (publish new version|发布新版本)/i });
    fireEvent.click(publishBtn);

    const preambleInput = screen.getByPlaceholderText(/enter instructions|请输入系统提示词/i);
    fireEvent.change(preambleInput, { target: { value: "invalid prompt" } });

    const submitBtn = screen.getByRole("button", { name: /publish version|确认发布/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByText("invalid preamble syntax")).toBeDefined();
    });
  });
});
