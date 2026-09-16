import { create } from "zustand";
import type { ActionDiff, ChatMessage } from "./types";

export const SAMPLE_APPROVALS: ActionDiff[] = [
  {
    id: "act-dns-001",
    agent: "Claude Code (MCP)",
    action: "delete_dns_record",
    title: "删除 DNS 解析记录",
    target: "api.octarq.org (A 记录 -> 1.2.3.4)",
    riskLevel: "destructive",
    status: "pending",
    createdAt: new Date(Date.now() - 1000 * 60 * 12).toISOString(),
    expiresAt: new Date(Date.now() + 1000 * 60 * 60 * 2).toISOString(),
    reason: "智能体检测到主 API 节点迁移完成，提议下线旧版直连 IP 解析以避免分流风险。",
    diff: [
      { field: "domain", label: "目标域名", before: "api.octarq.org", after: "api.octarq.org" },
      { field: "type", label: "记录类型", before: "A", after: "(已删除)" },
      { field: "value", label: "解析值", before: "1.2.3.4", after: "(已删除)" },
      { field: "ttl", label: "TTL", before: "300s", after: "(已删除)" },
    ],
  },
  {
    id: "act-link-002",
    agent: "Octarq Copilot",
    action: "update_redirect_target",
    title: "修改关键跳转目标",
    target: "短链: /black-friday",
    riskLevel: "write",
    status: "pending",
    createdAt: new Date(Date.now() - 1000 * 60 * 35).toISOString(),
    reason: "黑五预热活动结束，根据运营策略重定向至正式大促主会场页面。",
    diff: [
      {
        field: "target",
        label: "跳转目标",
        before: "https://octarq.org/campaign/warmup",
        after: "https://octarq.org/campaign/main-sale",
      },
      { field: "utm_campaign", label: "UTM 参数", before: "warmup_2026", after: "main_sale_2026" },
    ],
  },
];

interface CopilotUIState {
  isOpen: boolean;
  activeTab: "chat" | "approvals";
  messages: ChatMessage[];
  input: string;
  isStreaming: boolean;
  pendingApprovals: ActionDiff[];

  openCopilot: () => void;
  closeCopilot: () => void;
  toggleCopilot: () => void;
  setActiveTab: (tab: "chat" | "approvals") => void;
  setInput: (input: string) => void;
  addMessage: (message: ChatMessage) => void;
  updateLastMessage: (updater: Partial<ChatMessage> | ((prev: ChatMessage) => ChatMessage)) => void;
  clearMessages: () => void;
  setIsStreaming: (isStreaming: boolean) => void;
  approveAction: (actionId: string, approver?: string) => void;
  rejectAction: (actionId: string, reason?: string) => void;
  addActionDiff: (diff: ActionDiff) => void;
  resetToSample: () => void;
}

export const useCopilotStore = create<CopilotUIState>((set) => ({
  isOpen: false,
  activeTab: "chat",
  messages: [],
  input: "",
  isStreaming: false,
  pendingApprovals: SAMPLE_APPROVALS,

  openCopilot: () => set({ isOpen: true }),
  closeCopilot: () => set({ isOpen: false }),
  toggleCopilot: () => set((state) => ({ isOpen: !state.isOpen })),
  setActiveTab: (activeTab) => set({ activeTab }),
  setInput: (input) => set({ input }),

  addMessage: (message) =>
    set((state) => ({
      messages: [...state.messages, message],
    })),

  updateLastMessage: (updater) =>
    set((state) => {
      if (state.messages.length === 0) return state;
      const lastIndex = state.messages.length - 1;
      const lastMsg = state.messages[lastIndex];
      const updated =
        typeof updater === "function" ? updater(lastMsg) : { ...lastMsg, ...updater };
      const nextMessages = [...state.messages];
      nextMessages[lastIndex] = updated;
      return { messages: nextMessages };
    }),

  clearMessages: () => set({ messages: [], input: "" }),
  setIsStreaming: (isStreaming) => set({ isStreaming }),

  approveAction: (actionId, approver = "Operator") =>
    set((state) => {
      const now = new Date().toISOString();
      const nextApprovals = state.pendingApprovals.map((act) =>
        act.id === actionId
          ? { ...act, status: "approved" as const, approvedAt: now, approver }
          : act,
      );

      // Also update any inline actionDiff in chat messages
      const nextMessages = state.messages.map((msg) => {
        if (!msg.actionDiffs || msg.actionDiffs.length === 0) return msg;
        return {
          ...msg,
          actionDiffs: msg.actionDiffs.map((act) =>
            act.id === actionId
              ? { ...act, status: "approved" as const, approvedAt: now, approver }
              : act,
          ),
        };
      });

      return {
        pendingApprovals: nextApprovals,
        messages: nextMessages,
      };
    }),

  rejectAction: (actionId, rejectReason = "Rejected by operator") =>
    set((state) => {
      const nextApprovals = state.pendingApprovals.map((act) =>
        act.id === actionId
          ? { ...act, status: "rejected" as const, rejectReason }
          : act,
      );

      const nextMessages = state.messages.map((msg) => {
        if (!msg.actionDiffs || msg.actionDiffs.length === 0) return msg;
        return {
          ...msg,
          actionDiffs: msg.actionDiffs.map((act) =>
            act.id === actionId
              ? { ...act, status: "rejected" as const, rejectReason }
              : act,
          ),
        };
      });

      return {
        pendingApprovals: nextApprovals,
        messages: nextMessages,
      };
    }),

  addActionDiff: (diff) =>
    set((state) => {
      const exists = state.pendingApprovals.some((a) => a.id === diff.id);
      if (exists) return state;
      return { pendingApprovals: [diff, ...state.pendingApprovals] };
    }),

  resetToSample: () =>
    set({
      activeTab: "chat",
      input: "",
      messages: [],
      pendingApprovals: SAMPLE_APPROVALS,
      isStreaming: false,
    }),
}));
