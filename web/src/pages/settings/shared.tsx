import { useEffect, useState } from "react";
import { api, Settings as SettingsData } from "../../api";
import { Badge, GlassCard } from "../../ui";
import { Lock } from "lucide-react";
import { useTranslation } from "../../i18n";

// ── Settings module pages (split out of the old monolithic General Settings) ──

// useSettingsData loads the shared workspace settings object once.
export function useSettingsData() {
  const [s, setS] = useState<SettingsData | null>(null);
  const reload = () => api.settings().then(setS);
  useEffect(() => { reload(); }, []);
  return { s, reload };
}

// useInstanceSettingsData loads the instance-wide settings, which only an
// instance admin may read.
//
// Differentiates between loading and forbidden (non-instance-admin) states.
export function useInstanceSettingsData() {
  const { s: wS } = useSettingsData();
  const [s, setS] = useState<import("../../api").InstanceSettings | null>(null);
  const reload = () => api.instanceSettings().then(setS);
  useEffect(() => { if (wS?.isInstanceAdmin) reload(); }, [wS?.isInstanceAdmin]);
  // Undecidable until the workspace settings land: wS === null means the answer
  // isn't known yet, which is genuinely still loading.
  const forbidden = wS != null && !wS.isInstanceAdmin;
  return { s, reload, forbidden };
}

// Plain notice rendered for non-instance-admins when visiting instance admin routes.
export function InstanceAdminOnly() {
  const { t } = useTranslation();
  return (
    <GlassCard className="p-8 text-center">
      <Lock className="mx-auto mb-3 h-6 w-6 text-foreground/40" />
      <p className="text-sm font-medium text-foreground/80">{t("settings.instanceAdminOnly")}</p>
      <p className="mt-1 text-xs text-foreground/50">{t("settings.instanceAdminOnlyDesc")}</p>
    </GlassCard>
  );
}

export function SavedBadge({ on }: { on: boolean }) {
  const { t } = useTranslation();
  return on ? <Badge tone="green">{t("settings.saved")}</Badge> : null;
}
