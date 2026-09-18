import { z } from "zod";

export type RiskLevel = "read" | "write" | "destructive";

export type ApprovalStatus = "pending" | "approved" | "rejected" | "expired";

export interface DiffField {
  field: string;
  label?: string;
  before?: unknown;
  after?: unknown;
}

export interface ActionDiff {
  id: string;
  agent: string;
  action: string;
  title: string;
  target: string;
  riskLevel: RiskLevel;
  status: ApprovalStatus;
  createdAt: string;
  expiresAt?: string;
  reason?: string;
  diff: DiffField[];
  approvedAt?: string;
  approver?: string;
  rejectReason?: string;
  /**
   * Backend CAS binding: the `agent_approvals` row this card represents.
   * Present only for cards materialized from `/api/ai/approvals`; decisions
   * must go through the atomic approve/reject endpoints, never local state.
   */
  approvalId?: string;
  /** Anti-replay token echoed back on approve/reject. Never logged. */
  approvalToken?: string;
}

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  thinking?: string;
  isStreaming?: boolean;
  timestamp: number;
  actionDiffs?: ActionDiff[];
  toolCall?: {
    name: string;
    args: Record<string, unknown>;
  };
}

export interface AIStatus {
  configured: boolean;
  provider: string;
}
