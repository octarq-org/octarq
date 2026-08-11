import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts } from "../../../api";
import { linksApi, Link, LinkStats } from "../api";
import { Empty, Field, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../../../ui";
import { Link2, Copy, Archive, Trash2, QrCode, Download, Eye, ExternalLink, Calendar, Search, Tag, Globe, Settings, Share2, Filter, Target } from "lucide-react";
import { LinkSettings } from "./LinkSettings";
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
    <GlassCard className="p-5 space-y-5">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-foreground/80 uppercase tracking-wider flex items-center gap-1.5">
          <Eye className="h-4 w-4 text-accent-fg" />
          {t("links.clickPerformanceAnalytics")}
        </h3>
        <div className="flex items-center gap-2">
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
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard label={t("links.totalClicks")} value={stats.total} index={0} />
        <StatCard label={t("links.lastDays", { days: stats.days })} value={stats.windowed} index={1} />
        <StatCard label={t("links.trackingWindow")} value={t("links.daysWindowUnit", { days: stats.series?.length || 0 })} index={2} />
      </div>
      <Spark series={stats.series} />
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-6 pt-2 border-t border-foreground/[0.04]">
        <TopList title={t("links.channels")} icon={<Share2 className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.channels} />
        <TopList title={t("links.utmSources")} icon={<Tag className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.utmSources} />
        <TopList title={t("links.utmMediums")} icon={<Filter className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.utmMediums} />
        <TopList title={t("links.utmCampaigns")} icon={<Target className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.utmCampaigns} />
        <TopList title={t("links.countries")} icon={<Globe className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.countries} />
        <TopList title={t("links.regions")} icon={<ExternalLink className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.regions} />
        <TopList title={t("links.devices")} icon={<Eye className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.devices} />
        <TopList title={t("links.browsers")} icon={<Link2 className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.browsers} />
        <TopList title={t("links.referers")} icon={<Tag className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.referers} />
        {stats.variants && (
          <TopList title={t("links.variants")} icon={<Settings className="h-3 w-3 text-foreground/40 mr-1 inline" />} rows={stats.variants} />
        )}
      </div>
    </GlassCard>
  );
}


function Spark({ series }: { series: { key: string; count: number }[] }) {
  const { t } = useTranslation();
  if (!series || !series.length) return <p className="text-xs text-foreground/40 italic">{t("links.noClickData")}</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="rounded-xl bg-well border border-foreground/[0.05] flex h-24 items-end gap-1 p-3">
      {series.map((s) => (
        <div
          key={s.key}
          title={t("links.clicksTooltip", { key: s.key, count: s.count })}
          className="flex-1 rounded-t-md bg-indigo-500/70 hover:bg-indigo-400 transition-all cursor-pointer" /* ui-color-ok */
          style={{ height: `${(s.count / max) * 100}%`, minHeight: 3 }}
        />
      ))}
    </div>
  );
}


function TopList({ title, icon, rows }: { title: string; icon?: React.ReactNode; rows: { key: string; count: number }[] | null }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <div className="text-[11px] font-semibold uppercase tracking-wider text-foreground/50 flex items-center">
        {icon}
        {title}
      </div>
      {!rows || rows.length === 0 ? (
        <p className="text-xs text-foreground/50 italic">{t("links.emptyDash")}</p>
      ) : (
        <div className="space-y-1.5">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between text-xs font-normal">
              <span className="truncate text-foreground/70 mr-2 font-mono" title={r.key || t("links.directUnknown")}>{r.key || t("links.direct")}</span>
              <span className="text-foreground/45 font-semibold font-mono">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
