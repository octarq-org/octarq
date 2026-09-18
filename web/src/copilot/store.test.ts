import { describe, it, expect, beforeEach } from "vitest";
import { useCopilotStore } from "./store";
import type { ActionDiff } from "./types";

describe("useCopilotStore", () => {
  beforeEach(() => {
    const s = useCopilotStore.getState();
    s.resetCopilot();
    s.closeCopilot();
    s.addActionDiff({
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
    s.addActionDiff({
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

  it("starts with an empty approval queue (no seeded mock data)", () => {
    useCopilotStore.getState().resetCopilot();
    expect(useCopilotStore.getState().pendingApprovals).toHaveLength(0);
  });

  it("replaces the queue when backend approvals arrive", () => {
    useCopilotStore.getState().setApprovals([
      {
        id: "approval-7",
        agent: "AI Agent",
        action: "delete_dns_record",
        title: "删除 DNS 解析记录",
        target: "api.octarq.org",
        riskLevel: "destructive",
        status: "pending",
        createdAt: new Date().toISOString(),
        diff: [],
      },
    ]);
    const approvals = useCopilotStore.getState().pendingApprovals;
    expect(approvals).toHaveLength(1);
    expect(approvals[0].id).toBe("approval-7");
    expect(useCopilotStore.getState().approvalsError).toBeNull();
  });

  it("syncs a conflicted approval to expired without optimistic approve", () => {
    useCopilotStore.getState().syncApprovalStatus("act-dns-001", "expired", {
      rejectReason: "已被他人处理，本地状态已同步",
    });
    const target = useCopilotStore.getState().pendingApprovals.find((a) => a.id === "act-dns-001");
    expect(target?.status).toBe("expired");
    expect(target?.rejectReason).toBe("已被他人处理，本地状态已同步");
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
