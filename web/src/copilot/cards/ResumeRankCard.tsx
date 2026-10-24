/**
 * ResumeRankCard: Candidate ranking and screening action card.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Button } from "@octarq/plugin-sdk";
import { Users, Award, ExternalLink, Send, ShieldAlert } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";
import { useCopilotStore, type Artifact } from "../store";

export interface CandidateItem {
  id?: string;
  rank: number;
  name: string;
  score: number;
  current_title?: string;
  experience_years?: number;
  summary?: string;
  strengths?: string[];
  red_flags?: string[];
  status?: string;
}

export interface ResumeRankArgs {
  job_title?: string;
  total_candidates?: number;
  shortlist_count?: number;
  candidates?: CandidateItem[];
  approval_id?: string;
  approvalId?: string;
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
}

export function ResumeRankCard({
  args,
  onAction,
  state: propState,
  toolName = "recruiting_screen",
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const data = (args ?? {}) as ResumeRankArgs;
  const effectiveState = propState ?? data.state;
  const errorDetail = data.errorDetail;
  const forbiddenReason = data.forbiddenReason;

  const jobTitle = data.job_title ?? "Software Engineer";
  const candidates = Array.isArray(data.candidates) ? data.candidates : [];
  const totalCount = data.total_candidates ?? candidates.length;
  const shortlistCount = data.shortlist_count ?? Math.min(candidates.length, 5);
  const approvalId = String(data.approval_id ?? data.approvalId ?? "").trim();

  const artifactId = `artifact-recruiting-${data.job_title ? data.job_title.replace(/\s+/g, "-").toLowerCase() : "screen"}`;

  const handleOpenCanvas = () => {
    const artifact: Artifact = {
      id: artifactId,
      type: "resume_rank",
      title: `${jobTitle} — Candidate Leaderboard`,
      version: 1,
      status: "reviewing",
      content: {
        job_title: jobTitle,
        total_candidates: totalCount,
        shortlist_count: shortlistCount,
        candidates,
        approval_id: approvalId,
      },
      mutatingAction: {
        toolName: "recruiting_dispatch_invites",
        approvalId,
        riskLevel: "write",
        payload: {
          job_title: jobTitle,
          candidate_ids: candidates.slice(0, shortlistCount).map((c) => c.id || c.name),
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

  const handleStageInvites = () => {
    if (!approvalId) return;
    void onAction({
      type: "approve",
      toolName: "recruiting_dispatch_invites",
      approvalId,
      payload: {
        candidate_ids: candidates.slice(0, shortlistCount).map((c) => c.id || c.name),
      },
    });
  };

  return (
    <CardShell
      title={t("copilot.resumeRankCardTitle", "Candidate Ranking & Screening")}
      state={effectiveState}
      errorDetail={errorDetail}
      forbiddenReason={forbiddenReason}
    >
      <div className="space-y-2.5">
        <div className="flex items-center justify-between border-b border-border/40 pb-1.5">
          <div>
            <div className="font-semibold text-foreground text-xs" data-testid="job-title">
              {jobTitle}
            </div>
            <div className="text-[11px] text-muted-foreground">
              {`${totalCount} ${t("copilot.resumeEvaluatedDesc", "applicants evaluated against requisition rubric.")}`}
            </div>
          </div>
          <div className="flex items-center gap-1">
            <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
              {`${totalCount} ${t("copilot.resumeEvaluated", "Evaluated")}`}
            </span>
            <span className="rounded-full bg-success-fg/10 px-2 py-0.5 text-[10px] font-medium text-success-fg">
              {`${t("copilot.resumeTop", "Top")} ${shortlistCount} ${t("copilot.resumeShortlisted", "Shortlisted")}`}
            </span>
          </div>
        </div>

        {candidates.length === 0 ? (
          <div className="text-xs text-muted-foreground py-2 text-center" data-testid="empty-candidates">
            {t("copilot.resumeNoCandidates", "No candidates to display.")}
          </div>
        ) : (
          <div className="space-y-1.5" data-testid="candidates-preview">
            {candidates.slice(0, 3).map((candidate, idx) => (
              <div
                key={candidate.id || idx}
                className="flex items-center justify-between rounded-lg border border-border/50 bg-background/50 px-2.5 py-1.5 text-xs"
              >
                <div className="flex items-center gap-2">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-surface-hover font-mono text-[10px] font-bold text-foreground">
                    #{candidate.rank || idx + 1}
                  </span>
                  <div>
                    <span className="font-medium text-foreground">{candidate.name}</span>
                    {candidate.current_title && (
                      <span className="text-[11px] text-muted-foreground ml-1.5">
                        · {candidate.current_title}
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <span className="rounded-md bg-primary/10 px-1.5 py-0.5 font-mono text-[11px] font-semibold text-primary">
                    {Math.round(candidate.score > 1 ? candidate.score : candidate.score * 100)}%
                  </span>
                </div>
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
            onClick={handleStageInvites}
            data-testid="stage-invites-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <Send className="h-3 w-3" />
            <span>{t("copilot.cardStageInvites", "Stage Invites")}</span>
          </Button>
        </div>
      </div>
    </CardShell>
  );
}
