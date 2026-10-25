import { useTranslation } from "../../i18n";
import { A2UIWidget, ChartWidget, DiffWidget, ApprovalWidget } from "./types";
import { ChartCard } from "./ChartCard";
import { DiffCard } from "./DiffCard";
import { ApprovalCard } from "./ApprovalCard";

/**
 * Invariant #11: P3 pre-API volatility — A2UI kind and payload fields are internal contracts (VOLATILE).
 * Unknown kinds gracefully degrade to a formatted JSON preview to prevent render crashes.
 */
export function A2UIRenderer({ widget }: { widget: A2UIWidget }) {
  const { t } = useTranslation();

  if (!widget || typeof widget !== "object") {
    return null;
  }

  switch (widget.kind) {
    case "chart":
      return <ChartCard widget={widget as ChartWidget} />;
    case "diff":
      return <DiffCard widget={widget as DiffWidget} />;
    case "approval":
      return <ApprovalCard widget={widget as ApprovalWidget} />;
    default:
      return (
        <div
          role="region"
          aria-label={t("a2ui.unsupported")}
          className="my-2 rounded-lg border border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.02] dark:bg-white/[0.02] p-3 text-xs"
        >
          <div className="font-medium text-muted-foreground mb-1">
            {t("a2ui.unsupported")}: <span className="font-mono">{String(widget.kind)}</span>
          </div>
          <pre className="font-mono text-[11px] p-2 rounded bg-muted/40 text-muted-foreground overflow-x-auto max-h-40">
            {JSON.stringify(widget, null, 2)}
          </pre>
        </div>
      );
  }
}
