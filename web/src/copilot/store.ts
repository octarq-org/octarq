import { create } from "zustand";
import type { ActionDiff, ApprovalStatus, ChatMessage } from "./types";

interface CopilotUIState {
  isOpen: boolean;
  activeTab: "chat" | "approvals";
  messages: ChatMessage[];
  input: string;
  isStreaming: boolean;
  pendingApprovals: ActionDiff[];
  approvalsError: string | null;

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
  setApprovals: (approvals: ActionDiff[]) => void;
  setApprovalsError: (error: string | null) => void;
  syncApprovalStatus: (actionId: string, status: ApprovalStatus, extra?: Partial<ActionDiff>) => void;
  resetCopilot: () => void;
}

function applyApprovalStatus(
  list: ActionDiff[],
  messages: ChatMessage[],
  actionId: string,
  status: ApprovalStatus,
  extra?: Partial<ActionDiff>,
) {
  const now = new Date().toISOString();
  const patch = { status, ...extra } as Partial<ActionDiff>;
  const withTimestamp =
    status === "approved" && !patch.approvedAt ? { ...patch, approvedAt: now } : patch;
  return {
    pendingApprovals: list.map((act) => (act.id === actionId ? { ...act, ...withTimestamp } : act)),
    messages: messages.map((msg) => {
      if (!msg.actionDiffs || msg.actionDiffs.length === 0) return msg;
      return {
        ...msg,
        actionDiffs: msg.actionDiffs.map((act) =>
          act.id === actionId ? { ...act, ...withTimestamp } : act,
        ),
      };
    }),
  };
}

export const useCopilotStore = create<CopilotUIState>((set) => ({
  isOpen: false,
  activeTab: "chat",
  messages: [],
  input: "",
  isStreaming: false,
  pendingApprovals: [],
  approvalsError: null,

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
    set((state) => applyApprovalStatus(state.pendingApprovals, state.messages, actionId, "approved", { approver })),

  rejectAction: (actionId, rejectReason = "Rejected by operator") =>
    set((state) => applyApprovalStatus(state.pendingApprovals, state.messages, actionId, "rejected", { rejectReason })),

  addActionDiff: (diff) =>
    set((state) => {
      const exists = state.pendingApprovals.some((a) => a.id === diff.id);
      if (exists) return state;
      return { pendingApprovals: [diff, ...state.pendingApprovals] };
    }),

  setApprovals: (approvals) => set({ pendingApprovals: approvals, approvalsError: null }),

  setApprovalsError: (error) => set({ approvalsError: error }),

  syncApprovalStatus: (actionId, status, extra) =>
    set((state) => applyApprovalStatus(state.pendingApprovals, state.messages, actionId, status, extra)),

  resetCopilot: () =>
    set({
      activeTab: "chat",
      input: "",
      messages: [],
      pendingApprovals: [],
      approvalsError: null,
      isStreaming: false,
    }),
}));
