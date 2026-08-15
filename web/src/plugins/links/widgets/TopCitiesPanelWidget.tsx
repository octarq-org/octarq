import { useTranslation } from "../../../i18n";
import { BarList, Panel } from "../../../ui";
import { useOverviewData, StatKV } from "../../../api";
import { useIncludeBot } from "../store";

export default function TopCitiesPanelWidget() {
  const [includeBot] = useIncludeBot();
  const o = useOverviewData(includeBot);
  const { t } = useTranslation();

  if (!o || o.cities === undefined) return null;

  const cities = (o.cities as StatKV[]) ?? [];
  // cities only spans the last 30 days; clicks30d is the matching window.
  const hasClicksInWindow = (o.clicks30d ?? 0) > 0;

  return (
    <Panel title={`${t("links.topCities")}${includeBot ? " " + t("links.inclBots") : ""}`}>
      <BarList rows={cities} empty={
        hasClicksInWindow ? (
          <span>
            {t("links.noGeoData")}{" "}
            <a href="/admin/help" className="text-accent-fg hover:underline">
              {t("links.noGeoDataHelp")}
            </a>
          </span>
        ) : (
          t("links.noClickData")
        )
      } />
    </Panel>
  );
}
