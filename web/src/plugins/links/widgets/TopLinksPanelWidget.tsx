import { useNavigate } from "react-router-dom";
import { useTranslation } from "../../../i18n";
import { Panel } from "../../../ui";
import { useOverviewData } from "../../../api";

export default function TopLinksPanelWidget() {
  const o = useOverviewData();
  const nav = useNavigate();
  const { t } = useTranslation();

  if (!o || o.topLinks === undefined) return null;

  const topLinks = o.topLinks as { id: number; slug: string; host: string; title: string; tags: string; clicks: number }[] | null;

  return (
    <Panel title={t("links.topLinks")}>
      {!topLinks || topLinks.length === 0 ? (
        <p className="text-sm text-foreground/50">{t("links.noLinks")}</p>
      ) : (
        <div className="space-y-1">
          {topLinks.map((l) => {
            const rawTags = (l.tags || "").split(",").map((s) => s.trim()).filter(Boolean);
            const visibleTags = rawTags.slice(0, 2);
            const remainingCount = rawTags.length - visibleTags.length;
            const displayTitle = l.title || l.slug;
            const secondarySlug = l.host ? `${l.host}/${l.slug}` : `/${l.slug}`;

            return (
              <button
                key={l.id}
                onClick={() => nav("/links")}
                className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm hover:bg-foreground/[0.06] transition-colors"
              >
                <div className="min-w-0 flex-1 pr-2">
                  <div className="flex items-center gap-1.5 min-w-0">
                    <span className="truncate text-foreground font-medium">{displayTitle}</span>
                    {visibleTags.map((tag) => (
                      <span
                        key={tag}
                        title={tag}
                        aria-label={tag}
                        className="inline-flex items-center text-[10px] px-1.5 py-0.5 rounded bg-foreground/[0.06] text-foreground/70 font-mono shrink-0"
                      >
                        {tag}
                      </span>
                    ))}
                    {remainingCount > 0 && (
                      <span
                        title={rawTags.slice(2).join(", ")}
                        aria-label={`+${remainingCount}`}
                        className="inline-flex items-center text-[10px] px-1 py-0.5 rounded bg-foreground/[0.06] text-foreground/50 font-mono shrink-0"
                      >
                        +{remainingCount}
                      </span>
                    )}
                  </div>
                  <div className="font-mono text-xs text-foreground/40 truncate">
                    {secondarySlug}
                  </div>
                </div>
                <span className="shrink-0 font-semibold">{l.clicks}</span>
              </button>
            );
          })}
        </div>
      )}
    </Panel>
  );
}
