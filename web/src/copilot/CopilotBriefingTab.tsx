/**
 * CopilotBriefingTab: 6-Dimension Chief-of-Staff Morning Briefing v2
 * and Multi-dimensional Anomaly Broadcasts.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button, cn } from "@octarq/plugin-sdk";
import {
  Sunrise,
  RefreshCw,
  Calendar,
  CheckCircle2,
  Mail,
  ShieldAlert,
  RotateCcw,
  Activity,
  AlertTriangle,
  ArrowRight,
  TrendingUp,
  Globe2,
  Server,
  Zap,
} from "lucide-react";
import { useTranslation } from "../i18n";
import { useCopilotStore } from "./store";

export interface AnomalyItem {
  id: string;
  dimension: "traffic" | "dns" | "security" | "mail";
  severity: "info" | "warning" | "critical";
  title: string;
  description: string;
  detectedAt: string;
}

export interface BriefingData {
  generatedAt: string;
  headline: string;
  dimensions: {
    schedule: { eventsCount: number; summary: string; conflict: boolean };
    commitments: { overdueCount: number; pendingCount: number; topItem: string };
    priorityMail: { count: number; unreadHighImportance: number; summary: string };
    pendingDecisions: { approvalsCount: number; transactionsCount: number };
    autonomousActions: Array<{ id: number; summary: string; undone: boolean }>;
    infrastructure: { renewingSubscriptions: number; cashflowStatus: string; vpsHealthy: boolean };
  };
  anomalies: AnomalyItem[];
}

const DEFAULT_BRIEFING: BriefingData = {
  generatedAt: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
  headline: "Workspace operations are nominal. 2 items need your review today.",
  dimensions: {
    schedule: {
      eventsCount: 3,
      summary: "Scheduled database backup at 03:00 UTC and weekly operations review at 14:00 UTC.",
      conflict: false,
    },
    commitments: {
      overdueCount: 0,
      pendingCount: 2,
      topItem: "Renew SSL wildcard certificates for *.octarq.io before Friday.",
    },
    priorityMail: {
      count: 4,
      unreadHighImportance: 1,
      summary: "High priority security notice received. 1 OTP verification code pending.",
    },
    pendingDecisions: {
      approvalsCount: 1,
      transactionsCount: 0,
    },
    autonomousActions: [
      { id: 201, summary: "Automated redirect cache purge completed after DNS record update.", undone: false },
    ],
    infrastructure: {
      renewingSubscriptions: 1,
      cashflowStatus: "Positive cashflow ($340 surplus last 24 hours)",
      vpsHealthy: true,
    },
  },
  anomalies: [
    {
      id: "anom-1",
      dimension: "traffic",
      severity: "warning",
      title: "Sudden redirect surge detected",
      description: "Short link /go/launch received 450% higher traffic than normal baseline.",
      detectedAt: "12m ago",
    },
    {
      id: "anom-2",
      dimension: "dns",
      severity: "info",
      title: "DNS propagation verified",
      description: "Edge nodes in APAC and EMEA resolved apex domain changes with sub-15ms latency.",
      detectedAt: "38m ago",
    },
  ],
};

export function CopilotBriefingTab() {
  const { t } = useTranslation();
  const { setActiveTab, setInput } = useCopilotStore();

  const [briefing, setBriefing] = useState<BriefingData | null>(DEFAULT_BRIEFING);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedFilter, setSelectedFilter] = useState<string>("all");

  const handleRefresh = async () => {
    setLoading(true);
    setError(null);
    try {
      // Simulate real refresh / generation cycle
      await new Promise((resolve) => setTimeout(resolve, 350));
      setBriefing({
        ...DEFAULT_BRIEFING,
        generatedAt: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      });
    } catch {
      setError("Briefing generation failed. Please try again.");
    } finally {
      setLoading(false);
    }
  };

  const handleUndoAutoCommit = (commitId: number) => {
    if (!briefing) return;
    setBriefing({
      ...briefing,
      dimensions: {
        ...briefing.dimensions,
        autonomousActions: briefing.dimensions.autonomousActions.map((act) =>
          act.id === commitId ? { ...act, undone: true } : act,
        ),
      },
    });
  };

  const handleInvestigate = (prompt: string) => {
    setInput(prompt);
    setActiveTab("chat");
  };

  const filteredAnomalies =
    briefing?.anomalies.filter((a) => {
      if (selectedFilter === "all") return true;
      return a.dimension === selectedFilter;
    }) ?? [];

  return (
    <div className="flex flex-1 flex-col overflow-y-auto p-4 space-y-5" data-testid="copilot-briefing-tab">
      {/* Briefing Header Banner */}
      <div className="flex items-center justify-between rounded-2xl border border-primary/25 bg-primary/5 p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <Sunrise className="h-5 w-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-bold text-foreground">
                {t("copilot.morningBriefingTitle", "Chief-of-Staff Morning Briefing")}
              </h3>
              <span className="rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold text-primary">
                v2
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              {briefing ? `Updated today at ${briefing.generatedAt}` : t("copilot.generating", "Generating briefing…")}
            </p>
          </div>
        </div>

        <Button
          variant="outline"
          size="sm"
          disabled={loading}
          onClick={handleRefresh}
          className="flex items-center gap-1.5 text-xs"
        >
          <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          <span>{t("common.refresh", "Refresh")}</span>
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3 py-6 animate-pulse" data-testid="briefing-loading-skeleton">
          <div className="h-4 bg-muted rounded w-3/4" />
          <div className="h-16 bg-muted/60 rounded-xl" />
          <div className="h-16 bg-muted/60 rounded-xl" />
        </div>
      ) : error ? (
        <div role="alert" className="rounded-2xl border border-danger-fg/30 bg-danger-fg/10 p-4 text-xs">
          <div className="font-bold text-danger-fg">{error}</div>
          <Button variant="outline" size="sm" onClick={handleRefresh} className="mt-3">
            {t("common.retry", "Retry")}
          </Button>
        </div>
      ) : briefing ? (
        <>
          {/* Executive Headline */}
          <div className="rounded-xl border border-border bg-card p-3.5 shadow-xs">
            <div className="text-xs font-semibold text-foreground leading-relaxed flex items-center gap-2">
              <Zap className="h-4 w-4 text-warning-fg shrink-0" />
              <span>{briefing.headline}</span>
            </div>
          </div>

          {/* 6-Dimension Executive Breakdown */}
          <div className="space-y-2.5">
            <h4 className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground px-1">
              {t("copilot.dimensionsTitle", "6-Dimension Executive Status")}
            </h4>

            {/* 1. Schedule & Agenda */}
            <div className="rounded-xl border border-border bg-well/40 p-3 space-y-1">
              <div className="flex items-center justify-between text-xs font-semibold text-foreground">
                <span className="flex items-center gap-1.5">
                  <Calendar className="h-3.5 w-3.5 text-primary" />
                  <span>{t("copilot.dimSchedule")}</span>
                </span>
                <span className="text-[11px] text-muted-foreground">
                  {briefing.dimensions.schedule.eventsCount} {t("copilot.eventsToday")}
                </span>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {briefing.dimensions.schedule.summary}
              </p>
            </div>

            {/* 2. Tasks & Commitments */}
            <div className="rounded-xl border border-border bg-well/40 p-3 space-y-1">
              <div className="flex items-center justify-between text-xs font-semibold text-foreground">
                <span className="flex items-center gap-1.5">
                  <CheckCircle2 className="h-3.5 w-3.5 text-success-fg" />
                  <span>{t("copilot.dimCommitments")}</span>
                </span>
                <span className="text-[11px] text-muted-foreground">
                  {briefing.dimensions.commitments.pendingCount} {t("copilot.pending")}
                </span>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {briefing.dimensions.commitments.topItem}
              </p>
            </div>

            {/* 3. Priority Communications */}
            <div className="rounded-xl border border-border bg-well/40 p-3 space-y-1">
              <div className="flex items-center justify-between text-xs font-semibold text-foreground">
                <span className="flex items-center gap-1.5">
                  <Mail className="h-3.5 w-3.5 text-accent-fg" />
                  <span>{t("copilot.dimPriorityMail")}</span>
                </span>
                <span className="text-[11px] font-semibold text-primary">
                  {briefing.dimensions.priorityMail.unreadHighImportance} {t("copilot.highPriority")}
                </span>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {briefing.dimensions.priorityMail.summary}
              </p>
            </div>

            {/* 4. Pending Decisions */}
            <div className="rounded-xl border border-border bg-well/40 p-3 space-y-1">
              <div className="flex items-center justify-between text-xs font-semibold text-foreground">
                <span className="flex items-center gap-1.5">
                  <ShieldAlert className="h-3.5 w-3.5 text-warning-fg" />
                  <span>{t("copilot.dimPendingDecisions")}</span>
                </span>
                <span className="text-[11px] font-bold text-danger-fg">
                  {briefing.dimensions.pendingDecisions.approvalsCount} {t("copilot.approvalsAwaiting")}
                </span>
              </div>
              <div className="flex items-center justify-between pt-1">
                <span className="text-xs text-muted-foreground">{t("copilot.reviewDestructiveOps")}</span>
                <Button
                  variant="subtle"
                  size="sm"
                  onClick={() => setActiveTab("approvals")}
                  className="text-xs h-7"
                >
                  {t("copilot.viewApprovals")}
                </Button>
              </div>
            </div>

            {/* 5. Autonomous Actions (TierAuto) & Undo */}
            <div className="rounded-xl border border-border bg-well/40 p-3 space-y-2">
              <div className="flex items-center justify-between text-xs font-semibold text-foreground">
                <span className="flex items-center gap-1.5">
                  <RotateCcw className="h-3.5 w-3.5 text-primary" />
                  <span>{t("copilot.dimAutonomousActions")}</span>
                </span>
                <span className="text-[11px] text-muted-foreground">
                  {briefing.dimensions.autonomousActions.length} {t("copilot.actionsIn24h")}
                </span>
              </div>
              {briefing.dimensions.autonomousActions.map((act) => (
                <div
                  key={act.id}
                  className="flex items-center justify-between gap-2 rounded-lg bg-surface-hover/50 p-2 text-xs"
                >
                  <span className={cn("text-muted-foreground leading-snug", act.undone && "line-through opacity-60")}>
                    {act.summary}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={act.undone}
                    onClick={() => handleUndoAutoCommit(act.id)}
                    className="h-6 px-2 text-[10px] shrink-0"
                  >
                    {act.undone ? "Undone" : "↩️ Undo"}
                  </Button>
                </div>
              ))}
            </div>

            {/* 6. Finance & Infrastructure Health */}
            <div className="rounded-xl border border-border bg-well/40 p-3 space-y-1">
              <div className="flex items-center justify-between text-xs font-semibold text-foreground">
                <span className="flex items-center gap-1.5">
                  <Activity className="h-3.5 w-3.5 text-success-fg" />
                  <span>{t("copilot.dimHealth")}</span>
                </span>
                <span className="text-[10px] font-semibold text-success-fg bg-success-fg/10 px-2 py-0.5 rounded-full">
                  {t("copilot.allSystemsOnline")}
                </span>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {briefing.dimensions.infrastructure.cashflowStatus}. {briefing.dimensions.infrastructure.renewingSubscriptions} {t("copilot.subRenewing")}
              </p>
            </div>
          </div>

          {/* Multi-dimensional Anomaly Broadcasts */}
          <div className="space-y-3 pt-2">
            <div className="flex items-center justify-between">
              <h4 className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground px-1">
                {t("copilot.anomaliesTitle", "Multi-Dimensional Anomaly Broadcasts")}
              </h4>
              <span className="text-[11px] text-muted-foreground">
                {filteredAnomalies.length} {t("copilot.alerts")}
              </span>
            </div>

            {/* Dimension Filter Chips */}
            <div className="flex items-center gap-1 overflow-x-auto pb-1">
              {["all", "traffic", "dns", "security", "mail"].map((filterKey) => (
                <button
                  key={filterKey}
                  type="button"
                  onClick={() => setSelectedFilter(filterKey)}
                  className={cn(
                    "rounded-lg px-2.5 py-1 text-[11px] font-medium capitalize transition-colors shrink-0",
                    selectedFilter === filterKey
                      ? "bg-foreground text-background font-semibold"
                      : "bg-surface-hover text-muted-foreground hover:text-foreground",
                  )}
                >
                  {filterKey}
                </button>
              ))}
            </div>

            {/* Anomalies List */}
            {filteredAnomalies.length === 0 ? (
              <div className="rounded-xl border border-dashed border-border p-6 text-center text-xs text-muted-foreground">
                {t("copilot.noAnomalies")}
              </div>
            ) : (
              <div className="space-y-2">
                {filteredAnomalies.map((anom) => (
                  <div
                    key={anom.id}
                    className="flex flex-col gap-2 rounded-xl border border-border bg-card p-3 shadow-xs"
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        {anom.dimension === "traffic" && <TrendingUp className="h-4 w-4 text-warning-fg" />}
                        {anom.dimension === "dns" && <Globe2 className="h-4 w-4 text-accent-fg" />}
                        {anom.dimension === "security" && <ShieldAlert className="h-4 w-4 text-danger-fg" />}
                        {anom.dimension === "mail" && <Mail className="h-4 w-4 text-primary" />}
                        <span className="text-xs font-bold text-foreground">{anom.title}</span>
                      </div>
                      <span className="text-[10px] text-muted-foreground font-mono">{anom.detectedAt}</span>
                    </div>

                    <p className="text-xs text-muted-foreground leading-relaxed">{anom.description}</p>

                    <div className="flex items-center justify-end pt-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleInvestigate(`Investigate anomaly: ${anom.title}`)}
                        className="text-xs h-7 text-primary hover:text-primary-hover flex items-center gap-1"
                      >
                        <span>{t("copilot.investigateInCopilot")}</span>
                        <ArrowRight className="h-3 w-3" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      ) : null}
    </div>
  );
}
