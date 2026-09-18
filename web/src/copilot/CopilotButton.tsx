import { cn } from "@octarq/plugin-sdk";
import React from "react";
import { Sparkles } from "lucide-react";
import { useTranslation } from "../i18n";
import { useCopilotStore } from "./store";

interface CopilotButtonProps {
  className?: string;
}

export function CopilotButton({ className }: CopilotButtonProps) {
  const { t } = useTranslation();
  const toggleCopilot = useCopilotStore((s) => s.toggleCopilot);
  const isOpen = useCopilotStore((s) => s.isOpen);
  const pendingApprovals = useCopilotStore((s) => s.pendingApprovals);

  const pendingCount = pendingApprovals.filter((a) => a.status === "pending").length;

  return (
    <button
      type="button"
      onClick={toggleCopilot}
      aria-label={t("copilot.buttonTitle", "AI Copilot (⌘+J)")}
      title={t("copilot.buttonTitle", "AI Copilot (⌘+J)")}
      aria-pressed={isOpen}
      className={cn(
        "group relative flex h-9 items-center gap-1.5 rounded-xl border border-foreground/10 dark:border-white/10 px-2.5 text-xs font-semibold transition-all",
        isOpen
          ? "bg-primary/10 text-primary border-primary/30 ring-1 ring-primary/20"
          : "bg-surface-hover/50 text-muted-foreground hover:bg-surface-hover hover:text-foreground",
        className,
      )}
      data-testid="copilot-trigger-btn"
    >
      <div className="relative">
        <Sparkles className="h-4 w-4 text-primary group-hover:scale-110 transition-transform" />
        {pendingCount > 0 && (
          <span className="absolute -top-1.5 -right-1.5 flex h-2.5 w-2.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-danger-fg opacity-75" />
            <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-danger-fg" />
          </span>
        )}
      </div>

      <span className="hidden sm:inline font-medium group-hover:text-foreground">
        {t("copilot.triggerText", "Copilot")}
      </span>

      {pendingCount > 0 ? (
        <span className="hidden md:inline-flex items-center rounded-full bg-danger-fg/15 px-1.5 py-0.2 text-[10px] font-bold text-danger-fg">
          {pendingCount}
        </span>
      ) : (
        <kbd className="hidden rounded-md border border-foreground/10 dark:border-white/10 bg-muted/80 px-1 py-0.5 text-[9px] font-mono font-medium text-muted-foreground md:block">
          {t("copilot.shortcutKey", "⌘J")}
        </kbd>
      )}
    </button>
  );
}
