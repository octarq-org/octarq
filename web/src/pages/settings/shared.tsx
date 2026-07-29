import { useEffect, useState } from "react";
import { api, Settings as SettingsData } from "../../api";
import { Badge } from "../../ui";
import { useTranslation } from "../../i18n";

// ── Settings module pages (split out of the old monolithic General Settings) ──

// useSettingsData loads the shared workspace settings object once.
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
  return { s, reload };
}

export function SavedBadge({ on }: { on: boolean }) {
  const { t } = useTranslation();
  return on ? <Badge tone="green">{t("settings.saved")}</Badge> : null;
}

