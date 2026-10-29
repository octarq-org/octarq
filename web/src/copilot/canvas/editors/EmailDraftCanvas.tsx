/**
 * EmailDraftCanvas: Inbound email security containment and editable reply draft composer.
 *
 * Invariant: Inbound email body is isolated inside <untrusted> containment styling.
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { Mail, ShieldAlert, Send, Sparkles, User, CornerDownLeft } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { EmailThreadItem } from "../../cards/EmailManagementCard";
import { CanvasApprovalGate } from "../CanvasApprovalGate";
import type { CardActionHandler } from "../../cards/registry";
import { useCopilotStore } from "../../store";

export interface EmailDraftCanvasProps {
  artifactId: string;
  content: {
    total_unread?: number;
    threads?: EmailThreadItem[];
    active_thread_id?: string;
    approval_id?: string;
  };
  onAction: CardActionHandler;
}

export function EmailDraftCanvas({ artifactId, content, onAction }: EmailDraftCanvasProps) {
  const { t } = useTranslation();
  const threads = content.threads || [];
  const [activeThreadIndex, setActiveThreadIndex] = useState(0);

  const currentThread: EmailThreadItem = threads[activeThreadIndex] || {
    id: "mail-1",
    sender: "david.miller@acme.com",
    subject: "Custom Domain DNS Setup & High-Volume Redirect Questions",
    summary: "Customer inquires about setting up 450 custom domains with SOC 2 compliance and 25M redirects.",
    draft_reply: {
      to: "david.miller@acme.com",
      subject: "Re: Custom Domain DNS Setup & High-Volume Redirect Questions",
      body: "Hi David,\n\nThank you for reaching out. Octarq Pro supports unlimited apex and wildcard custom domains with automated Let's Encrypt certificates. Our distributed edge routing easily processes 25 million monthly redirects with sub-20ms latency.\n\nI have prepared a formal price quotation for your team. Would Tuesday at 2 PM EST work for a brief technical walkthrough?\n\nBest regards,\nSarah Jenkins\nOctarq Solutions",
    },
  };

  const initialDraft = currentThread.draft_reply || {
    to: currentThread.sender || "client@example.com",
    subject: `Re: ${currentThread.subject}`,
    body: "Thank you for contacting us. We have reviewed your request.",
  };

  const [to, setTo] = useState(initialDraft.to);
  const [subject, setSubject] = useState(initialDraft.subject);
  const [body, setBody] = useState(initialDraft.body);

  const syncDraft = (nextTo: string, nextSubject: string, nextBody: string) => {
    useCopilotStore.getState().updateArtifactContent(artifactId, (prev: any) => {
      const prevThreads = prev.threads || [];
      const updatedThreads = prevThreads.length > 0
        ? prevThreads.map((t: any, idx: number) => {
            if (idx === activeThreadIndex) {
              return {
                ...t,
                draft_reply: {
                  ...(t.draft_reply || {}),
                  to: nextTo,
                  subject: nextSubject,
                  body: nextBody,
                },
              };
            }
            return t;
          })
        : [
            {
              id: currentThread.id,
              sender: currentThread.sender,
              subject: currentThread.subject,
              summary: currentThread.summary,
              draft_reply: { to: nextTo, subject: nextSubject, body: nextBody },
            },
          ];
      return {
        ...prev,
        threads: updatedThreads,
        draft_reply: { to: nextTo, subject: nextSubject, body: nextBody },
      };
    });
  };

  const handleToChange = (val: string) => {
    setTo(val);
    syncDraft(val, subject, body);
  };

  const handleSubjectChange = (val: string) => {
    setSubject(val);
    syncDraft(to, val, body);
  };

  const handleBodyChange = (val: string) => {
    setBody(val);
    syncDraft(to, subject, val);
  };

  const handleApplyTone = (tone: "concise" | "formal" | "urgent") => {
    let updated = body;
    if (tone === "concise") {
      updated = "Hi David,\n\nOctarq Pro supports 450 custom domains and 25M monthly redirects with sub-20ms latency.\n\nQuotation attached. Does Tuesday at 2 PM EST work for a technical call?\n\nBest,\nSarah";
    } else if (tone === "formal") {
      updated = "Dear Mr. Miller,\n\nWe acknowledge receipt of your inquiry regarding enterprise domain routing. Octarq Pro complies fully with SOC 2 Type II controls and scales beyond 25 million monthly transactions.\n\nWe would welcome an opportunity to present our architectural overview on Tuesday, October 27 at 2:00 PM EST.\n\nSincerely,\nSarah Jenkins\nDirector of Enterprise Solutions";
    }
    setBody(updated);
    syncDraft(to, subject, updated);
  };

  const approvalId = String(currentThread.approval_id || content.approval_id || "").trim();

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-y-auto" data-testid="email-draft-canvas">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {t("copilot.emailWorkspaceTitle", "Inbound Email Workspace & Staged Reply")}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.emailWorkspaceDesc", "Inspect untrusted email content and compose verified outbound responses.")}
          </p>
        </div>
        {threads.length > 1 && (
          <div className="flex rounded-lg bg-well p-0.5 text-xs">
            {threads.map((tItem, idx) => (
              <button
                key={tItem.id || idx}
                onClick={() => {
                  setActiveThreadIndex(idx);
                  if (tItem.draft_reply) {
                    setTo(tItem.draft_reply.to);
                    setSubject(tItem.draft_reply.subject);
                    setBody(tItem.draft_reply.body);
                  }
                }}
                className={`rounded px-2.5 py-1 transition-colors ${
                  activeThreadIndex === idx
                    ? "bg-background text-foreground shadow-xs font-semibold"
                    : "text-muted-foreground"
                }`}
              >
                #{idx + 1}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Untrusted Inbound Message Containment */}
      <div
        data-testid="untrusted-email-container"
        className="rounded-xl border border-warning-fg/30 bg-warning-fg/5 p-3 space-y-2 text-xs"
      >
        <div className="flex items-center justify-between border-b border-warning-fg/20 pb-1.5">
          <div className="flex items-center gap-1.5 font-semibold text-warning-fg">
            <ShieldAlert className="h-4 w-4" />
            <span>{t("copilot.emailUntrustedContainer", "Untrusted External Ingress Container")}</span>
          </div>
          <span className="font-mono text-[10px] text-muted-foreground">
            {`${t("copilot.emailFrom", "From:")} ${currentThread.sender || currentThread.from}`}
          </span>
        </div>

        <div className="text-xs text-foreground font-medium">
          {`${t("copilot.emailSubject", "Subject:")} ${currentThread.subject}`}
        </div>
        <div className="text-xs text-muted-foreground leading-relaxed italic bg-well p-2.5 rounded-lg border border-border">
          {currentThread.summary}
        </div>
      </div>

      {/* Draft Composer Pane */}
      <div className="flex-1 flex flex-col rounded-xl border border-border bg-card p-3 space-y-2 text-xs">
        <div className="flex items-center justify-between border-b border-border pb-2">
          <span className="font-semibold text-foreground flex items-center gap-1.5">
            <CornerDownLeft className="h-3.5 w-3.5 text-accent-fg" />
            <span>{t("copilot.emailOutboundDraft", "Outbound Reply Draft")}</span>
          </span>
          <div className="flex items-center gap-1 text-[11px]">
            <span className="text-muted-foreground">{t("copilot.emailQuickTone", "Quick tone:")}</span>
            <button
              onClick={() => handleApplyTone("concise")}
              className="rounded bg-well px-2 py-0.5 hover:bg-surface-hover text-foreground font-medium"
            >
              {t("copilot.emailToneConcise", "Concise")}
            </button>
            <button
              onClick={() => handleApplyTone("formal")}
              className="rounded bg-well px-2 py-0.5 hover:bg-surface-hover text-foreground font-medium"
            >
              {t("copilot.emailToneFormal", "Formal")}
            </button>
          </div>
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center gap-2">
            <label className="w-14 text-[11px] font-semibold text-muted-foreground">{t("copilot.emailTo", "To:")}</label>
            <input
              type="text"
              aria-label={t("copilot.emailRecipientAria", "Recipient email")}
              value={to}
              onChange={(e) => handleToChange(e.target.value)}
              className="flex-1 rounded border border-border bg-background px-2.5 py-1 text-xs text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div className="flex items-center gap-2">
            <label className="w-14 text-[11px] font-semibold text-muted-foreground">{t("copilot.emailSubject", "Subject:")}</label>
            <input
              type="text"
              aria-label={t("copilot.emailSubjectAria", "Email subject")}
              value={subject}
              onChange={(e) => handleSubjectChange(e.target.value)}
              className="flex-1 rounded border border-border bg-background px-2.5 py-1 text-xs text-foreground focus:border-primary focus:outline-none"
            />
          </div>
        </div>

        <textarea
          rows={7}
          aria-label={t("copilot.emailBodyAria", "Email reply body")}
          value={body}
          onChange={(e) => handleBodyChange(e.target.value)}
          className="flex-1 w-full rounded-lg border border-border bg-background p-3 font-mono text-xs leading-relaxed text-foreground resize-none focus:border-primary focus:outline-none"
        />
      </div>

      {/* Fail-Closed Approval Gate */}
      <CanvasApprovalGate
        approvalId={approvalId}
        toolName="mail_send_reply"
        actionLabel="Approve & Send Email Reply"
        riskLevel="destructive"
        description={t("copilot.emailGateDesc", "Send email reply to client. The system will deliver the message immediately.")}
        payload={{
          thread_id: currentThread.id,
          to,
          subject,
          body,
        }}
        onAction={onAction}
      />
    </div>
  );
}
