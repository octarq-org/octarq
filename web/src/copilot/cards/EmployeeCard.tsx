/**
 * EmployeeCard: AI Crew persona and presence display.
 * Read-only by construction; points to Crew settings.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Bot } from "lucide-react";
import { useTranslation } from "../../i18n";
import { CardShell, type CardState } from "./CardShell";
import type { ToolCardProps } from "./registry";

export interface EmployeeArgs {
  id?: number | string;
  name?: string;
  trigger?: string;
  status?: string;
  soul?: string;
  tools?: string[];
}

export function EmployeeCard({
  args,
  state,
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const e = (args ?? {}) as EmployeeArgs;
  const isOnline = e.status === "online";
  const persona = (e.soul ?? "").trim();
  const tools = e.tools ?? [];

  return (
    <CardShell title={`${t("copilot.cardAiCrewMember", "AI Crew Member")} — ${e.name ?? "Assistant"}`} state={state}>
      <div className="flex items-center justify-between text-xs">
        <div className="flex items-center gap-1.5">
          <Bot className="h-4 w-4 text-accent-fg" />
          <span
            data-testid="employee-status"
            data-status={isOnline ? "online" : "offline"}
            className={isOnline ? "font-semibold text-success-fg" : "text-muted-foreground"}
          >
            <span aria-hidden="true">• </span>
            {isOnline ? t("copilot.cardStatusOnline", "Online") : t("copilot.cardStatusOffline", "Offline")}
          </span>
        </div>
        {e.id && (
          <span className="font-mono text-muted-foreground">
            {t("copilot.cardEmployeeIdPrefix", "ID: #")}{e.id}
          </span>
        )}
      </div>

      {persona && (
        <div className="mt-2 space-y-1">
          <div className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
            {t("copilot.cardEmployeePersona", "Persona & Directives")}
          </div>
          <div
            data-testid="employee-persona"
            className="max-h-28 overflow-y-auto rounded-lg bg-well p-2 font-mono text-[11px] leading-relaxed whitespace-pre-wrap"
          >
            {persona}
          </div>
        </div>
      )}

      {tools.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {tools.map((t) => (
            <span
              key={t}
              className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
            >
              {t}
            </span>
          ))}
        </div>
      )}

      <div className="pt-1 text-[11px] text-muted-foreground">
        {t("copilot.cardEmployeeHint", "Manage employee personas and tool grants in Workspace Settings.")}
      </div>
    </CardShell>
  );
}
