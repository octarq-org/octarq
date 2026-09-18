import React, { useState } from "react";
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  Clock,
  Shield,
  ShieldAlert,
  XCircle,
  Check,
  X,
  ArrowRight,
} from "lucide-react";
import { useTranslation } from "../i18n";
import { Button, cn, timeAgo } from "../ui";
import type { ActionDiff, RiskLevel, ApprovalStatus } from "./types";
import { useCopilotStore } from "./store";
import { AIStreamError, decideApproval } from "./api";

interface ActionDiffCardProps {
  action: ActionDiff;
  onApprove?: (id: string) => void;
  onReject?: (id: string) => void;
  compact?: boolean;
  className?: string;
}

export function ActionDiffCard({
  action,
  onApprove,
  onReject,
  compact = false,
  className,
}: ActionDiffCardProps) {
  const { t } = useTranslation();
  const approveAction = useCopilotStore((s) => s.approveAction);
  const rejectAction = useCopilotStore((s) => s.rejectAction);
  const syncApprovalStatus = useCopilotStore((s) => s.syncApprovalStatus);

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [decisionError, setDecisionError] = useState<string | null>(null);

  // Cards materialized from /api/ai/approvals carry approvalId + token and
  // must decide through the atomic CAS endpoints. Cards without a binding
  // (e.g. inline diffs from chat tool calls) fall back to local state, and an
  // explicit onApprove/onReject prop always wins (used by tests/embeds).
  const decideRemote = async (decision: "approve" | "reject") => {
    setIsSubmitting(true);
    setDecisionError(null);
    try {
      const result = await decideApproval(action.approvalId as string, decision, action.approvalToken);
      if (result.outcome === "approved") {
        syncApprovalStatus(action.id, "approved", { approver: result.approver ?? "Operator" });
      } else if (result.outcome === "rejected") {
        syncApprovalStatus(action.id, "rejected", { rejectReason: "Rejected by operator" });
      } else if (result.outcome === "expired" || result.outcome === "not_found") {
        syncApprovalStatus(action.id, "expired", { rejectReason: t("copilot.decisionExpired", "审批已过期或不存在") });
      } else {
        syncApprovalStatus(action.id, "expired", {
          rejectReason: t("copilot.decisionConflict", "已被他人处理，本地状态已同步"),
        });
      }
    } catch (err) {
      if (err instanceof AIStreamError && err.status === 409) {
        syncApprovalStatus(action.id, "expired", {
          rejectReason: t("copilot.decisionConflict", "已被他人处理，本地状态已同步"),
        });
        return;
      }
      if (err instanceof AIStreamError && err.status === 410) {
        syncApprovalStatus(action.id, "expired", { rejectReason: t("copilot.decisionExpired", "审批已过期或不存在") });
        return;
      }
      setDecisionError(err instanceof Error ? err.message : t("copilot.decisionFailed", "审批请求失败，请重试"));
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleApprove = async () => {
    if (onApprove) {
      setIsSubmitting(true);
      try {
        await onApprove(action.id);
      } finally {
        setIsSubmitting(false);
      }
      return;
    }
    if (action.approvalId) {
      await decideRemote("approve");
      return;
    }
    setIsSubmitting(true);
    try {
      approveAction(action.id);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleReject = async () => {
    if (onReject) {
      setIsSubmitting(true);
      try {
        await onReject(action.id);
      } finally {
        setIsSubmitting(false);
      }
      return;
    }
    if (action.approvalId) {
      await decideRemote("reject");
      return;
    }
    setIsSubmitting(true);
    try {
      rejectAction(action.id);
    } finally {
      setIsSubmitting(false);
    }
  };

  const isPending = action.status === "pending";

  return (
    <div
      className={cn(
        "rounded-2xl border bg-card/90 p-4 shadow-xs transition-all",
        action.riskLevel === "destructive"
          ? "border-danger-fg/30 hover:border-danger-fg/50"
          : "border-border hover:border-foreground/20",
        className,
      )}
      data-testid={`action-diff-card-${action.id}`}
    >
      {/* Header Bar */}
      <div className="flex items-start justify-between gap-2 border-b border-border/60 pb-3">
        <div className="flex flex-wrap items-center gap-2">
          {/* Agent Badge */}
          <span className="inline-flex items-center gap-1.5 rounded-lg bg-surface-hover/80 px-2.5 py-1 text-xs font-semibold text-foreground">
            <Bot className="h-3.5 w-3.5 text-primary" />
            <span>{action.agent}</span>
          </span>

          {/* Risk Level Badge */}
          <RiskBadge risk={action.riskLevel} t={t} />
        </div>

        {/* Status Badge */}
        <StatusBadge status={action.status} t={t} />
      </div>

      {/* Title & Target */}
      <div className="pt-3 space-y-1">
        <h4 className="text-sm font-semibold text-foreground flex items-center gap-1.5">
          {action.title}
        </h4>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="font-medium text-foreground/70">{t("copilot.targetResource", "目标资源")}:</span>
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-foreground font-semibold">
            {action.target}
          </code>
        </div>
      </div>

      {/* Reason / Context */}
      {action.reason && (
        <p className="mt-2.5 rounded-xl border border-border/50 bg-well/40 p-2.5 text-xs leading-relaxed text-muted-foreground">
          <span className="font-semibold text-foreground/80">{t("copilot.changeReason", "变更说明")}: </span>
          {action.reason}
        </p>
      )}

      {/* Diff Table / Key-value compare */}
      {action.diff.length > 0 && (
        <div className="mt-3 overflow-hidden rounded-xl border border-border bg-well/30 text-xs">
          <div className="grid grid-cols-12 border-b border-border bg-muted/40 px-3 py-1.5 font-medium text-muted-foreground text-[11px]">
            <div className="col-span-4">{t("copilot.diffField", "字段")}</div>
            <div className="col-span-4">{t("copilot.diffBefore", "现行原值")}</div>
            <div className="col-span-4">{t("copilot.diffAfter", "提议变更")}</div>
          </div>
          <div className="divide-y divide-border/60">
            {action.diff.map((item, idx) => {
              const beforeStr = formatDiffValue(item.before);
              const afterStr = formatDiffValue(item.after);
              const isChanged = beforeStr !== afterStr;

              return (
                <div
                  key={idx}
                  className={cn(
                    "grid grid-cols-12 px-3 py-2 items-center transition-colors",
                    isChanged ? "bg-card/40" : "",
                  )}
                >
                  <div className="col-span-4 font-mono font-medium text-foreground/90 truncate pr-2">
                    {item.label || item.field}
                  </div>
                  <div className="col-span-4 pr-2 font-mono text-[11px] break-all">
                    <span
                      className={cn(
                        "inline-block rounded px-1.5 py-0.5",
                        isChanged
                          ? "bg-danger-fg/10 text-danger-fg line-through decoration-danger-fg/50"
                          : "text-muted-foreground",
                      )}
                    >
                      {beforeStr}
                    </span>
                  </div>
                  <div className="col-span-4 font-mono text-[11px] break-all">
                    <span
                      className={cn(
                        "inline-block rounded px-1.5 py-0.5 font-semibold",
                        isChanged
                          ? "bg-success-fg/10 text-success-fg"
                          : "text-muted-foreground",
                      )}
                    >
                      {afterStr}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Footer Actions / Meta info */}
      {decisionError && (
        <p
          role="alert"
          className="mt-3 rounded-xl border border-danger-fg/30 bg-danger-fg/10 p-2.5 text-xs leading-relaxed text-danger-fg"
        >
          {decisionError}
        </p>
      )}
      <div className="mt-3.5 flex flex-wrap items-center justify-between gap-2 pt-1">
        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <Clock className="h-3 w-3" />
          <span>{timeAgo(action.createdAt)}</span>
          {action.approver && (
            <>
              <span>·</span>
              <span>{t("copilot.approver", "审批人")}: {action.approver}</span>
            </>
          )}
        </div>

        {/* Action Buttons */}
        {isPending ? (
          <div className="flex items-center gap-2">
            <Button
              variant="subtle"
              size="sm"
              disabled={isSubmitting}
              onClick={handleReject}
              className="h-8 px-3 text-xs text-muted-foreground hover:text-danger-fg hover:bg-danger-fg/10"
              title={t("copilot.rejectAction", "拒绝变更")}
            >
              <X className="mr-1 h-3.5 w-3.5" />
              <span>{t("copilot.rejectAction", "拒绝变更")}</span>
            </Button>
            <Button
              variant={action.riskLevel === "destructive" ? "danger" : "primary"}
              size="sm"
              disabled={isSubmitting}
              onClick={handleApprove}
              className="h-8 px-3 text-xs font-semibold"
              title={t("copilot.approveAction", "确认执行")}
            >
              <Check className="mr-1 h-3.5 w-3.5" />
              <span>{t("copilot.approveAction", "确认执行")}</span>
            </Button>
          </div>
        ) : (
          <div className="text-xs font-medium">
            {action.status === "approved" && (
              <span className="inline-flex items-center gap-1 text-success-fg font-semibold">
                <CheckCircle2 className="h-3.5 w-3.5" />
                <span>{t("copilot.statusApproved", "已批准执行")}</span>
              </span>
            )}
            {action.status === "rejected" && (
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <XCircle className="h-3.5 w-3.5 text-danger-fg" />
                <span>{t("copilot.statusRejected", "已拒绝")}</span>
              </span>
            )}
            {action.status === "expired" && (
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <Clock className="h-3.5 w-3.5" />
                <span>{t("copilot.statusExpired", "已过期")}</span>
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function RiskBadge({ risk, t }: { risk: RiskLevel; t: (k: string, fb?: string) => string }) {
  switch (risk) {
    case "destructive":
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-danger-fg/10 border border-danger-fg/20 px-2 py-0.5 text-[11px] font-bold text-danger-fg">
          <AlertTriangle className="h-3 w-3" />
          <span>{t("copilot.riskDestructive", "高危操作")}</span>
        </span>
      );
    case "write":
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-warning-fg/10 border border-warning-fg/20 px-2 py-0.5 text-[11px] font-bold text-warning-fg">
          <ShieldAlert className="h-3 w-3" />
          <span>{t("copilot.riskWrite", "写入变更")}</span>
        </span>
      );
    case "read":
    default:
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-success-fg/10 border border-success-fg/20 px-2 py-0.5 text-[11px] font-bold text-success-fg">
          <Shield className="h-3 w-3" />
          <span>{t("copilot.riskRead", "只读操作")}</span>
        </span>
      );
  }
}

function StatusBadge({ status, t }: { status: ApprovalStatus; t: (k: string, fb?: string) => string }) {
  switch (status) {
    case "pending":
      return (
        <span className="rounded-full bg-warning-fg/15 px-2 py-0.5 text-[11px] font-semibold text-warning-fg">
          {t("copilot.badgePending", "待审批")}
        </span>
      );
    case "approved":
      return (
        <span className="rounded-full bg-success-fg/15 px-2 py-0.5 text-[11px] font-semibold text-success-fg">
          {t("copilot.badgeApproved", "已批准")}
        </span>
      );
    case "rejected":
      return (
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">
          {t("copilot.badgeRejected", "已拒绝")}
        </span>
      );
    case "expired":
    default:
      return (
        <span className="rounded-full bg-muted/60 px-2 py-0.5 text-[11px] font-semibold text-muted-foreground">
          {t("copilot.badgeExpired", "已过期")}
        </span>
      );
  }
}

function formatDiffValue(val: unknown): string {
  if (val === undefined || val === null) return "-";
  if (typeof val === "object") {
    try {
      return JSON.stringify(val);
    } catch {
      return String(val);
    }
  }
  return String(val);
}
