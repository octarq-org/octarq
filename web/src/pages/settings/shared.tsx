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
// `forbidden` exists because "no data" and "not allowed" are different states
// and the hook could not tell them apart. It fetches only for an instance
// admin, so for everyone else `s` stayed null forever — and the pages read null
// as "still loading" and rendered a spinner that never resolved. The routes are
// registered unconditionally (only the nav group is filtered), so any member
// who typed the URL or hit browser history landed on a page that hung.
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

// InstanceAdminOnly is what those pages render instead. Deliberately a plain
// notice rather than a redirect: silently bouncing someone off a URL they typed
// reads as a broken link.
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

