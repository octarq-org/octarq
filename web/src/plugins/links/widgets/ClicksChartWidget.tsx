import { Panel } from "@octarq/plugin-sdk";
import { AreaChart } from "../../../app-ui";
import { useTranslation } from "../../../i18n";
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
    <Panel
      className="mb-6"
      title={t("links.clicksLast30")}
      action={
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground">
            {t("links.clicksTotal", { count: clicks30d })} · {botLabel}
          </span>
          <BotToggle value={includeBot} onChange={setIncludeBot} />
        </div>
      }
    >
      <AreaChart series={series} />
    </Panel>
  );
}
