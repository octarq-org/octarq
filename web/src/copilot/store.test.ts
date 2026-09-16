import { describe, it, expect, beforeEach } from "vitest";
import { useCopilotStore } from "./store";
import type { ActionDiff } from "./types";

describe("useCopilotStore", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetToSample();
    useCopilotStore.getState().closeCopilot();
  });

  it("handles drawer open, close, and toggle state", () => {
    expect(useCopilotStore.getState().isOpen).toBe(false);

    useCopilotStore.getState().openCopilot();
    expect(useCopilotStore.getState().isOpen).toBe(true);

    useCopilotStore.getState().closeCopilot();
    expect(useCopilotStore.getState().isOpen).toBe(false);

    useCopilotStore.getState().toggleCopilot();
    expect(useCopilotStore.getState().isOpen).toBe(true);
  });

  it("switches active tab between chat and approvals", () => {
    expect(useCopilotStore.getState().activeTab).toBe("chat");

    useCopilotStore.getState().setActiveTab("approvals");
    expect(useCopilotStore.getState().activeTab).toBe("approvals");

    useCopilotStore.getState().setActiveTab("chat");
    expect(useCopilotStore.getState().activeTab).toBe("chat");
  });

  it("manages chat message lifecycle and streaming state", () => {
    const msg = {
      id: "msg-1",
      role: "user" as const,
      content: "汇总短链",
      timestamp: Date.now(),
    };

    useCopilotStore.getState().addMessage(msg);
    expect(useCopilotStore.getState().messages).toHaveLength(1);
    expect(useCopilotStore.getState().messages[0].content).toBe("汇总短链");

    useCopilotStore.getState().updateLastMessage({ content: "更新内容" });
    expect(useCopilotStore.getState().messages[0].content).toBe("更新内容");

    useCopilotStore.getState().setIsStreaming(true);
    expect(useCopilotStore.getState().isStreaming).toBe(true);

    useCopilotStore.getState().clearMessages();
    expect(useCopilotStore.getState().messages).toHaveLength(0);
  });

  it("approves an action diff and updates approval status with approver", () => {
    const sampleId = "act-dns-001";
    useCopilotStore.getState().approveAction(sampleId, "AdminUser");

    const approvals = useCopilotStore.getState().pendingApprovals;
    const target = approvals.find((a) => a.id === sampleId);
    expect(target).toBeDefined();
    expect(target?.status).toBe("approved");
    expect(target?.approver).toBe("AdminUser");
    expect(target?.approvedAt).toBeDefined();
  });

  it("rejects an action diff and records rejection reason", () => {
    const sampleId = "act-link-002";
    useCopilotStore.getState().rejectAction(sampleId, "Risk too high");

    const approvals = useCopilotStore.getState().pendingApprovals;
    const target = approvals.find((a) => a.id === sampleId);
    expect(target).toBeDefined();
    expect(target?.status).toBe("rejected");
    expect(target?.rejectReason).toBe("Risk too high");
  });

  it("adds new action diff without duplicates", () => {
    const newDiff: ActionDiff = {
      id: "act-new-999",
      agent: "Autonomous Solopreneur",
      action: "purge_cache",
      title: "刷新边缘缓存",
      target: "zone: example.com",
      riskLevel: "write",
      status: "pending",
      createdAt: new Date().toISOString(),
      diff: [],
    };

    useCopilotStore.getState().addActionDiff(newDiff);
    expect(useCopilotStore.getState().pendingApprovals.some((a) => a.id === "act-new-999")).toBe(true);

    // Duplicate add should be ignored
    useCopilotStore.getState().addActionDiff(newDiff);
    const matches = useCopilotStore.getState().pendingApprovals.filter((a) => a.id === "act-new-999");
    expect(matches).toHaveLength(1);
  });
});
