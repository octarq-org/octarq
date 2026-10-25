import { useState } from "react";
import { useTranslation } from "../../i18n";
import { ApprovalWidget } from "./types";
import { Button, toast } from "@octarq/plugin-sdk";
import { ShieldAlert, CheckCircle2, XCircle } from "lucide-react";

export function ApprovalCard({ widget }: { widget: ApprovalWidget }) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<"pending" | "approved" | "rejected">(
    widget.status || "pending",
  );
  const [busy, setBusy] = useState(false);

  const handleDecision = async (decision: "approve" | "reject") => {
    if (!widget.approvalId) {
      setStatus(decision === "approve" ? "approved" : "rejected");
      toast.success(
        decision === "approve"
          ? t("a2ui.approved", "Action approved")
          : t("a2ui.rejected", "Action rejected"),
      );
      return;
    }

    setBusy(true);
    try {
      const res = await fetch(`/api/ai/approvals/${widget.approvalId}/${decision}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.detail || errData.message || "Failed to process decision");
      }
      setStatus(decision === "approve" ? "approved" : "rejected");
      toast.success(
        decision === "approve"
          ? t("a2ui.approved", "Action approved and resumed execution.")
          : t("a2ui.rejected", "Action rejected and cancelled."),
      );
    } catch (err: any) {
      toast.error(err.message || t("a2ui.decisionFailed", "Failed to submit approval decision"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      role="region"
      aria-label={widget.title || t("a2ui.approval", "Human-in-the-Loop Approval Required")}
      className="my-2 rounded-xl border border-warning-border bg-warning-bg/20 p-3.5 text-xs shadow-xs space-y-2.5"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1.5 font-semibold text-foreground">
          <ShieldAlert className="h-4 w-4 text-warning-fg shrink-0" />
          <span>{widget.title || t("a2ui.approvalTitle", "High-Risk Action Pending Approval")}</span>
        </div>
        <span className="font-mono text-[10px] px-2 py-0.5 rounded-full bg-surface-hover text-foreground font-medium">
          {widget.tool}
        </span>
      </div>

      {widget.args && Object.keys(widget.args).length > 0 && (
        <pre className="font-mono text-[11px] p-2 rounded-lg bg-card border border-border text-foreground/80 overflow-x-auto max-h-36">
          {JSON.stringify(widget.args, null, 2)}
        </pre>
      )}

      <div className="flex items-center justify-between pt-1">
        {status === "pending" ? (
          <div className="flex items-center gap-2 w-full justify-end">
            <Button
              variant="subtle"
              className="text-xs py-1 px-3 border-0 text-destructive hover:bg-destructive/10"
              disabled={busy}
              onClick={() => handleDecision("reject")}
            >
              {t("a2ui.reject", "Reject")}
            </Button>
            <Button
              variant="primary"
              className="text-xs py-1 px-3"
              disabled={busy}
              onClick={() => handleDecision("approve")}
            >
              {busy ? t("a2ui.submitting", "Submitting...") : t("a2ui.approve", "Approve & Execute")}
            </Button>
          </div>
        ) : status === "approved" ? (
          <div className="flex items-center gap-1.5 text-success-fg font-medium text-xs">
            <CheckCircle2 className="h-3.5 w-3.5" />
            <span>{t("a2ui.decisionApproved", "Approved")}</span>
          </div>
        ) : (
          <div className="flex items-center gap-1.5 text-destructive font-medium text-xs">
            <XCircle className="h-3.5 w-3.5" />
            <span>{t("a2ui.decisionRejected", "Rejected")}</span>
          </div>
        )}
      </div>
    </div>
  );
}
