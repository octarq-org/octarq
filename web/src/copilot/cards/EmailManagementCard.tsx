/**
 * EmailManagementCard: Inbound email parsing and staged reply drafts card.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Button } from "@octarq/plugin-sdk";
import { Mail, Send, ExternalLink, ShieldAlert, AlertCircle } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";
import { useCopilotStore, type Artifact } from "../store";

export interface EmailThreadItem {
  id?: string;
  sender?: string;
  from?: string;
  subject: string;
  summary: string;
  urgency?: "low" | "medium" | "high" | "urgent" | string;
  priority?: string;
  draft_reply?: {
    to: string;
    subject: string;
    body: string;
    cc?: string[];
  };
  approval_id?: string;
}

export interface EmailManagementArgs {
  total_unread?: number;
  total_scanned?: number;
  unread_only?: boolean;
  limit?: number;
  emails?: EmailThreadItem[];
  threads?: EmailThreadItem[];
  approval_id?: string;
  approvalId?: string;
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
}

export function EmailManagementCard({
  args,
  onAction,
  state: propState,
  toolName = "email_manage",
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const data = (args ?? {}) as EmailManagementArgs;
  const effectiveState = propState ?? data.state;
  const errorDetail = data.errorDetail;
  const forbiddenReason = data.forbiddenReason;

  const rawThreads = Array.isArray(data.threads)
    ? data.threads
    : Array.isArray(data.emails)
      ? data.emails
      : [];
  const threads = [...rawThreads];

  if (
    threads.length === 0 &&
    Boolean((data as any).to || (data as any).recipient || (data as any).subject || (data as any).body)
  ) {
    const to = String((data as any).to || (data as any).recipient || "");
    const subject = String((data as any).subject || "Email Draft");
    const body = String((data as any).body || "");
    threads.push({
      id: String((data as any).id || "draft-1"),
      sender: to,
      subject,
      summary: body.length > 120 ? `${body.slice(0, 120)}…` : body || "Email draft created by Copilot.",
      draft_reply: {
        to,
        subject,
        body,
        cc: (data as any).cc,
      },
      approval_id: data.approval_id || data.approvalId,
    });
  }

  const totalCount = data.total_unread ?? data.total_scanned ?? threads.length;
  const approvalId = String(data.approval_id ?? data.approvalId ?? "").trim();

  const artifactId = `artifact-email-${threads[0]?.id || "inbox-batch"}`;

  const handleOpenCanvas = () => {
    const artifact: Artifact = {
      id: artifactId,
      type: "email_draft",
      title: "Inbound Email Workspace & Staged Drafts",
      version: 1,
      status: "reviewing",
      content: {
        total_unread: totalCount,
        threads,
        active_thread_id: threads[0]?.id,
        approval_id: approvalId,
      },
      mutatingAction: {
        toolName: "mail_send_reply",
        approvalId,
        riskLevel: "destructive",
        payload: {
          thread_id: threads[0]?.id,
          to: threads[0]?.draft_reply?.to || threads[0]?.sender,
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

  const handleSendReplies = () => {
    if (!approvalId) return;
    void onAction({
      type: "approve",
      toolName: "mail_send_reply",
      approvalId,
      payload: {
        sent_count: threads.length,
      },
    });
  };

  return (
    <CardShell
      title={t("copilot.emailCardTitle", "Email Inbound & Draft Replies")}
      state={effectiveState}
      errorDetail={errorDetail}
      forbiddenReason={forbiddenReason}
    >
      <div className="space-y-2.5">
        <div className="flex items-center justify-between border-b border-border/40 pb-1.5">
          <div className="flex items-center gap-1.5">
            <Mail className="h-3.5 w-3.5 text-muted-foreground" />
            <div>
              <div className="font-semibold text-foreground text-xs" data-testid="email-count">
                {`${totalCount} ${t("copilot.emailInboundMessages", "Inbound Messages")}`}
              </div>
              <div className="text-[11px] text-muted-foreground">
                {t("copilot.emailExtractedSummary", "Extracted operational items and prepared replies.")}
              </div>
            </div>
          </div>
          <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
            {`${threads.length} ${t("copilot.emailStagedDrafts", "Staged Drafts")}`}
          </span>
        </div>

        {threads.length === 0 ? (
          <div className="text-xs text-muted-foreground py-2 text-center" data-testid="empty-threads">
            {t("copilot.emailNoEmails", "No emails to display.")}
          </div>
        ) : (
          <div className="space-y-1.5" data-testid="threads-preview">
            {threads.slice(0, 3).map((item, idx) => {
              const sender = item.sender || item.from || "Client";
              const urgency = item.urgency || item.priority || "normal";
              const isUrgent = urgency === "urgent" || urgency === "high";

              return (
                <div
                  key={item.id || idx}
                  className="rounded-lg border border-border/50 bg-background/50 p-2 text-xs space-y-1"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-foreground truncate mr-2">
                      {item.subject}
                    </span>
                    {isUrgent && (
                      <span className="rounded-full bg-destructive/10 px-1.5 py-0.2 text-[10px] font-bold text-destructive shrink-0">
                        {t("copilot.emailUrgent", "Urgent")}
                      </span>
                    )}
                  </div>
                  <div className="text-[11px] text-muted-foreground">
                    {`${t("copilot.emailFrom", "From:")} ${sender}`}
                  </div>
                  <div className="text-xs text-foreground/80 line-clamp-2">
                    {item.summary}
                  </div>
                </div>
              );
            })}
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

        <div className="flex items-center justify-between gap-2 pt-1 border-t border-border/40">
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
            onClick={handleSendReplies}
            data-testid="send-replies-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <Send className="h-3 w-3" />
            <span>{t("copilot.cardSendReplies", "Send Replies")}</span>
          </Button>
        </div>
      </div>
    </CardShell>
  );
}
