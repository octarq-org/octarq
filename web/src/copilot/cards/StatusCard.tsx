/**
 * StatusCard: Status page and probe monitor status.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Activity } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";

export interface StatusArgs {
  slug?: string;
  pageSlug?: string;
  name?: string;
  target?: string;
}

export function StatusCard({
  args,
  toolName,
  state,
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const a = (args ?? {}) as StatusArgs;
  const slug = (a.slug ?? a.pageSlug ?? "").trim();

  return (
    <CardShell title={`Status — ${toolName.replace("statuspage_", "")}`} state={state}>
      <div className="flex items-start gap-2">
        <Activity className="h-4 w-4 text-primary shrink-0 mt-0.5" />
        <div className="flex-1 space-y-1">
          {slug ? (
            <div data-testid="status-slug" className="font-mono text-xs">
              <span className="text-muted-foreground">
                <span>{t("copilot.cardPublicPagePrefix", "Public Page:")} </span>
              </span>
              <span className="font-semibold text-foreground">/status/{slug}</span>
            </div>
          ) : (
            <div data-testid="status-probe" className="font-mono text-xs text-muted-foreground">
              {t("copilot.cardProbeRequested", "Probe check requested.")}
            </div>
          )}
          <div className="text-[11px] text-muted-foreground">
            {t("copilot.cardStatusHint", "Live health checks and probe telemetry display under Operations.")}
          </div>
        </div>
      </div>
    </CardShell>
  );
}
