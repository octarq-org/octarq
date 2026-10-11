import { useState } from "react";
import { ChevronDown, Sparkles } from "lucide-react";
import { useTranslation } from "../../i18n";

export interface ThinkingCollapsibleProps {
  thinking?: string;
  tokens?: number;
  defaultOpen?: boolean;
}

export function ThinkingCollapsible({
  thinking,
  tokens,
  defaultOpen = false,
}: ThinkingCollapsibleProps) {
  const [open, setOpen] = useState(defaultOpen);
  const { t } = useTranslation();

  if (!thinking) return null;

  return (
    <div
      role="region"
      aria-label={t("command.chat.thinking")}
      className="my-2 rounded-lg border border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.02] dark:bg-white/[0.02] overflow-hidden text-xs"
    >
      <button
        type="button"
        role="button"
        aria-expanded={open}
        onClick={() => setOpen((prev) => !prev)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setOpen((prev) => !prev);
          }
        }}
        className="flex w-full items-center justify-between px-3 py-1.5 text-left text-muted-foreground hover:bg-surface-hover/50 transition-colors font-medium cursor-pointer select-none"
      >
        <span className="flex items-center gap-1.5">
          <Sparkles className="h-3.5 w-3.5 text-primary shrink-0" />
          <span>{t("command.chat.thinking")}</span>
          {tokens !== undefined && (
            <span className="text-[10px] text-muted-foreground/70 font-mono">
              ({t("command.chat.tokens", { count: tokens })})
            </span>
          )}
        </span>
        <ChevronDown
          className={`h-3.5 w-3.5 shrink-0 transition-transform duration-200 ${open ? "rotate-180" : ""}`}
        />
      </button>
      {open && (
        <div className="border-t border-foreground/[0.06] dark:border-white/[0.06] p-2.5 font-mono text-[11px] leading-relaxed text-muted-foreground whitespace-pre-wrap max-h-48 overflow-y-auto">
          {thinking}
        </div>
      )}
    </div>
  );
}
