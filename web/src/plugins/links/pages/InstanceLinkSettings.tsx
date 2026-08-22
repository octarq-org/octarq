// Instance console → Short Link Settings — the deployment-wide reserved-slug
// list. A config that exists once per deployment is instance-scoped
// (GET/PUT /api/instance-settings), so this page lives in the /instance
// console, announced by the Go half's InstanceMenus() and registered here via
// the plugin's instanceRoutes. The console shell is already instance-admin
// only (console.tsx's isAdmin gate) and the server re-checks every request, so
// the workspace-settings frontend gate the old tenant page carried has no
// place here.
import { useEffect, useState } from "react";
import { api } from "../../../api";
import { Field, Button, PageHeader, GlassCard, toast } from "../../../ui";
import { useTranslation } from "../../../i18n";
import { SavedBadge } from "../../../pages/settings/shared";

export function InstanceLinkSettings() {
  const { t } = useTranslation();
  const [reservedSlugs, setReservedSlugs] = useState("");
  const [builtinReserved, setBuiltinReserved] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    api
      .instanceSettings()
      .then((s) => {
        setReservedSlugs(s.reservedSlugs);
        setBuiltinReserved(s.builtinReserved);
      })
      .catch(() => {
        /* instance admin is the only caller; the server stays authoritative */
      })
      .finally(() => setLoading(false));
  }, []);

  async function save() {
    setBusy(true);
    try {
      await api.updateInstanceSettings({ reservedSlugs });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (err: any) {
      toast.error(err?.message || t("settings.saveFailed", "Failed to save settings"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.shortLinksSettings")} />

      {loading ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/50">{t("settings.loadingLower")}</GlassCard>
      ) : (
        <GlassCard className="p-6 space-y-6">
          <div className="flex justify-end">
            <SavedBadge on={saved} />
          </div>
          <Field label={t("settings.reservedSlugsLabel")} hint={t("settings.reservedSlugsHint", { list: builtinReserved.join(", ") })}>
            <textarea
              className="input w-full font-mono text-xs"
              rows={3}
              value={reservedSlugs}
              onChange={(e) => setReservedSlugs(e.target.value)}
              placeholder="pricing&#10;login&#10;about"
            />
          </Field>
          <div className="border-t border-foreground/[0.06] pt-4 flex justify-end">
            <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>
              {busy ? t("settings.saving") : t("settings.saveSettings")}
            </Button>
          </div>
        </GlassCard>
      )}
    </div>
  );
}