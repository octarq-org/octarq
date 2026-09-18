import { Panel } from "@octarq/plugin-sdk";
import { BarList } from "../../../app-ui";
import { useTranslation } from "../../../i18n";
import { useOverviewData, StatKV } from "../../../api";
import { useIncludeBot } from "../store";

export default function DevicesPanelWidget() {
  const [includeBot] = useIncludeBot();
  const o = useOverviewData(includeBot);
  const { t } = useTranslation();

  if (!o || o.devices === undefined) return null;

  const devices = (o.devices as StatKV[]) ?? [];

  return (
    <Panel title={`${t("links.devices")}${includeBot ? " " + t("links.inclBots") : ""}`}>
      <BarList rows={devices} />
    </Panel>
  );
}
