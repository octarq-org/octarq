import { useTranslation } from "../../i18n";
import { ChartWidget } from "./types";

export function ChartCard({ widget }: { widget: ChartWidget }) {
  const { t } = useTranslation();
  const title = widget.title || t("a2ui.chart");
  const data = widget.data;
  const labels = data?.labels || [];
  const series = data?.series || [];

  // Compute maximum value for scaling
  const allValues = series.flatMap((s) => s.values || []);
  const maxVal = Math.max(1, ...allValues);

  return (
    <div
      role="region"
      aria-label={title}
      className="my-2 rounded-lg border border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.02] dark:bg-white/[0.02] p-3 text-sm"
    >
      <div className="font-medium text-foreground mb-2 flex items-center justify-between">
        <span>{title}</span>
      </div>

      {labels.length === 0 ? (
        <p className="text-xs text-muted-foreground italic py-2">{t("a2ui.empty")}</p>
      ) : (
        <div className="space-y-3 mt-1">
          {labels.map((label, idx) => (
            <div key={idx} className="space-y-1">
              <div className="flex justify-between text-xs text-muted-foreground font-medium">
                <span className="truncate max-w-[60%]">{label}</span>
                <span className="font-mono text-[11px]">
                  {series.map((s) => s.values?.[idx] ?? 0).join(" / ")}
                </span>
              </div>
              {series.map((s, sIdx) => {
                const val = s.values?.[idx] ?? 0;
                const pct = Math.max(0, Math.min(100, (val / maxVal) * 100));
                return (
                  <div key={sIdx} className="flex items-center gap-2">
                    {s.name && (
                      <span className="text-[10px] text-muted-foreground/70 w-16 truncate shrink-0">
                        {s.name}
                      </span>
                    )}
                    <div className="flex-1 h-3 rounded-full bg-muted/50 overflow-hidden">
                      <div
                        className="h-full bg-primary rounded-full transition-all duration-300"
                        style={{ width: `${pct}%` }}
                        role="progressbar"
                        aria-valuenow={val}
                        aria-valuemin={0}
                        aria-valuemax={maxVal}
                        aria-label={s.name ? `${s.name}: ${val}` : `${label}: ${val}`}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          ))}

          {series.length > 1 && (
            <div className="flex flex-wrap gap-3 mt-2 pt-2 border-t border-foreground/[0.04] dark:border-white/[0.04] text-[11px] text-muted-foreground">
              {series.map((s, i) => (
                <div key={i} className="flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-primary" />
                  <span>{s.name || `Series ${i + 1}`}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
