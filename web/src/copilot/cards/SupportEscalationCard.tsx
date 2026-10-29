/**
 * SupportEscalationCard: Support query resolution and high-risk incident escalation card.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Button } from "@octarq/plugin-sdk";
import { LifeBuoy, AlertCircle, ExternalLink, ShieldAlert, ArrowUpRight } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";
import { useCopilotStore, type Artifact } from "../store";

export interface KBCitation {
  article_id?: string;
  title: string;
  snippet?: string;
  score?: number;
}

export interface SupportEscalationArgs {
  ticket_id?: string;
  customer_email?: string;
  plan_tier?: string;
  severity?: "CRITICAL" | "HIGH" | "MEDIUM" | "LOW" | string;
  sentiment?: string;
  sentiment_score?: number;
  high_risk?: boolean;
  issue_summary?: string;
  resolution_text?: string;
  kb_citations?: KBCitation[];
  kb_matches?: KBCitation[];
  escalation?: {
    recommended_owner?: string;
    team?: string;
    manager_summary?: string;
    root_cause_summary?: string;
    action_plan?: string;
    severity?: string;
  };
  approval_id?: string;
  approvalId?: string;
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
}

export function SupportEscalationCard({
  args,
  onAction,
  state: propState,
  toolName = "support_escalate",
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const data = (args ?? {}) as SupportEscalationArgs;
  const effectiveState = propState ?? data.state;
  const errorDetail = data.errorDetail;
  const forbiddenReason = data.forbiddenReason;

  const ticketId = data.ticket_id ?? "TICK-4091";
  const email = data.customer_email ?? "cto@acme.com";
  const severity = data.severity || data.escalation?.severity || (data.high_risk ? "CRITICAL" : "MEDIUM");
  const isCritical = severity === "CRITICAL" || severity === "HIGH" || Boolean(data.high_risk);

  const escalationInfo = data.escalation || {};
  const owner = escalationInfo.recommended_owner || "On-Call Engineering Lead";
  const summary = escalationInfo.root_cause_summary || escalationInfo.manager_summary || data.issue_summary || data.resolution_text || "Production DNS service disruption and redirect failure.";

  const citations = Array.isArray(data.kb_citations)
    ? data.kb_citations
    : Array.isArray(data.kb_matches)
      ? data.kb_matches
      : [];

  const approvalId = String(data.approval_id ?? data.approvalId ?? "").trim();
  const artifactId = `artifact-support-${ticketId.toLowerCase()}`;

  const handleOpenCanvas = () => {
    const artifact: Artifact = {
      id: artifactId,
      type: "support_escalation",
      title: `Incident Dossier — ${ticketId}`,
      version: 1,
      status: "reviewing",
      content: {
        ticket_id: ticketId,
        customer_email: email,
        severity,
        summary,
        recommended_owner: owner,
        kb_citations: citations,
        resolution_text: data.resolution_text || "",
        approval_id: approvalId,
      },
      mutatingAction: {
        toolName: "support_execute_escalation",
        approvalId,
        riskLevel: "write",
        payload: {
          ticket_id: ticketId,
          target_owner: owner,
          severity,
        },
      },
      createdAt: new Date().toISOString(),
    };

    useCopilotStore.getState().openArtifact(artifact);
    void onAction({
      type: "open_canvas",
      toolName,
      artifactId,
      payload: artifact.content,
    });
  };

  const handleEscalate = () => {
    if (!approvalId) return;
    void onAction({
      type: "approve",
      toolName: "support_execute_escalation",
      approvalId,
      payload: {
        ticket_id: ticketId,
        target_owner: owner,
      },
    });
  };

  return (
    <CardShell
      title={t("copilot.supportCardTitle", "Customer Support Ticket Escalation")}
      state={effectiveState}
      errorDetail={errorDetail}
      forbiddenReason={forbiddenReason}
    >
      <div className="space-y-2.5">
        <div className="flex items-center justify-between border-b border-border pb-1.5">
          <div className="flex items-center gap-1.5">
            <LifeBuoy className="h-3.5 w-3.5 text-muted-foreground" />
            <div>
              <div className="font-semibold text-foreground text-xs font-mono" data-testid="ticket-id">
                {ticketId}
              </div>
              <div className="text-[11px] text-muted-foreground">{email}</div>
            </div>
          </div>
          <span
            className={`rounded-full px-2 py-0.5 text-[10px] font-bold ${
              isCritical
                ? "bg-danger-bg text-danger-fg border border-danger-border"
                : "bg-warning-fg/15 text-warning-fg"
            }`}
            data-testid="severity-chip"
          >
            {severity}
          </span>
        </div>

        <div className="rounded-lg border border-border bg-well p-2 text-xs space-y-1">
          <div className="text-[11px] font-semibold text-muted-foreground">
            {t("copilot.supportSummary", "Escalation Summary")}
          </div>
          <div className="text-foreground font-medium">
            {summary}
          </div>
          <div className="text-[11px] text-muted-foreground pt-1 border-t border-border">
            {`${t("copilot.supportRoutingTarget", "Routing Target:")} `}<span className="text-foreground font-medium">{owner}</span>
          </div>
        </div>

        {citations.length > 0 && (
          <div className="space-y-1" data-testid="kb-citations">
            <div className="text-[11px] font-semibold text-muted-foreground">
              {`${t("copilot.supportKbMatches", "Knowledge Base Matches")} (${citations.length})`}
            </div>
            {citations.slice(0, 2).map((cite, idx) => (
              <div
                key={cite.article_id || idx}
                className="rounded-md border border-border bg-well px-2 py-1 text-[11px] text-muted-foreground truncate"
              >
                <span className="font-medium text-foreground">{cite.title}</span>
                {cite.snippet && <span className="ml-1">· {cite.snippet}</span>}
              </div>
            ))}
          </div>
        )}

        {!approvalId && (
          <div
            role="alert"
            data-testid="approval-missing-id"
            className="flex items-center gap-1.5 rounded-lg border border-warning-fg/30 bg-warning-fg/10 p-2 text-[11px] text-warning-fg"
          >
            <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
            <span>{t("copilot.cardMissingApprovalId", "No approval ID provided. You cannot submit this action.")}</span>
          </div>
        )}

        <div className="flex items-center justify-between gap-2 pt-1 border-t border-border">
          <Button
            size="sm"
            variant="secondary"
            onClick={handleOpenCanvas}
            data-testid="open-canvas-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <ExternalLink className="h-3 w-3" />
            <span>{t("copilot.cardOpenInCanvas", "Open in Canvas")}</span>
          </Button>

          <Button
            size="sm"
            variant="primary"
            disabled={!approvalId}
            onClick={handleEscalate}
            data-testid="escalate-ticket-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <ArrowUpRight className="h-3 w-3" />
            <span>{t("copilot.cardEscalateTicket", "Escalate Ticket")}</span>
          </Button>
        </div>
      </div>
    </CardShell>
  );
}
