import { useEffect, useState } from "react";
import { linksApi, Link, LinkStats } from "../api";
import { StatCard } from "../../../ui";
import { Link2, Eye, ExternalLink, Tag, Globe, Settings, Share2, Filter, Target } from "lucide-react";
import { useTranslation } from "../../../i18n";

export function StatsView({ link }: { link: Link }) {
  const { t } = useTranslation();
  const [metric, setMetric] = useState<"uv" | "pv">("uv");
  const [stats, setStats] = useState<LinkStats | null>(null);

  useEffect(() => {
    setStats(null);
    linksApi.linkStats(link.id, 30, metric).then(setStats);
  }, [link.id, metric]);
  
  if (!stats) return <div className="text-foreground/40 p-4 text-xs">{t("links.loadingAnalytics")}</div>;
  
  return (
    <div className="space-y-5 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-3 min-w-0">
        <h3 className="text-sm font-semibold text-foreground/80 uppercase tracking-wider flex items-center gap-1.5 min-w-0">
          <Eye className="h-4 w-4 text-accent-fg shrink-0" />
          <span className="truncate">{t("links.clickPerformanceAnalytics")}</span>
        </h3>
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs text-foreground/50 font-medium">
            {t("links.metricMode", { mode: metric === "uv" ? t("links.uvLabel") : t("links.pvLabel") })}
          </span>
          <div className="inline-flex rounded-lg bg-well p-0.5 border border-foreground/[0.08]">
            <button
              type="button"
              onClick={() => setMetric("uv")}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
                metric === "uv"
                  ? "bg-accent text-accent-fg shadow-sm"
                  : "text-foreground/60 hover:text-foreground"
              }`}
            >
              {t("links.uvShort")}
            </button>
            <button
              type="button"
              onClick={() => setMetric("pv")}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
                metric === "pv"
                  ? "bg-accent text-accent-fg shadow-sm"
                  : "text-foreground/60 hover:text-foreground"
              }`}
            >
              {t("links.pvShort")}
            </button>
          </div>
        </div>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 min-w-0">
        <StatCard label={t("links.totalClicks")} value={stats.total} index={0} />
        <StatCard label={t("links.lastDays", { days: stats.days })} value={stats.windowed} index={1} />
        <StatCard label={t("links.trackingWindow")} value={t("links.daysWindowUnit", { days: stats.series?.length || 0 })} index={2} />
      </div>
      <Spark series={stats.series} />
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-6 pt-2 border-t border-foreground/[0.04] min-w-0">
        <TopList title={t("links.channels")} icon={<Share2 className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.channels} />
        <TopList title={t("links.utmSources")} icon={<Tag className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.utmSources} />
        <TopList title={t("links.utmMediums")} icon={<Filter className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.utmMediums} />
        <TopList title={t("links.utmCampaigns")} icon={<Target className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.utmCampaigns} />
        <TopList title={t("links.countries")} icon={<Globe className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.countries} />
        <TopList title={t("links.regions")} icon={<ExternalLink className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.regions} />
        <TopList title={t("links.devices")} icon={<Eye className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.devices} />
        <TopList title={t("links.browsers")} icon={<Link2 className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.browsers} />
        <TopList title={t("links.referers")} icon={<Tag className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.referers} />
        {stats.variants && (
          <TopList title={t("links.variants")} icon={<Settings className="h-3 w-3 text-foreground/40 mr-1 inline shrink-0" />} rows={stats.variants} />
        )}
      </div>
    </div>
  );
}

function Spark({ series }: { series: { key: string; count: number }[] }) {
  const { t } = useTranslation();
  if (!series || !series.length) return <p className="text-xs text-foreground/40 italic">{t("links.noClickData")}</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="rounded-xl bg-well border border-foreground/[0.05] flex h-24 items-end gap-1 p-3 min-w-0 overflow-hidden">
      {series.map((s) => (
        <div
          key={s.key}
          title={t("links.clicksTooltip", { key: s.key, count: s.count })}
          className="flex-1 min-w-[2px] rounded-t-md bg-primary/70 hover:bg-primary transition-all cursor-pointer"
          style={{ height: `${(s.count / max) * 100}%`, minHeight: 3 }}
        />
      ))}
    </div>
  );
}

function TopList({ title, icon, rows }: { title: string; icon?: React.ReactNode; rows: { key: string; count: number }[] | null }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 min-w-0">
      <div className="text-[11px] font-semibold uppercase tracking-wider text-foreground/50 flex items-center min-w-0">
        {icon}
        <span className="truncate">{title}</span>
      </div>
      {!rows || rows.length === 0 ? (
        <p className="text-xs text-foreground/50 italic">{t("links.emptyDash")}</p>
      ) : (
        <div className="space-y-1.5 min-w-0">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between items-center text-xs font-normal min-w-0 gap-2">
              <span className="truncate text-foreground/70 font-mono min-w-0 flex-1" title={r.key || t("links.directUnknown")}>{r.key || t("links.direct")}</span>
              <span className="font-mono tnum text-foreground/45 font-semibold shrink-0">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
