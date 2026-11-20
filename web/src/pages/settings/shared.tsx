import { Badge, PageCard } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { api, Settings as SettingsData } from "../../api";
import { Lock } from "lucide-react";
import { useTranslation } from "../../i18n";


export function useSettingsData() {
  const [s, setS] = useState<SettingsData | null>(null);
  const reload = () => api.settings().then(setS);
  useEffect(() => { reload(); }, []);
  return { s, reload };
}

export function useInstanceSettingsData() {
  const { s: wS } = useSettingsData();
  const [s, setS] = useState<import("../../api").InstanceSettings | null>(null);
  const reload = () => api.instanceSettings().then(setS);
  useEffect(() => { if (wS?.isInstanceAdmin) reload(); }, [wS?.isInstanceAdmin]);
  const forbidden = wS != null && !wS.isInstanceAdmin;
  return { s, reload, forbidden };
}

export function InstanceAdminOnly() {
  const { t } = useTranslation();
  return (
    <PageCard className="mt-0 p-8 text-center">
      <Lock className="mx-auto mb-3 h-6 w-6 text-muted-foreground" />
      <p className="text-sm font-medium text-foreground">{t("settings.instanceAdminOnly")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("settings.instanceAdminOnlyDesc")}</p>
    </PageCard>
  );
}

export function SavedBadge({ on }: { on: boolean }) {
  const { t } = useTranslation();
  return on ? <Badge tone="green">{t("settings.saved")}</Badge> : null;
}
