/**
 * FallbackCard: renders when no specialized card can render.
 * Always renders: tool name, ASD-STE100 reason, and raw args safely.
 * Never throws and never returns blank.
 */
import React from "react";
import { useTranslation } from "../../i18n";
import type { CardActionHandler } from "./registry";

export type FallbackReason =
  | "unregistered"
  | "malformed-args"
  | "no-schema"
  | "invalid-args";

export const REASON_TEXT: Record<FallbackReason, string> = {
  unregistered: "No card is registered for this tool.",
  "malformed-args": "Tool arguments are not valid JSON.",
  "no-schema": "The tool has no schema to validate against.",
  "invalid-args": "Tool arguments failed validation.",
};

export interface FallbackCardProps {
  toolName: string;
  callId?: string;
  reason: FallbackReason;
  detail?: string;
  onAction?: CardActionHandler;
}

export function FallbackCard({
  toolName,
  callId,
  reason,
  detail,
  onAction,
}: FallbackCardProps) {
  void callId;
  void onAction;
  const { t } = useTranslation();

  return (
    <div
      data-testid="fallback-card"
      data-tool={toolName}
      data-reason={reason}
      className="rounded-xl border border-dashed border-border bg-muted p-3 text-xs text-muted-foreground space-y-1"
    >
      <div className="font-mono font-semibold text-foreground">
        <span>{t("copilot.cardToolPrefix", "tool:")} </span>
        {toolName}
      </div>
      <div>{REASON_TEXT[reason]}</div>
      {detail && (
        <div className="font-mono text-[11px] text-muted-foreground break-all">
          <span>{t("copilot.cardDetail", "detail:")} </span>
          {detail}
        </div>
      )}
    </div>
  );
}
