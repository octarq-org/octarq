/**
 * ResumeRankCanvas: Interactive candidate ranking matrix and interview invitation editor.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState, useMemo } from "react";
import { Button, Table, THead, TBody, TR, TH, TD } from "@octarq/plugin-sdk";
import { Search, ArrowUpDown, Check, User, Mail, Award, X } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { CandidateItem } from "../../cards/ResumeRankCard";
import { CanvasApprovalGate } from "../CanvasApprovalGate";
import type { CardActionHandler } from "../../cards/registry";
import { useCopilotStore } from "../../store";

export interface ResumeRankCanvasProps {
  artifactId: string;
  content: {
    job_title?: string;
    total_candidates?: number;
    shortlist_count?: number;
    candidates?: CandidateItem[];
    approval_id?: string;
  };
  onAction: CardActionHandler;
}

export function ResumeRankCanvas({ artifactId, content, onAction }: ResumeRankCanvasProps) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [sortField, setSortField] = useState<"rank" | "score" | "experience_years">("rank");
  const [sortAsc, setSortAsc] = useState(true);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => {
    const top = (content.candidates || []).slice(0, content.shortlist_count || 5);
    return new Set(top.map((c, i) => c.id || `cand-${i + 1}`));
  });
  const [activeCandidate, setActiveCandidate] = useState<CandidateItem | null>(null);

  const candidates = content.candidates || [];

  const filteredCandidates = useMemo(() => {
    return candidates
      .filter((c) => {
        if (!search.trim()) return true;
        const q = search.toLowerCase();
        return (
          c.name.toLowerCase().includes(q) ||
          (c.current_title && c.current_title.toLowerCase().includes(q)) ||
          (c.strengths && c.strengths.some((s) => s.toLowerCase().includes(q)))
        );
      })
      .sort((a, b) => {
        const valA = a[sortField] ?? 0;
        const valB = b[sortField] ?? 0;
        if (valA < valB) return sortAsc ? -1 : 1;
        if (valA > valB) return sortAsc ? 1 : -1;
        return 0;
      });
  }, [candidates, search, sortField, sortAsc]);

  const toggleSort = (field: "rank" | "score" | "experience_years") => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(field === "rank");
    }
  };

  const toggleSelect = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    setSelectedIds(next);
  };

  const approvalId = content.approval_id || "";

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-y-auto" data-testid="resume-rank-canvas">
      {/* Header Controls */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {`${content.job_title || "Job Requisition"} ${t("copilot.resumeLeaderboard", "— Candidate Leaderboard")}`}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.resumeLeaderboardDesc", "Evaluate applicants, inspect rubric scores, and stage invitations.")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="absolute left-2.5 top-2 h-3.5 w-3.5 text-muted-foreground" />
            <input
              type="text"
              placeholder={t("copilot.resumeFilterPlaceholder", "Filter by name or skill…")}
              aria-label={t("copilot.resumeFilterAria", "Filter candidates")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8 rounded-lg border border-border bg-background pl-8 pr-3 text-xs text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <span className="rounded-full bg-accent-soft px-2.5 py-1 text-xs font-semibold text-accent-fg">
            {`${selectedIds.size} ${t("copilot.resumeSelected", "Selected")}`}
          </span>
        </div>
      </div>

      {/* Main Table */}
      <div className="flex-1 overflow-auto rounded-xl border border-border bg-card">
        <Table className="w-full text-left text-xs border-collapse">
          <THead>
            <TR className="border-b border-border bg-well font-semibold text-muted-foreground">
              <TH className="py-2 px-3 w-8">
                <input
                  type="checkbox"
                  aria-label={t("copilot.resumeSelectAllAria", "Select all filtered candidates")}
                  checked={filteredCandidates.length > 0 && filteredCandidates.every((c, i) => selectedIds.has(c.id || `cand-${i + 1}`))}
                  onChange={(e) => {
                    const next = new Set(selectedIds);
                    if (e.target.checked) {
                      filteredCandidates.forEach((c, i) => next.add(c.id || `cand-${i + 1}`));
                    } else {
                      filteredCandidates.forEach((c, i) => next.delete(c.id || `cand-${i + 1}`));
                    }
                    setSelectedIds(next);
                  }}
                  className="rounded border-border"
                />
              </TH>
              <TH
                className="py-2 px-3 cursor-pointer select-none hover:text-foreground"
                onClick={() => toggleSort("rank")}
              >
                <div className="flex items-center gap-1">
                  <span>{t("copilot.resumeColRank", "Rank")}</span>
                  <ArrowUpDown className="h-3 w-3" />
                </div>
              </TH>
              <TH className="py-2 px-3">{t("copilot.resumeColCandidate", "Candidate")}</TH>
              <TH
                className="py-2 px-3 cursor-pointer select-none hover:text-foreground"
                onClick={() => toggleSort("score")}
              >
                <div className="flex items-center gap-1">
                  <span>{t("copilot.resumeColScore", "Score")}</span>
                  <ArrowUpDown className="h-3 w-3" />
                </div>
              </TH>
              <TH
                className="py-2 px-3 cursor-pointer select-none hover:text-foreground"
                onClick={() => toggleSort("experience_years")}
              >
                <div className="flex items-center gap-1">
                  <span>{t("copilot.resumeColExp", "Experience")}</span>
                  <ArrowUpDown className="h-3 w-3" />
                </div>
              </TH>
              <TH className="py-2 px-3">{t("copilot.resumeColStrengths", "Core Strengths")}</TH>
              <TH className="py-2 px-3 text-right">{t("copilot.resumeColActions", "Actions")}</TH>
            </TR>
          </THead>
          <TBody className="divide-y divide-border/40">
            {filteredCandidates.map((candidate, idx) => {
              const id = candidate.id || `cand-${idx + 1}`;
              const isSelected = selectedIds.has(id);
              const scorePct = Math.round(candidate.score > 1 ? candidate.score : candidate.score * 100);

              return (
                <TR
                  key={id}
                  className={`hover:bg-surface-hover/50 transition-colors ${
                    isSelected ? "bg-accent-soft" : ""
                  }`}
                  data-testid="candidate-row"
                >
                  <TD className="py-2 px-3">
                    <input
                      type="checkbox"
                      aria-label={`Select ${candidate.name}`}
                      checked={isSelected}
                      onChange={() => toggleSelect(id)}
                      className="rounded border-border"
                    />
                  </TD>
                  <TD className="py-2 px-3 font-mono font-bold text-foreground">
                    #{candidate.rank || idx + 1}
                  </TD>
                  <TD className="py-2 px-3">
                    <div className="font-semibold text-foreground">{candidate.name}</div>
                    <div className="text-[11px] text-muted-foreground">{candidate.current_title}</div>
                  </TD>
                  <TD className="py-2 px-3">
                    <span
                      className={`inline-block rounded-md px-1.5 py-0.5 font-mono text-[11px] font-bold ${
                        scorePct >= 90
                          ? "bg-success-fg/10 text-success-fg"
                          : scorePct >= 70
                            ? "bg-accent-soft text-accent-fg"
                            : "bg-muted text-muted-foreground"
                      }`}
                    >
                      {scorePct}%
                    </span>
                  </TD>
                  <TD className="py-2 px-3 font-mono text-muted-foreground">
                    {candidate.experience_years ? `${candidate.experience_years} yrs` : "—"}
                  </TD>
                  <TD className="py-2 px-3">
                    <div className="flex flex-wrap gap-1 max-w-xs">
                      {(candidate.strengths || []).slice(0, 3).map((st, i) => (
                        <span
                          key={i}
                          className="rounded bg-well px-1.5 py-0.5 text-[10px] text-muted-foreground"
                        >
                          {st}
                        </span>
                      ))}
                    </div>
                  </TD>
                  <TD className="py-2 px-3 text-right">
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => setActiveCandidate(candidate)}
                      className="h-6 text-[11px] px-2"
                    >
                      {t("copilot.resumeInspect", "Inspect")}
                    </Button>
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      </div>

      {/* Candidate Profile Drawer / Modal */}
      {activeCandidate && (
        <div
          data-testid="candidate-detail-modal"
          className="rounded-xl border border-border bg-well p-3 space-y-2 text-xs"
        >
          <div className="flex items-center justify-between border-b border-border pb-2">
            <div>
              <div className="text-sm font-bold text-foreground">{activeCandidate.name}</div>
              <div className="text-xs text-muted-foreground">{activeCandidate.current_title}</div>
            </div>
            <button
              onClick={() => setActiveCandidate(null)}
              className="rounded p-1 text-muted-foreground hover:bg-surface-hover hover:text-foreground"
              aria-label={t("copilot.resumeCloseDetailsAria", "Close candidate details")}
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <p className="text-xs text-foreground leading-relaxed">
            {activeCandidate.summary || "Candidate matches key criteria for this requisition."}
          </p>
          <div className="grid grid-cols-2 gap-2 pt-1">
            <div>
              <span className="text-[11px] font-semibold text-success-fg">{t("copilot.resumeStrengths", "Strengths:")}</span>
              <ul className="list-disc list-inside text-[11px] text-muted-foreground">
                {(activeCandidate.strengths || []).map((s, i) => (
                  <li key={i}>{s}</li>
                ))}
              </ul>
            </div>
            <div>
              <span className="text-[11px] font-semibold text-danger-fg">{t("copilot.resumeRedFlags", "Red Flags:")}</span>
              <ul className="list-disc list-inside text-[11px] text-muted-foreground">
                {(activeCandidate.red_flags || ["None detected"]).map((f, i) => (
                  <li key={i}>{f}</li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Fail-Closed Approval Gate */}
      <CanvasApprovalGate
        approvalId={approvalId}
        toolName="recruiting_dispatch_invites"
        actionLabel="Send Selected Invitations"
        riskLevel="write"
        description={t("copilot.resumeGateDesc", "Dispatch formal interview invitations to selected candidates.")}
        payload={{
          candidate_ids: Array.from(selectedIds),
          count: selectedIds.size,
        }}
        onAction={onAction}
        disabled={selectedIds.size === 0}
      />
    </div>
  );
}
