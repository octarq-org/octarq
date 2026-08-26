import { useTranslation } from "../../i18n";
import { DiffWidget } from "./types";

export function DiffCard({ widget }: { widget: DiffWidget }) {
  const { t } = useTranslation();
  const title = widget.title || t("a2ui.diff");

  return (
    <div
      role="region"
      aria-label={title}
      className="my-2 rounded-lg border border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.02] dark:bg-white/[0.02] p-3 text-sm"
    >
      <div className="font-medium text-foreground mb-2 flex items-center justify-between">
        <span>{title}</span>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-2 mt-1 font-mono text-xs">
        <div className="rounded-md border border-destructive/20 bg-destructive/5 p-2.5 overflow-x-auto">
          <div className="text-[11px] font-semibold text-destructive mb-1.5 flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-full bg-destructive/60" />
            <span>{t("a2ui.before")}</span>
          </div>
          <pre className="text-foreground/80 leading-relaxed whitespace-pre-wrap">{widget.before}</pre>
        </div>

        <div className="rounded-md border border-primary/20 bg-primary/5 p-2.5 overflow-x-auto">
          <div className="text-[11px] font-semibold text-primary mb-1.5 flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-full bg-primary/60" />
            <span>{t("a2ui.after")}</span>
          </div>
          <pre className="text-foreground/90 leading-relaxed whitespace-pre-wrap">{widget.after}</pre>
        </div>
      </div>
    </div>
  );
}
