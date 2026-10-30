import { create } from "zustand";
import type { ActionDiff, ApprovalStatus, ChatMessage } from "./types";

export type ArtifactType =
  | "resume_rank"
  | "invoice_audit"
  | "quotation"
  | "weekly_report"
  | "storyboard"
  | "email_draft"
  | "support_escalation"
  | "financial_deck";

export type CanvasLayout = "drawer" | "split" | "maximized";

export interface Artifact<T = unknown> {
  id: string;
  type: ArtifactType;
  title: string;
  version: number;
  status: "draft" | "reviewing" | "approved" | "rejected" | "executed";
  content: T;
  mutatingAction?: {
    toolName: string;
    approvalId?: string;
    approvalToken?: string;
    payload?: Record<string, unknown>;
    riskLevel?: "read" | "write" | "destructive";
  };
  createdAt?: string;
  updatedAt?: string;
}

export interface CopilotUIState {
  isOpen: boolean;
  activeTab: "chat" | "briefing" | "approvals";
  messages: ChatMessage[];
  input: string;
  isStreaming: boolean;
  pendingApprovals: ActionDiff[];
  approvalsError: string | null;
  threadId?: string;

  // Artifact & Canvas layout slice
  artifacts: Record<string, Artifact>;
  activeArtifactId: string | null;
  canvasLayout: CanvasLayout;

  openCopilot: () => void;
  closeCopilot: () => void;
  toggleCopilot: () => void;
  setActiveTab: (tab: "chat" | "briefing" | "approvals") => void;
  setInput: (input: string) => void;
  setThreadId: (threadId?: string) => void;
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

  openArtifact: (artifact: Artifact) => void;
  updateArtifactContent: <T>(id: string, updater: (prev: T) => T) => void;
  closeCanvas: () => void;
  setCanvasLayout: (layout: CanvasLayout) => void;
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
  threadId: undefined,

  artifacts: {},
  activeArtifactId: null,
  canvasLayout: "drawer",

  openCopilot: () => set({ isOpen: true }),
  closeCopilot: () => set({ isOpen: false }),
  toggleCopilot: () => set((state) => ({ isOpen: !state.isOpen })),
  setActiveTab: (activeTab) => set({ activeTab }),
  setInput: (input) => set({ input }),
  setThreadId: (threadId) => set({ threadId }),

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

  openArtifact: (artifact) =>
    set((state) => ({
      artifacts: { ...state.artifacts, [artifact.id]: artifact },
      activeArtifactId: artifact.id,
      canvasLayout: state.canvasLayout === "drawer" ? "split" : state.canvasLayout,
      isOpen: true,
    })),

  updateArtifactContent: (id, updater) =>
    set((state) => {
      const existing = state.artifacts[id];
      if (!existing) return state;
      const updatedContent =
        typeof updater === "function" ? updater(existing.content as never) : existing.content;
      return {
        artifacts: {
          ...state.artifacts,
          [id]: {
            ...existing,
            content: updatedContent,
            version: existing.version + 1,
            updatedAt: new Date().toISOString(),
          },
        },
      };
    }),

  closeCanvas: () => set({ activeArtifactId: null, canvasLayout: "drawer" }),

  setCanvasLayout: (canvasLayout) => set({ canvasLayout }),

  resetCopilot: () =>
    set({
      activeTab: "chat",
      input: "",
      messages: [],
      pendingApprovals: [],
      approvalsError: null,
      isStreaming: false,
      threadId: undefined,
      artifacts: {},
      activeArtifactId: null,
      canvasLayout: "drawer",
    }),
}));
