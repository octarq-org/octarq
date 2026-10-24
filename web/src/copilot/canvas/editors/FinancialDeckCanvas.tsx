/**
 * FinancialDeckCanvas: 16:9 widescreen 3-slide visual presentation viewer and outline editor.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { Presentation, Play, Edit3, Copy, Check, Plus, Trash2 } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { PresentationSlide } from "../../cards/FinancialAnalysisCard";
import type { CardActionHandler } from "../../cards/registry";
import { useCopilotStore } from "../../store";

export interface FinancialDeckCanvasProps {
  artifactId: string;
  content: {
    fiscal_quarter?: string;
    budget_cents?: number;
    actual_cents?: number;
    variance_cents?: number;
    variance_pct?: number;
    slides?: PresentationSlide[];
  };
  onAction: CardActionHandler;
}

export function FinancialDeckCanvas({ artifactId, content, onAction }: FinancialDeckCanvasProps) {
  const { t } = useTranslation();
  const [activeSlideIndex, setActiveSlideIndex] = useState(0);
  const [presentationMode, setPresentationMode] = useState(false);
  const [copied, setCopied] = useState(false);

  const [slides, setSlides] = useState<PresentationSlide[]>(() => {
    if (content.slides && content.slides.length === 3) return content.slides;
    return [
      {
        slide_number: 1,
        title: `${content.fiscal_quarter || "FY26 Q3"} Financial Health & Executive Snapshot`,
        subtitle: "Budget vs Actual Operating Expenditures & Variance Analysis",
        bullets: [
          "Total planned budget was $345,000 across 5 core departments.",
          "Total actual settled expenditures reached $410,600 with net variance of +$65,600.",
          "Operating margins remained positive at 24% despite cloud infrastructure overruns.",
          "Marketing and Customer Support maintained favorable expense discipline.",
        ],
        speaker_notes: "Highlight the overall revenue growth that cushioned the unexpected infrastructure spend.",
      },
      {
        slide_number: 2,
        title: "Infrastructure Cost Driver Breakdown & Root Cause",
        subtitle: "Analysis of GPU Cluster Compute and Multi-Region Network Growth",
        bullets: [
          "Engineering cloud compute exceeded plan by 36.4% ($65,600 overrun).",
          "Un-pruned GPU inference development clusters ran continuously through September.",
          "Redirect bandwidth surged 40% due to major customer product launches.",
          "Automated node termination policies were disabled during cluster migration.",
        ],
        speaker_notes: "Emphasize that 80% of the overrun is concentrated in non-production GPU sandboxes.",
      },
      {
        slide_number: 3,
        title: "Corrective Actions & FY26 Q4 Forecast Re-alignment",
        subtitle: "Immediate Containment Protocols and Revised Cost Targets",
        bullets: [
          "Implemented automated 8-hour TTL termination on all development GPU nodes.",
          "Migrated standby Redis cache instances to reserved annual instances (saving $14k/yr).",
          "Re-negotiated Cloudflare Enterprise bandwidth commitments for 2027.",
          "Targeting FY26 Q4 operating expenses to return within 3% of original budget.",
        ],
        speaker_notes: "Reassure executive committee that corrective actions are already live in production.",
      },
    ];
  });

  const currentSlide = slides[activeSlideIndex] || slides[0];

  const handleUpdateCurrentSlide = (field: keyof PresentationSlide, val: unknown) => {
    const next = [...slides];
    next[activeSlideIndex] = { ...next[activeSlideIndex], [field]: val };
    setSlides(next);

    useCopilotStore.getState().updateArtifactContent(artifactId, (prev: any) => ({
      ...prev,
      slides: next,
    }));
  };

  const handleUpdateBullet = (bulletIndex: number, text: string) => {
    const nextBullets = [...(currentSlide.bullets || [])];
    nextBullets[bulletIndex] = text;
    handleUpdateCurrentSlide("bullets", nextBullets);
  };

  const handleAddBullet = () => {
    const nextBullets = [...(currentSlide.bullets || []), "New strategic action item for executive review."];
    handleUpdateCurrentSlide("bullets", nextBullets);
  };

  const handleDeleteBullet = (bulletIndex: number) => {
    const nextBullets = (currentSlide.bullets || []).filter((_, i) => i !== bulletIndex);
    handleUpdateCurrentSlide("bullets", nextBullets);
  };

  const handleCopyOutline = async () => {
    const outline = slides.map((s, idx) => `Slide ${s.slide_number || idx + 1}: ${s.title}\n${(s.bullets || []).map((b) => `  - ${b}`).join("\n")}`).join("\n\n");
    try {
      await navigator.clipboard.writeText(outline);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-y-auto" data-testid="financial-deck-canvas">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border/40 pb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {`${content.fiscal_quarter || "FY26 Q3"} ${t("copilot.financeCanvasTitle", "— 3-Slide Executive Presentation")}`}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.financeCanvasDesc", "Edit slide titles, strategic bullet points, and speaker notes in 16:9 widescreen.")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => setPresentationMode(!presentationMode)}
            className="h-8 text-xs flex items-center gap-1"
          >
            {presentationMode ? <Edit3 className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
            <span>{presentationMode ? t("copilot.financeEditMode", "Edit Mode") : t("copilot.financePreviewDeck", "Preview Deck")}</span>
          </Button>

          <Button
            size="sm"
            variant="secondary"
            onClick={handleCopyOutline}
            className="h-8 text-xs flex items-center gap-1"
          >
            {copied ? <Check className="h-3.5 w-3.5 text-success-fg" /> : <Copy className="h-3.5 w-3.5" />}
            <span>{copied ? t("copilot.cardCopied", "Copied") : "Copy Outline"}</span>
          </Button>
        </div>
      </div>

      {/* Slide Navigation Strip */}
      <div className="flex gap-2">
        {slides.map((s, idx) => (
          <button
            key={idx}
            onClick={() => setActiveSlideIndex(idx)}
            className={`flex-1 rounded-xl border p-2.5 text-left transition-all ${
              activeSlideIndex === idx
                ? "border-primary bg-primary/10 shadow-xs ring-1 ring-primary"
                : "border-border bg-card hover:bg-surface-hover"
            }`}
          >
            <div className="font-mono text-[10px] font-bold text-primary">{`${t("copilot.financeSlidePrefix", "SLIDE #")}${s.slide_number || idx + 1}`}</div>
            <div className="font-semibold text-xs text-foreground truncate mt-0.5">{s.title}</div>
          </button>
        ))}
      </div>

      {/* 16:9 Widescreen Canvas Slide */}
      <div
        className="w-full aspect-[16/9] max-h-[440px] rounded-2xl border border-border bg-gradient-to-br from-card to-well/80 p-6 flex flex-col justify-between shadow-lg relative overflow-hidden"
        data-testid="presentation-slide-view"
      >
        <div className="space-y-3">
          {presentationMode ? (
            <div>
              <h2 className="text-xl font-bold text-foreground tracking-tight">{currentSlide.title}</h2>
              {currentSlide.subtitle && (
                <p className="text-xs text-muted-foreground mt-1">{currentSlide.subtitle}</p>
              )}
            </div>
          ) : (
            <div className="space-y-1.5">
              <input
                type="text"
                aria-label={t("copilot.financeSlideTitleAria", "Slide Title")}
                value={currentSlide.title}
                onChange={(e) => handleUpdateCurrentSlide("title", e.target.value)}
                className="w-full text-base font-bold text-foreground bg-transparent border-b border-border/50 pb-1 focus:border-primary focus:outline-none"
              />
              <input
                type="text"
                aria-label={t("copilot.financeSlideSubtitleAria", "Slide Subtitle")}
                value={currentSlide.subtitle || ""}
                onChange={(e) => handleUpdateCurrentSlide("subtitle", e.target.value)}
                placeholder={t("copilot.financeSlideSubtitlePlaceholder", "Slide Subtitle")}
                className="w-full text-xs text-muted-foreground bg-transparent focus:outline-none"
              />
            </div>
          )}

          {/* Bullet Points */}
          <div className="space-y-2 pt-2">
            {(currentSlide.bullets || []).map((bullet, idx) => (
              <div key={idx} className="flex items-start gap-2 text-xs text-foreground/90 leading-relaxed">
                <span className="h-1.5 w-1.5 rounded-full bg-primary mt-1.5 shrink-0" />
                {presentationMode ? (
                  <span>{bullet}</span>
                ) : (
                  <div className="flex-1 flex items-center gap-1.5">
                    <input
                      type="text"
                      aria-label={`Slide bullet point ${idx + 1}`}
                      value={bullet}
                      onChange={(e) => handleUpdateBullet(idx, e.target.value)}
                      className="flex-1 rounded bg-background/50 border border-border/40 px-2 py-1 text-xs focus:border-primary focus:outline-none"
                    />
                    <button
                      onClick={() => handleDeleteBullet(idx)}
                      disabled={(currentSlide.bullets || []).length <= 1}
                      className="p-1 text-muted-foreground hover:text-destructive disabled:opacity-30"
                      aria-label={t("copilot.financeRemoveBulletAria", "Remove bullet point")}
                    >
                      <Trash2 className="h-3 w-3" />
                    </button>
                  </div>
                )}
              </div>
            ))}

            {!presentationMode && (
              <button
                onClick={handleAddBullet}
                className="flex items-center gap-1 text-[11px] text-primary hover:underline pt-1"
              >
                <Plus className="h-3 w-3" />
                <span>{t("copilot.financeAddBullet", "Add bullet point")}</span>
              </button>
            )}
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-border/40 pt-2 text-[10px] text-muted-foreground font-mono">
          <span>OCTARQ PRO ENTERPRISE BRIEFING</span>
          <span>{`${t("copilot.financePage", "PAGE #")}${activeSlideIndex + 1} ${t("copilot.financeOf", "OF 3")}`}</span>
        </div>
      </div>

      {/* Speaker Notes */}
      <div className="rounded-xl border border-border bg-well/20 p-3 space-y-1.5 text-xs">
        <label className="text-[11px] font-semibold text-muted-foreground uppercase">
          {t("copilot.financeSpeakerNotes", "Speaker Notes:")}
        </label>
        <textarea
          rows={2}
          aria-label={t("copilot.financeSpeakerNotesAria", "Speaker notes")}
          value={currentSlide.speaker_notes || ""}
          onChange={(e) => handleUpdateCurrentSlide("speaker_notes", e.target.value)}
          placeholder={t("copilot.financeSpeakerNotesPlaceholder", "Add talking points for executive briefing…")}
          className="w-full rounded border border-border bg-background p-2 text-xs text-foreground resize-none focus:border-primary focus:outline-none"
        />
      </div>
    </div>
  );
}
