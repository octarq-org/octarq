/**
 * FinancialAnalysisCard: Budget variance analysis and 3-slide presentation card.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { TrendingUp, TrendingDown, Presentation, ExternalLink, Copy, Check } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";
import { useCopilotStore, type Artifact } from "../store";

export interface PresentationSlide {
  slide_number?: number;
  slide_no?: number;
  title: string;
  subtitle?: string;
  bullets?: string[];
  speaker_notes?: string;
  kpis?: Record<string, unknown>;
}

export interface FinancialAnalysisArgs {
  fiscal_quarter?: string;
  period?: string;
  budget_cents?: number;
  planned_total?: number;
  actual_cents?: number;
  actual_total?: number;
  variance_cents?: number;
  variance_total?: number;
  variance_pct?: number;
  status?: string;
  slides?: PresentationSlide[];
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
}

export function FinancialAnalysisCard({
  args,
  onAction,
  state: propState,
  toolName = "financial_analysis",
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const data = (args ?? {}) as FinancialAnalysisArgs;
  const effectiveState = propState ?? data.state;
  const errorDetail = data.errorDetail;
  const forbiddenReason = data.forbiddenReason;

  const quarter = data.fiscal_quarter ?? data.period ?? "FY26 Q3";
  const budgetCents = data.budget_cents ?? (data.planned_total ? Math.round(data.planned_total * 100) : 34500000);
  const actualCents = data.actual_cents ?? (data.actual_total ? Math.round(data.actual_total * 100) : 41060000);
  const varianceCents = data.variance_cents ?? (data.variance_total ? Math.round(data.variance_total * 100) : actualCents - budgetCents);
  const variancePct = data.variance_pct ?? (budgetCents > 0 ? (varianceCents / budgetCents) * 100 : 19.0);

  const isOverBudget = varianceCents > 0;
  const slides = Array.isArray(data.slides) ? data.slides : [];
  const [copied, setCopied] = useState(false);

  const artifactId = `artifact-financial-${quarter.replace(/\s+/g, "-").toLowerCase()}`;

  const formatCurrency = (cents: number) => {
    return (cents / 100).toLocaleString("en-US", { style: "currency", currency: "USD" });
  };

  const handleOpenCanvas = () => {
    const artifact: Artifact = {
      id: artifactId,
      type: "financial_deck",
      title: `${quarter} Financial Analysis & 3-Slide Presentation`,
      version: 1,
      status: "draft",
      content: {
        fiscal_quarter: quarter,
        budget_cents: budgetCents,
        actual_cents: actualCents,
        variance_cents: varianceCents,
        variance_pct: variancePct,
        slides,
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

  const handleCopyOutline = async () => {
    const text = slides.map((s, idx) => `Slide ${s.slide_number || idx + 1}: ${s.title}\n${(s.bullets || []).map((b) => `• ${b}`).join("\n")}`).join("\n\n");
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <CardShell
      title={t("copilot.financeCardTitle", "Financial Budget Variance Analysis")}
      state={effectiveState}
      errorDetail={errorDetail}
      forbiddenReason={forbiddenReason}
    >
      <div className="space-y-2.5">
        <div className="flex items-center justify-between border-b border-border/40 pb-1.5">
          <div>
            <div className="font-semibold text-foreground text-xs" data-testid="fiscal-quarter">
              {`${quarter} ${t("copilot.financeReconciliation", "Budget Reconciliation")}`}
            </div>
            <div className="text-[11px] text-muted-foreground">
              {t("copilot.financeReconciliationDesc", "Analyzed department ledgers and capital expenditures.")}
            </div>
          </div>
          <span
            className={`rounded-full px-2 py-0.5 text-[10px] font-bold flex items-center gap-1 ${
              isOverBudget
                ? "bg-destructive/15 text-destructive border border-destructive/20"
                : "bg-success-fg/15 text-success-fg border border-success-fg/20"
            }`}
            data-testid="variance-badge"
          >
            {isOverBudget ? <TrendingUp className="h-3 w-3" /> : <TrendingDown className="h-3 w-3" />}
            <span>{isOverBudget ? `+${variancePct.toFixed(1)}% Over` : `${variancePct.toFixed(1)}% Under`}</span>
          </span>
        </div>

        {/* Financial Metrics */}
        <div className="grid grid-cols-3 gap-1.5" data-testid="finance-metrics">
          <div className="rounded-lg border border-border/60 bg-background/50 p-1.5 text-center">
            <div className="text-[10px] text-muted-foreground">{t("copilot.financeBudget", "Budget")}</div>
            <div className="font-mono text-xs font-semibold text-foreground">
              {formatCurrency(budgetCents)}
            </div>
          </div>
          <div className="rounded-lg border border-border/60 bg-background/50 p-1.5 text-center">
            <div className="text-[10px] text-muted-foreground">{t("copilot.financeActual", "Actual")}</div>
            <div className="font-mono text-xs font-semibold text-foreground">
              {formatCurrency(actualCents)}
            </div>
          </div>
          <div
            className={`rounded-lg border p-1.5 text-center ${
              isOverBudget
                ? "border-destructive/30 bg-destructive/5 text-destructive"
                : "border-success-fg/30 bg-success-fg/5 text-success-fg"
            }`}
          >
            <div className="text-[10px]">{isOverBudget ? "Overrun" : "Savings"}</div>
            <div className="font-mono text-xs font-bold">
              {formatCurrency(Math.abs(varianceCents))}
            </div>
          </div>
        </div>

        {/* 3-Slide outline preview */}
        {slides.length > 0 ? (
          <div className="space-y-1" data-testid="slides-preview">
            <div className="text-[11px] font-semibold text-muted-foreground">
              {`${t("copilot.financeSlides", "Presentation Slides")} (${slides.length})`}
            </div>
            {slides.map((slide, idx) => (
              <div
                key={slide.slide_number || idx}
                className="flex items-center justify-between rounded-md border border-border/40 bg-well/20 px-2 py-1 text-xs"
              >
                <span className="font-medium text-foreground truncate mr-2">
                  #{slide.slide_number || idx + 1} {slide.title}
                </span>
                <span className="text-[10px] text-muted-foreground shrink-0 font-mono">
                  {`${(slide.bullets || []).length} ${t("copilot.financeBullets", "bullets")}`}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <div className="text-xs text-muted-foreground py-2 text-center" data-testid="empty-slides">
            {t("copilot.financeNoSlides", "No slides generated.")}
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

          {slides.length > 0 && (
            <Button
              size="sm"
              variant="secondary"
              onClick={handleCopyOutline}
              data-testid="copy-deck-btn"
              className="h-7 text-xs flex items-center gap-1"
            >
              {copied ? <Check className="h-3 w-3 text-success-fg" /> : <Copy className="h-3 w-3" />}
              <span>{copied ? "Copied" : "Copy Outline"}</span>
            </Button>
          )}
        </div>
      </div>
    </CardShell>
  );
}
