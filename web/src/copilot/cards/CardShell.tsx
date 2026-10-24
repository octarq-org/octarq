/**
 * CardShell: the four non-success states every generative card
 * must render — loading, error, empty, forbidden (entitlement) — plus the
 * normal body.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { type ReactNode } from "react";
import { cn } from "@octarq/plugin-sdk";
import { AlertCircle, Lock, Loader2, Inbox } from "lucide-react";
import { useTranslation } from "../../i18n";

export type CardState = "loading" | "error" | "empty" | "forbidden";

export const CARD_STATE_TEXT: Record<CardState, string> = {
  loading: "Loading data…",
  error: "An error occurred. The system made no changes.",
  empty: "No items to display.",
  forbidden: "This capability requires an upgraded license tier.",
};

export interface CardShellProps {
  title: string;
  /** Normal state is undefined; the four failure/empty states are explicit. */
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
  className?: string;
  children?: ReactNode;
}

export function CardShell({
  title,
  state,
  errorDetail,
  forbiddenReason,
  className,
  children,
}: CardShellProps) {
  const { t } = useTranslation();

  const stateText: Record<CardState, string> = {
    loading: t("copilot.cardStateLoading", CARD_STATE_TEXT.loading),
    error: t("copilot.cardStateError", CARD_STATE_TEXT.error),
    empty: t("copilot.cardStateEmpty", CARD_STATE_TEXT.empty),
    forbidden: t("copilot.cardStateForbidden", CARD_STATE_TEXT.forbidden),
  };

  if (state) {
    return (
      <div
        data-testid="card-shell"
        data-state={state}
        className={cn(
          "rounded-xl border border-border bg-card p-3 text-xs leading-relaxed shadow-xs space-y-1.5",
          state === "forbidden" && "border-warning-fg/30 bg-warning-fg/10",
          state === "error" && "border-destructive/30 bg-destructive/5",
          className,
        )}
      >
        <div className="flex items-center gap-1.5 font-semibold text-foreground">
          {state === "loading" && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
          {state === "error" && <AlertCircle className="h-3.5 w-3.5 text-destructive" />}
          {state === "empty" && <Inbox className="h-3.5 w-3.5 text-muted-foreground" />}
          {state === "forbidden" && <Lock className="h-3.5 w-3.5 text-warning-fg" />}
          <span>{title}</span>
        </div>
        <div
          className={cn(
            "text-xs text-muted-foreground",
            state === "forbidden" && "text-warning-fg font-medium",
            state === "error" && "text-destructive",
          )}
          role={state === "error" ? "alert" : undefined}
        >
          {state === "forbidden" && forbiddenReason
            ? forbiddenReason
            : stateText[state]}
        </div>
        {state === "error" && errorDetail && (
          <div className="font-mono text-[11px] text-muted-foreground break-all">
            <span>{t("copilot.cardDetail", "detail:")} </span>
            {errorDetail}
          </div>
        )}
      </div>
    );
  }

  return (
    <div
      data-testid="card-shell"
      data-state="ok"
      className={cn(
        "rounded-xl border border-border bg-card p-3 text-xs leading-relaxed shadow-xs space-y-2",
        className,
      )}
    >
      <div className="flex items-center justify-between border-b border-border/40 pb-1.5 font-semibold text-foreground">
        <span>{title}</span>
      </div>
      {children}
    </div>
  );
}
