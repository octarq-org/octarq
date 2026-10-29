import { useTranslation } from "../../i18n";
import { DiffWidget } from "./types";

export function DiffCard({ widget }: { widget: DiffWidget }) {
  const { t } = useTranslation();
  const title = widget.title || t("a2ui.diff");

  return (
    <div
      role="region"
      aria-label={title}
      className="my-2 rounded-lg border border-border bg-well p-3 text-sm"
    >
      <div className="font-medium text-foreground mb-2 flex items-center justify-between">
        <span>{title}</span>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-2 mt-1 font-mono text-xs">
        <div className="rounded-md border border-danger-border bg-danger-bg p-2.5 overflow-x-auto">
          <div className="text-[11px] font-semibold text-danger-fg mb-1.5 flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-full bg-danger-solid/60" />
            <span>{t("a2ui.before")}</span>
          </div>
          <pre className="text-foreground leading-relaxed whitespace-pre-wrap">{widget.before}</pre>
        </div>

        <div className="rounded-md border border-primary/20 bg-accent-soft p-2.5 overflow-x-auto">
          <div className="text-[11px] font-semibold text-accent-fg mb-1.5 flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-full bg-primary/60" />
            <span>{t("a2ui.after")}</span>
          </div>
          <pre className="text-foreground leading-relaxed whitespace-pre-wrap">{widget.after}</pre>
        </div>
      </div>
    </div>
  );
}
