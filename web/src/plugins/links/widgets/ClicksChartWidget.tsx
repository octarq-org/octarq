import { useTranslation } from "../../../i18n";
import { AreaChart, GlassCard } from "../../../ui";
import { useOverviewData, StatKV } from "../../../api";
import { useIncludeBot } from "../store";
import { BotToggle } from "../components/BotToggle";

export default function ClicksChartWidget() {
  const [includeBot, setIncludeBot] = useIncludeBot();
  const o = useOverviewData(includeBot);
  const { t } = useTranslation();

  if (!o || o.series === undefined) return null;

  const series = (o.series as StatKV[]) ?? [];
  const clicks30d = (o.clicks30d as number) ?? 0;
  const botClicks30d = (o.botClicks30d as number) ?? 0;

  const botLabel = includeBot
    ? t("links.botInclLabel", { count: botClicks30d })
    : t("links.botHiddenLabel", { count: botClicks30d });

  return (
    <GlassCard className="mb-6 p-5">
      <div className="mb-4 flex items-center justify-between gap-2">
        <h3 className="font-display font-semibold text-foreground">{t("links.clicksLast30")}</h3>
        <div className="flex items-center gap-3">
          <span className="text-sm text-foreground/40">
            {t("links.clicksTotal", { count: clicks30d })} · {botLabel}
          </span>
          <BotToggle value={includeBot} onChange={setIncludeBot} />
        </div>
      </div>
      <AreaChart series={series} />
    </GlassCard>
  );
}
