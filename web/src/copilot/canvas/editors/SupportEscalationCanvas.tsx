/**
 * SupportEscalationCanvas: Customer support incident dossier, RAG KB citations, and escalation routing editor.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { LifeBuoy, AlertCircle, BookOpen, UserCheck, ShieldAlert, ArrowUpRight } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { KBCitation } from "../../cards/SupportEscalationCard";
import { CanvasApprovalGate } from "../CanvasApprovalGate";
import type { CardActionHandler } from "../../cards/registry";

export interface SupportEscalationCanvasProps {
  artifactId: string;
  content: {
    ticket_id?: string;
    customer_email?: string;
    severity?: string;
    summary?: string;
    recommended_owner?: string;
    kb_citations?: KBCitation[];
    resolution_text?: string;
    approval_id?: string;
  };
  onAction: CardActionHandler;
}

export function SupportEscalationCanvas({ artifactId, content, onAction }: SupportEscalationCanvasProps) {
  const { t } = useTranslation();
  const [selectedOwner, setSelectedOwner] = useState(content.recommended_owner || "VP of Infrastructure");
  const [handoverNotes, setHandoverNotes] = useState(
    "High churn risk enterprise client. DNS renewal script encountered loop on apex domain records. Escalate with priority 1."
  );

  const citations = content.kb_citations || [
    {
      article_id: "kb-02",
      title: "DNS & Custom Domain Setup Guide",
      snippet: "Apex domain records require an ALIAS or ANAME record pointing to edge gateway clusters.",
      score: 0.94,
    },
    {
      article_id: "kb-05",
      title: "Incident Escalation & SLA Protocol",
      snippet: "Critical severity outages on Tier Elite accounts require response within 60 minutes.",
      score: 0.88,
    },
  ];

  const approvalId = content.approval_id || "";

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-y-auto" data-testid="support-escalation-canvas">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {`${t("copilot.supportCanvasTitle", "Incident Dossier —")} ${content.ticket_id || "TICK-4091"}`}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.supportCanvasDesc", "Review incident timeline, RAG knowledge citations, and assign escalation owner.")}
          </p>
        </div>
        <span className="rounded-full bg-danger-bg text-danger-fg border border-danger-border px-2.5 py-0.5 text-xs font-bold">
          {content.severity || "CRITICAL"}
        </span>
      </div>

      {/* Customer Information & Complaint Chronology */}
      <div className="rounded-xl border border-border bg-card p-3 space-y-2 text-xs">
        <div className="flex items-center justify-between border-b border-border pb-1.5">
          <div className="font-semibold text-foreground">{t("copilot.supportProfileChronology", "Customer Profile & Incident Chronology")}</div>
          <div className="font-mono text-muted-foreground">{content.customer_email || "cto@acme.com"}</div>
        </div>
        <p className="text-xs text-foreground leading-relaxed">
          {content.summary || "Customer reported production redirect failures across all custom domains. SLA commitment requires immediate resolution."}
        </p>
      </div>

      {/* RAG Knowledge Base Citations */}
      <div className="rounded-xl border border-border bg-card p-3 space-y-2 text-xs">
        <div className="flex items-center gap-1.5 font-semibold text-foreground border-b border-border pb-1.5">
          <BookOpen className="h-4 w-4 text-accent-fg" />
          <span>{t("copilot.supportKbCitations", "RAG Knowledge Base Citations")}</span>
        </div>
        <div className="space-y-1.5">
          {citations.map((cite, idx) => (
            <div key={idx} className="rounded-lg border border-border bg-well p-2 text-xs space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-medium text-foreground">{cite.title}</span>
                {cite.score && (
                  <span className="font-mono text-[10px] text-accent-fg">
                    {`${t("copilot.supportMatch", "Match:")} ${Math.round(cite.score * 100)}%`}
                  </span>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">{cite.snippet}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Escalation Routing & Notes */}
      <div className="rounded-xl border border-border bg-card p-3 space-y-2 text-xs">
        <div className="flex items-center gap-1.5 font-semibold text-foreground border-b border-border pb-1.5">
          <UserCheck className="h-4 w-4 text-accent-fg" />
          <span>{t("copilot.supportTargetHandover", "Escalation Target & Handover Notes")}</span>
        </div>

        <div className="space-y-1">
          <label className="text-[11px] font-semibold text-muted-foreground">{t("copilot.supportAssignee", "Assignee:")}</label>
          <select
            value={selectedOwner}
            aria-label={t("copilot.supportAssigneeAria", "Escalation Target Owner")}
            onChange={(e) => setSelectedOwner(e.target.value)}
            className="w-full rounded border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
          >
            <option value="VP of Infrastructure">{t("copilot.supportRoleVp", "VP of Infrastructure (SLA 30m)")}</option>
            <option value="On-Call Engineering Lead">{t("copilot.supportRoleLead", "On-Call Engineering Lead")}</option>
            <option value="Customer Success Director">{t("copilot.supportRoleSuccess", "Customer Success Director")}</option>
          </select>
        </div>

        <div className="space-y-1">
          <label className="text-[11px] font-semibold text-muted-foreground">{t("copilot.supportHandoverNotes", "Internal Handover Notes:")}</label>
          <textarea
            rows={3}
            aria-label={t("copilot.supportHandoverNotesAria", "Internal handover notes")}
            value={handoverNotes}
            onChange={(e) => setHandoverNotes(e.target.value)}
            className="w-full rounded border border-border bg-background p-2 text-xs text-foreground resize-none focus:border-primary focus:outline-none"
          />
        </div>
      </div>

      {/* Fail-Closed Approval Gate */}
      <CanvasApprovalGate
        approvalId={approvalId}
        toolName="support_execute_escalation"
        actionLabel="Confirm & Dispatch Escalation"
        riskLevel="write"
        description={t("copilot.supportGateDesc", "Escalate ticket to lead engineer. The system will dispatch an immediate notification.")}
        payload={{
          ticket_id: content.ticket_id || "TICK-4091",
          target_owner: selectedOwner,
          notes: handoverNotes,
        }}
        onAction={onAction}
      />
    </div>
  );
}
