/**
 * CanvasApprovalGate: Standard Fail-Closed CAS Approval Gate for Canvas Views.
 *
 * Strict Zero-I/O: Dispatches purely through onAction handler.
 * Strict Fail-Closed: If approvalId is missing or empty, action is strictly disabled.
 * All copy adheres to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { ShieldCheck, ShieldAlert, CheckCircle2, XCircle, Clock, AlertTriangle } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { CardAction, CardOutcome } from "../cards/registry";

export interface CanvasApprovalGateProps {
  approvalId?: string;
  approvalToken?: string;
  toolName?: string;
  actionLabel: string;
  riskLevel?: "read" | "write" | "destructive";
  description?: string;
  payload?: Record<string, unknown>;
  onAction: (action: CardAction) => Promise<CardOutcome> | void;
  disabled?: boolean;
}

export function CanvasApprovalGate({
  approvalId = "",
  approvalToken = "",
  toolName,
  actionLabel,
  riskLevel = "write",
  description,
  payload = {},
  onAction,
  disabled = false,
}: CanvasApprovalGateProps) {
  const { t } = useTranslation();
  const [terminalState, setTerminalState] = useState<"decided" | "rejected" | "expired" | "failed" | null>(null);
  const [terminalMessage, setTerminalMessage] = useState<string>("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const hasValidApprovalId = Boolean(approvalId && approvalId.trim() !== "");

  const handleApprove = async () => {
    if (!hasValidApprovalId || disabled || terminalState || isSubmitting) return;

    setIsSubmitting(true);
    try {
      const outcome = await onAction({
        type: "approve",
        approvalId: approvalId.trim(),
        approvalToken,
        toolName,
        payload,
      });

      if (outcome && typeof outcome === "object" && "success" in outcome && (outcome as any).success === false) {
        setTerminalState("failed");
        setTerminalMessage("Action failed. The system made no changes.");
        return;
      }

      const status = outcome && typeof outcome.status === "number" ? outcome.status : 200;
      if (status >= 200 && status < 300) {
        setTerminalState("decided");
        setTerminalMessage("Action approved. The change executed successfully.");
      } else if (status === 409) {
        setTerminalState("decided");
        setTerminalMessage("Action already decided. The system made no changes.");
      } else if (status === 410) {
        setTerminalState("expired");
        setTerminalMessage("Approval ticket expired. Submit a new request.");
      } else {
        setTerminalState("failed");
        setTerminalMessage("Server returned an error. The system made no changes.");
      }
    } catch (err) {
      setTerminalState("failed");
      setTerminalMessage(err instanceof Error ? err.message : "Failed to submit approval.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleReject = async () => {
    if (!hasValidApprovalId || disabled || terminalState || isSubmitting) return;

    setIsSubmitting(true);
    try {
      const outcome = await onAction({
        type: "reject",
        approvalId: approvalId.trim(),
        approvalToken,
        toolName,
        payload,
      });

      if (outcome && typeof outcome === "object" && "success" in outcome && (outcome as any).success === false) {
        setTerminalState("failed");
        setTerminalMessage("Action failed. The system made no changes.");
        return;
      }

      const status = outcome && typeof outcome.status === "number" ? outcome.status : 200;
      if (status === 410) {
        setTerminalState("expired");
        setTerminalMessage("Approval ticket expired. Submit a new request.");
      } else if (status >= 200 && status < 300) {
        setTerminalState("rejected");
        setTerminalMessage("Action rejected. The system made no changes.");
      } else if (status === 409) {
        setTerminalState("decided");
        setTerminalMessage("Action already decided. The system made no changes.");
      } else {
        setTerminalState("failed");
        setTerminalMessage("Server returned an error. The system made no changes.");
      }
    } catch (err) {
      setTerminalState("failed");
      setTerminalMessage(err instanceof Error ? err.message : "Failed to reject approval.");
    } finally {
      setIsSubmitting(false);
    }
  };

  if (terminalState) {
    return (
      <div
        data-testid="gate-terminal"
        data-verdict={terminalState}
        role={terminalState === "failed" ? "alert" : undefined}
        className={`flex items-center justify-between rounded-xl border p-3 text-xs ${
          terminalState === "failed"
            ? "border-destructive/30 bg-destructive/10"
            : "border-border bg-well/40"
        }`}
      >
        <div className="flex items-center gap-2">
          {terminalState === "decided" && <CheckCircle2 className="h-4 w-4 text-success-fg" />}
          {terminalState === "rejected" && <XCircle className="h-4 w-4 text-destructive" />}
          {terminalState === "expired" && <Clock className="h-4 w-4 text-warning-fg" />}
          {terminalState === "failed" && <AlertTriangle className="h-4 w-4 text-destructive" />}
          <span className="font-medium text-foreground">{terminalMessage}</span>
        </div>
        <span className="font-mono text-[11px] text-muted-foreground uppercase">
          {terminalState}
        </span>
      </div>
    );
  }

  return (
    <div
      data-testid="canvas-approval-gate"
      className="rounded-xl border border-border bg-card p-3 space-y-2 text-xs"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5 font-semibold text-foreground">
          <ShieldCheck className="h-4 w-4 text-primary" />
          <span>{t("copilot.operatorAuthorizationGate", "Operator Authorization Gate")}</span>
        </div>
        <span
          className={`rounded-full px-2 py-0.5 text-[10px] font-bold uppercase font-mono ${
            riskLevel === "destructive"
              ? "bg-destructive/15 text-destructive border border-destructive/20"
              : "bg-warning-fg/15 text-warning-fg border border-warning-fg/20"
          }`}
        >
          {riskLevel}
        </span>
      </div>

      {description && (
        <p className="text-[11px] text-muted-foreground">{description}</p>
      )}

      {!hasValidApprovalId && (
        <div
          role="alert"
          data-testid="approval-missing-id"
          className="flex items-center gap-1.5 rounded-lg border border-warning-fg/30 bg-warning-fg/10 p-2 text-[11px] text-warning-fg font-medium"
        >
          <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
          <span>{t("copilot.cardMissingApprovalId", "No approval ID provided. You cannot submit this action.")}</span>
        </div>
      )}

      <div className="flex items-center justify-end gap-2 pt-1 border-t border-border/40">
        <Button
          size="sm"
          variant="secondary"
          disabled={!hasValidApprovalId || disabled || isSubmitting}
          onClick={handleReject}
          data-testid="gate-reject-btn"
          className="h-7 text-xs"
        >
          {t("copilot.rejectAction", "Reject")}
        </Button>
        <Button
          size="sm"
          variant="primary"
          disabled={!hasValidApprovalId || disabled || isSubmitting}
          onClick={handleApprove}
          data-testid="gate-approve-btn"
          className="h-7 text-xs flex items-center gap-1"
        >
          <ShieldCheck className="h-3 w-3" />
          <span>{actionLabel}</span>
        </Button>
      </div>
    </div>
  );
}
