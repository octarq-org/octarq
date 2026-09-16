import { z } from "zod";
import { parseWithFallback } from "../lib/parseWithFallback";
import type { ActionDiff, AIStatus, DiffField, RiskLevel, ApprovalStatus } from "./types";

export const riskLevelSchema = z.enum(["read", "write", "destructive"]);

export const approvalStatusSchema = z.enum(["pending", "approved", "rejected", "expired"]);

export const diffFieldSchema = z.object({
  field: z.string(),
  label: z.string().optional(),
  before: z.unknown().optional(),
  after: z.unknown().optional(),
});

export const actionDiffSchema = z.object({
  id: z.string(),
  agent: z.string(),
  action: z.string(),
  title: z.string(),
  target: z.string(),
  riskLevel: riskLevelSchema,
  status: approvalStatusSchema,
  createdAt: z.string(),
  expiresAt: z.string().optional(),
  reason: z.string().optional(),
  diff: z.array(diffFieldSchema),
  approvedAt: z.string().optional(),
  approver: z.string().optional(),
  rejectReason: z.string().optional(),
});

export const actionDiffListSchema = z.array(actionDiffSchema);

export const aiStatusSchema = z.object({
  configured: z.boolean(),
  provider: z.string().default(""),
});

const defaultAIStatus: AIStatus = {
  configured: false,
  provider: "",
};

const defaultActionDiff: ActionDiff = {
  id: "unknown",
  agent: "unknown",
  action: "unknown",
  title: "Untitled Action",
  target: "unknown",
  riskLevel: "write",
  status: "pending",
  createdAt: new Date().toISOString(),
  diff: [],
};

export function parseAIStatus(data: unknown): AIStatus {
  return parseWithFallback(aiStatusSchema, data, defaultAIStatus);
}

export function parseActionDiff(data: unknown): ActionDiff {
  return parseWithFallback(actionDiffSchema, data, defaultActionDiff);
}

export function parseActionDiffList(data: unknown): ActionDiff[] {
  return parseWithFallback(actionDiffListSchema, data, []);
}
