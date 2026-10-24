/**
 * ArtifactCanvas: Main container for side-canvas artifact editors.
 *
 * Hosts layout mode toggles (split vs maximized), version control badges,
 * and dynamically mounts the specialized domain editors.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Maximize2, Minimize2, X, Sparkles } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useCopilotStore } from "../store";
import type { CardActionHandler } from "../cards/registry";
import { ResumeRankCanvas } from "./editors/ResumeRankCanvas";
import { InvoiceAuditCanvas } from "./editors/InvoiceAuditCanvas";
import { QuotationCanvas } from "./editors/QuotationCanvas";
import { WeeklyReportCanvas } from "./editors/WeeklyReportCanvas";
import { StoryboardCanvas } from "./editors/StoryboardCanvas";
import { EmailDraftCanvas } from "./editors/EmailDraftCanvas";
import { SupportEscalationCanvas } from "./editors/SupportEscalationCanvas";
import { FinancialDeckCanvas } from "./editors/FinancialDeckCanvas";

export interface ArtifactCanvasProps {
  onAction?: CardActionHandler;
}

export function ArtifactCanvas({ onAction = () => {} }: ArtifactCanvasProps) {
  const { t } = useTranslation();
  const { activeArtifactId, artifacts, canvasLayout, setCanvasLayout, closeCanvas } =
    useCopilotStore();

  if (!activeArtifactId || !artifacts[activeArtifactId]) {
    return (
      <div
        data-testid="artifact-canvas-empty"
        className="flex h-full flex-col items-center justify-center p-6 text-center text-xs text-muted-foreground"
      >
        <Sparkles className="h-6 w-6 text-muted-foreground/60 mb-2" />
        <p className="font-semibold text-foreground">{t("copilot.canvasEmptyTitle", "No active artifact selected.")}</p>
        <p className="mt-1">{t("copilot.canvasEmptyDesc", "Open a visual action card from the chat stream to edit here.")}</p>
      </div>
    );
  }

  const artifact = artifacts[activeArtifactId];

  const renderEditor = () => {
    switch (artifact.type) {
      case "resume_rank":
        return (
          <ResumeRankCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "invoice_audit":
        return (
          <InvoiceAuditCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "quotation":
        return (
          <QuotationCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "weekly_report":
        return (
          <WeeklyReportCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "storyboard":
        return (
          <StoryboardCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "email_draft":
        return (
          <EmailDraftCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "support_escalation":
        return (
          <SupportEscalationCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      case "financial_deck":
        return (
          <FinancialDeckCanvas
            artifactId={artifact.id}
            content={artifact.content as any}
            onAction={onAction}
          />
        );
      default:
        return (
          <div className="p-4 text-xs text-muted-foreground">
            {t("copilot.canvasUnknownType", "Unknown artifact type:")} {artifact.type}
          </div>
        );
    }
  };

  return (
    <div
      data-testid="artifact-canvas"
      className="flex h-full w-full flex-col bg-background overflow-hidden"
    >
      {/* Canvas Top Bar */}
      <div className="flex items-center justify-between border-b border-border bg-well/30 px-4 py-2.5 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <span className="truncate text-xs font-bold text-foreground" data-testid="artifact-title">
            {artifact.title}
          </span>
          <span className="rounded-full bg-well px-2 py-0.5 font-mono text-[10px] font-medium text-muted-foreground shrink-0">
            {`v${artifact.version || 1}`}
          </span>
          <span className="rounded-full bg-primary/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-primary capitalize shrink-0">
            {artifact.status || "draft"}
          </span>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {canvasLayout === "split" ? (
            <button
              type="button"
              onClick={() => setCanvasLayout("maximized")}
              aria-label={t("copilot.canvasMaximize", "Maximize Canvas")}
              title={t("copilot.canvasMaximize", "Maximize Canvas")}
              data-testid="maximize-canvas-btn"
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
            >
              <Maximize2 className="h-4 w-4" />
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setCanvasLayout("split")}
              aria-label={t("copilot.canvasRestore", "Restore Split View")}
              title={t("copilot.canvasRestore", "Restore Split View")}
              data-testid="restore-split-btn"
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
            >
              <Minimize2 className="h-4 w-4" />
            </button>
          )}

          <button
            type="button"
            onClick={closeCanvas}
            aria-label={t("copilot.canvasClose", "Close Canvas")}
            title={t("copilot.canvasClose", "Close Canvas")}
            data-testid="close-canvas-btn"
            className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Editor Body */}
      <div className="flex-1 overflow-hidden" data-testid="canvas-editor-body">
        {renderEditor()}
      </div>
    </div>
  );
}
