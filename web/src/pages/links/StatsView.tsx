import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts, Link, LinkStats } from "../../api";
import { Empty, Field, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../../ui";
import { Link2, Copy, Archive, Trash2, QrCode, Download, Eye, ExternalLink, Calendar, Search, Tag, Globe, Settings } from "lucide-react";
import { LinkSettings } from "../Settings";
import { useTranslation } from "../../i18n";

export function StatsView({ link }: { link: Link }) {
  const { t } = useTranslation();
  const [stats, setStats] = useState<LinkStats | null>(null);
  useEffect(() => {
    setStats(null);
    api.linkStats(link.id).then(setStats);
  }, [link.id]);
  
  if (!stats) return <div className="text-white/40 p-4 text-xs">{t("links.loadingAnalytics")}</div>;
  
  return (
    <GlassCard className="p-5 space-y-5">
      <h3 className="text-sm font-semibold text-white/80 uppercase tracking-wider flex items-center gap-1.5">
        <Eye className="h-4 w-4 text-indigo-400" />
        {t("links.clickPerformanceAnalytics")}
      </h3>
      <div className="grid grid-cols-3 gap-4">
        <StatCard label={t("links.totalClicks")} value={stats.total} index={0} />
        <StatCard label={t("links.lastDays", { days: stats.days })} value={stats.windowed} index={1} />
        <StatCard label={t("links.trackingWindow")} value={`${stats.series?.length || 0}d`} index={2} />
      </div>
      <Spark series={stats.series} />
      <div className="grid grid-cols-2 md:grid-cols-3 gap-6 pt-2 border-t border-white/[0.04]">
        <TopList title={t("links.countries")} icon={<Globe className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.countries} />
        <TopList title={t("links.regions")} icon={<ExternalLink className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.regions} />
        <TopList title={t("links.devices")} icon={<Eye className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.devices} />
        <TopList title={t("links.browsers")} icon={<Link2 className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.browsers} />
        <TopList title={t("links.referers")} icon={<Tag className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.referers} />
      </div>
    </GlassCard>
  );
}


function Spark({ series }: { series: { key: string; count: number }[] }) {
  const { t } = useTranslation();
  if (!series || !series.length) return <p className="text-xs text-white/40 italic">{t("links.noClickData")}</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="rounded-xl bg-black/30 border border-white/[0.05] flex h-24 items-end gap-1 p-3">
      {series.map((s) => (
        <div
          key={s.key}
          title={t("links.clicksTooltip", { key: s.key, count: s.count })}
          className="flex-1 rounded-t-md bg-indigo-500/70 hover:bg-indigo-400 transition-all cursor-pointer"
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
      <div className="text-[11px] font-semibold uppercase tracking-wider text-white/50 flex items-center">
        {icon}
        {title}
      </div>
      {!rows || rows.length === 0 ? (
        <p className="text-xs text-white/50 italic">—</p>
      ) : (
        <div className="space-y-1.5">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between text-xs font-normal">
              <span className="truncate text-white/70 mr-2 font-mono" title={r.key || t("links.directUnknown")}>{r.key || t("links.direct")}</span>
              <span className="text-white/45 font-semibold font-mono">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
