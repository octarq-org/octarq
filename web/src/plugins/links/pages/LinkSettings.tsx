import { useEffect, useState } from "react";
import { api } from "../../../api";
import { Field, Button } from "../../../ui";
import { useTranslation } from "../../../i18n";
import { useSettingsData, useInstanceSettingsData, SavedBadge } from "../../../pages/settings/shared";

export function LinkSettings() {
  const { t } = useTranslation();
  const { s: wS } = useSettingsData();
  const { s } = useInstanceSettingsData();
  const [reservedSlugs, setReservedSlugs] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => { if (s) { setReservedSlugs(s.reservedSlugs); } }, [s]);

  if (!wS?.isInstanceAdmin) return null;

  async function save() {
    setBusy(true);
    try { await api.updateInstanceSettings({ reservedSlugs }); setSaved(true); setTimeout(() => setSaved(false), 2000); }
    finally { setBusy(false); }
  }
  if (!s) return <div className="text-sm text-white/40">{t("settings.loadingLower")}</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white/90">{t("settings.shortLinksSettings")}</h2>
        <SavedBadge on={saved} />
      </div>
      <Field label={t("settings.reservedSlugsLabel")} hint={t("settings.reservedSlugsHint", { list: s.builtinReserved.join(", ") })}>
        <textarea className="input w-full font-mono text-xs" rows={3} value={reservedSlugs} onChange={(e) => setReservedSlugs(e.target.value)} placeholder="pricing&#10;login&#10;about" />
      </Field>
      <div className="border-t border-white/[0.06] pt-4 flex justify-end">
        <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>{busy ? t("settings.saving") : t("settings.saveSettings")}</Button>
      </div>
    </div>
  );
}
