/**
 * ApprovalCard: Human-In-The-Loop approval card.
 *
 * Renders proposed changes as before/after diffs.
 * Refuses submissions without an approval ID (Fail-Closed P0 rule).
 * Maps 409/410 to terminal states and never repolls.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { Check, X, AlertTriangle, ShieldCheck } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { CardAction, ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";

export interface ApprovalArgs {
  approval_id?: string;
  approvalId?: string;
  _approval_id?: string;
  tool_name?: string;
  toolName?: string;
  summary?: string;
  payload?: unknown;
  before?: unknown;
  after?: unknown;
}

type Verdict = "none" | "decided" | "expired" | "rejected" | "failed";

function pretty(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function ApprovalCard({
  args,
  toolName,
  onAction,
  state,
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const a = (args ?? {}) as ApprovalArgs;
  const [verdict, setVerdict] = useState<Verdict>("none");
  const [detail, setDetail] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const approvalID = (a.approval_id ?? a.approvalId ?? a._approval_id ?? "").trim();
  const missingTicket = approvalID === "";
  const hasDiff = a.before !== undefined || a.after !== undefined;
  const targetTool = a.tool_name ?? a.toolName ?? toolName;

  const submit = async (type: "approve" | "reject") => {
    if (missingTicket) {
      setVerdict("failed");
      setDetail("No approval ID provided. The system submitted nothing.");
      return;
    }
    setSubmitting(true);
    const action: CardAction = { type, approvalId: approvalID };
    try {
      const outcome = await onAction(action);
      if (!outcome) {
        setVerdict(type === "approve" ? "decided" : "rejected");
        return;
      }
      if (outcome.status === 410) {
        setVerdict("expired");
        return;
      }
      if (outcome.status === 409) {
        setVerdict("decided");
        return;
      }
      if (outcome.status >= 200 && outcome.status < 300) {
        setVerdict(type === "approve" ? "decided" : "rejected");
        return;
      }
      setVerdict("failed");
      setDetail(`Server returned status ${outcome.status}.`);
    } catch (err) {
      setVerdict("failed");
      setDetail(err instanceof Error ? err.message : "Transport error occurred.");
    } finally {
      setSubmitting(false);
    }
  };

  const isTerminal = verdict === "decided" || verdict === "expired" || verdict === "rejected";

  return (
    <CardShell title={`Approval Required — ${targetTool || "Action"}`} state={state}>
      {a.summary && <div className="text-xs text-muted-foreground">{a.summary}</div>}

      {hasDiff && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 pt-1">
          <div>
            <div className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">{t("copilot.diffBefore", "Before")}</div>
            <pre data-testid="diff-before" className="mt-1 max-h-36 overflow-auto rounded-lg bg-well p-2 font-mono text-[11px]">
              {pretty(a.before ?? null)}
            </pre>
          </div>
          <div>
            <div className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">{t("copilot.diffAfter", "After")}</div>
            <pre data-testid="diff-after" className="mt-1 max-h-36 overflow-auto rounded-lg bg-well p-2 font-mono text-[11px]">
              {pretty(a.after ?? null)}
            </pre>
          </div>
        </div>
      )}

      {missingTicket && !isTerminal && (
        <div
          role="alert"
          data-testid="approval-missing-id"
          className="flex items-center gap-1.5 rounded-lg border border-warning-fg/30 bg-warning-fg/10 p-2 text-xs text-warning-fg"
        >
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{t("copilot.cardMissingApprovalId", "No approval ID provided. You cannot submit this action.")}</span>
        </div>
      )}

      {isTerminal ? (
        <div
          data-testid="approval-terminal"
          data-verdict={verdict}
          className="flex items-center gap-2 rounded-lg bg-well p-2 text-xs font-medium"
        >
          <ShieldCheck className="h-4 w-4 text-accent-fg" />
          <span>
            {verdict === "decided" && t("copilot.cardApprovedDesc", "Action approved. The change executed successfully.")}
            {verdict === "rejected" && t("copilot.cardRejectedDesc", "Action rejected. The system made no changes.")}
            {verdict === "expired" && t("copilot.cardExpiredDesc", "Approval expired. Submit a new request.")}
          </span>
        </div>
      ) : (
        <div className="flex items-center gap-2 pt-1">
          <Button
            size="sm"
            variant="primary"
            disabled={submitting || missingTicket}
            onClick={() => void submit("approve")}
            className="flex items-center gap-1 h-7 text-xs"
          >
            <Check className="h-3.5 w-3.5" />
            <span>{t("copilot.approveAction", "Approve")}</span>
          </Button>
          <Button
            size="sm"
            variant="danger"
            disabled={submitting || missingTicket}
            onClick={() => void submit("reject")}
            className="flex items-center gap-1 h-7 text-xs"
          >
            <X className="h-3.5 w-3.5" />
            <span>{t("copilot.rejectAction", "Reject")}</span>
          </Button>
          {approvalID && (
            <span className="font-mono text-[11px] text-muted-foreground ml-auto">
              <span>{t("copilot.cardIdPrefix", "ID:")} </span>
              {approvalID.slice(0, 8)}…
            </span>
          )}
        </div>
      )}

      {verdict === "failed" && detail && (
        <div role="alert" className="text-xs text-danger-fg">
          {detail}
        </div>
      )}
    </CardShell>
  );
}
