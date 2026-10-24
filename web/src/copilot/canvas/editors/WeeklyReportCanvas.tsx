/**
 * WeeklyReportCanvas: Dual-pane split Markdown editor and live preview for weekly executive briefings.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { Copy, Check, FileDown, Eye, Edit3 } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { WeeklyReportKPIs } from "../../cards/WeeklyReportCard";
import type { CardActionHandler } from "../../cards/registry";
import { useCopilotStore } from "../../store";

export interface WeeklyReportCanvasProps {
  artifactId: string;
  content: {
    report_title?: string;
    date_range?: string;
    summary_kpis?: WeeklyReportKPIs;
    markdown_report?: string;
    sections?: Array<{ key?: string; title: string; content_markdown?: string }>;
  };
  onAction: CardActionHandler;
}

export function WeeklyReportCanvas({ artifactId, content, onAction }: WeeklyReportCanvasProps) {
  const { t } = useTranslation();
  const defaultText =
    content.markdown_report ||
    `# ${content.report_title || "Executive Operations Weekly Report"}
*Period: ${content.date_range || "Oct 18 – Oct 24, 2026"}*

## Executive Summary
This week link redirect traffic grew by 18.4% across 450 active hostnames. Mail deliverability maintained 99.8% with zero reputation incidents.

## Key Performance Indicators
- **Redirect Traffic**: 25,400,000 requests processed
- **Mail Deliverability**: 99.8%
- **Invoice Reconciliation**: 94% matched cleanly
- **Open Critical Incidents**: 0

## Department Operational Health
### 1. Infrastructure & Networking
All 12 edge nodes operated at 99.99% availability. Latency averaged 18ms across North America and Europe.

### 2. Billing & Finance
Batch reconciliation processed 54 vendor invoices. Six overdue invoices flagged for audit resolution.

## Priorities for Next Week
1. Complete Go 1.27.3 CVE remediation across production clusters.
2. Deliver enterprise price quotation to Acme Cloud Logistics.
`;

  const [markdown, setMarkdown] = useState<string>(defaultText);
  const [copied, setCopied] = useState(false);
  const [viewMode, setViewMode] = useState<"split" | "edit" | "preview">("split");

  const handleMarkdownChange = (text: string) => {
    setMarkdown(text);
    useCopilotStore.getState().updateArtifactContent(artifactId, (prev: any) => ({
      ...prev,
      markdown_report: text,
    }));
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(markdown);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-hidden" data-testid="weekly-report-canvas">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border/40 pb-3 shrink-0">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {content.report_title || "Executive Operations Weekly Report"}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.weeklyCanvasDesc", "Edit briefing markdown and inspect live rendered document side by side.")}
          </p>
        </div>
        <div className="flex items-center gap-1.5">
          <div className="flex rounded-lg bg-well p-0.5 text-xs">
            <button
              onClick={() => setViewMode("edit")}
              className={`rounded px-2.5 py-1 transition-colors ${viewMode === "edit" ? "bg-background text-foreground shadow-xs font-semibold" : "text-muted-foreground"}`}
            >
              {t("copilot.weeklyTabEdit", "Edit")}
            </button>
            <button
              onClick={() => setViewMode("split")}
              className={`rounded px-2.5 py-1 transition-colors ${viewMode === "split" ? "bg-background text-foreground shadow-xs font-semibold" : "text-muted-foreground"}`}
            >
              {t("copilot.weeklyTabSplit", "Split")}
            </button>
            <button
              onClick={() => setViewMode("preview")}
              className={`rounded px-2.5 py-1 transition-colors ${viewMode === "preview" ? "bg-background text-foreground shadow-xs font-semibold" : "text-muted-foreground"}`}
            >
              {t("copilot.weeklyTabPreview", "Preview")}
            </button>
          </div>

          <Button
            size="sm"
            variant="secondary"
            onClick={handleCopy}
            className="h-7 text-xs flex items-center gap-1"
          >
            {copied ? <Check className="h-3 w-3 text-success-fg" /> : <Copy className="h-3 w-3" />}
            <span>{copied ? t("copilot.cardCopied", "Copied") : t("copilot.cardCopy", "Copy")}</span>
          </Button>
        </div>
      </div>

      {/* Editor & Preview Panes */}
      <div className="flex-1 flex gap-3 overflow-hidden">
        {(viewMode === "split" || viewMode === "edit") && (
          <div className="flex-1 flex flex-col rounded-xl border border-border bg-card overflow-hidden">
            <div className="border-b border-border bg-well/30 px-3 py-1.5 text-[11px] font-semibold text-muted-foreground flex items-center gap-1">
              <Edit3 className="h-3 w-3" />
              <span>{t("copilot.weeklyMarkdownSource", "Markdown Source")}</span>
            </div>
            <textarea
              value={markdown}
              onChange={(e) => handleMarkdownChange(e.target.value)}
              aria-label={t("copilot.weeklyMarkdownAria", "Markdown editor")}
              className="flex-1 w-full p-3 font-mono text-xs leading-relaxed text-foreground bg-transparent resize-none focus:outline-none"
            />
          </div>
        )}

        {(viewMode === "split" || viewMode === "preview") && (
          <div className="flex-1 flex flex-col rounded-xl border border-border bg-card overflow-hidden">
            <div className="border-b border-border bg-well/30 px-3 py-1.5 text-[11px] font-semibold text-muted-foreground flex items-center gap-1">
              <Eye className="h-3 w-3" />
              <span>{t("copilot.weeklyLiveDocument", "Live Rendered Document")}</span>
            </div>
            <div
              className="flex-1 p-4 overflow-y-auto prose prose-invert prose-xs max-w-none text-xs leading-relaxed space-y-2 text-foreground/90"
              data-testid="markdown-preview"
            >
              {markdown.split("\n\n").map((block, idx) => {
                if (block.startsWith("# ")) {
                  return <h1 key={idx} className="text-base font-bold text-foreground">{block.replace("# ", "")}</h1>;
                }
                if (block.startsWith("## ")) {
                  return <h2 key={idx} className="text-sm font-semibold text-foreground pt-1">{block.replace("## ", "")}</h2>;
                }
                if (block.startsWith("### ")) {
                  return <h3 key={idx} className="text-xs font-semibold text-foreground">{block.replace("### ", "")}</h3>;
                }
                if (block.startsWith("- ")) {
                  return (
                    <ul key={idx} className="list-disc list-inside space-y-0.5 text-muted-foreground">
                      {block.split("\n").map((line, i) => (
                        <li key={i}>{line.replace(/^- /, "")}</li>
                      ))}
                    </ul>
                  );
                }
                return <p key={idx} className="text-muted-foreground leading-relaxed">{block}</p>;
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
