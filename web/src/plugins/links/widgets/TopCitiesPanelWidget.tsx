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

  return (
    <Panel title={`${t("links.topCities")}${includeBot ? " " + t("links.inclBots") : ""}`}>
      <BarList rows={cities} empty={
        <span>
          {t("links.noGeoData")}{" "}
          <a href="/admin/help" className="text-indigo-500 hover:underline"> /* ui-color-ok */
            {t("links.noGeoDataHelp")}
          </a>
        </span>
      } />
    </Panel>
  );
}
