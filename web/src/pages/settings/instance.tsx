import { useEffect, useState } from "react";
import { api, InstanceSettings as InstanceSettingsData } from "../../api";
import { Field, PageHeader, GlassCard, Button } from "../../ui";
import { Server, Sliders } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useInstanceSettingsData, SavedBadge } from "./shared";
import { ExtensionSlot } from "../../plugin-sdk";

export function InstanceSettings() {
  const { t } = useTranslation();
  const { s: settings, reload } = useInstanceSettingsData();

  const [appName, setAppName] = useState("");
  const [retention, setRetention] = useState(90);
  const [rlAuth, setRlAuth] = useState(60);
  const [rlApi, setRlApi] = useState(600);
  const [rlRedirect, setRlRedirect] = useState(6000);
  const [metricsToken, setMetricsToken] = useState("");
  const [metricsTokenSet, setMetricsTokenSet] = useState(false);

  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (settings) {
      setAppName(settings.appName ?? "");
      setRetention(settings.dataRetentionDays ?? 90);
      setRlAuth(settings.ratelimitAuthRpm ?? 60);
      setRlApi(settings.ratelimitApiRpm ?? 600);
      setRlRedirect(settings.ratelimitRedirectRpm ?? 6000);
      setMetricsTokenSet(settings.metricsTokenSet ?? false);
    }
  }, [settings]);

  async function saveGeneral() {
    setBusy(true);
    try {
      const payload: Parameters<typeof api.updateInstanceSettings>[0] = {
        appName,
        dataRetentionDays: retention,
        ratelimitAuthRpm: rlAuth,
        ratelimitApiRpm: rlApi,
        ratelimitRedirectRpm: rlRedirect,
        ...(metricsToken ? { metricsToken } : {}),
      };
      const v = await api.updateInstanceSettings(payload);
      setMetricsTokenSet(v.metricsTokenSet);
      setMetricsToken("");
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
      reload();
    } finally {
      setBusy(false);
    }
  }

  if (!settings) {
    return (
      <div className="flex h-32 items-center justify-center text-sm text-foreground/40">
        {t("settings.loadingInstanceSettings")}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.instanceTitle")}
        description={t("settings.instanceDesc")}
      />

      {/* App configuration */}
      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Server className="h-5 w-5 text-accent-fg" />
            <h2 className="text-base font-bold text-foreground">{t("settings.generalInfo")}</h2>
          </div>
          <SavedBadge on={saved} />
        </div>
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label={t("settings.instanceAppName")} hint={t("settings.instanceAppNameHint")}>
            <input
              className="input w-full text-sm"
              value={appName}
              onChange={(e) => setAppName(e.target.value)}
              placeholder="octarq"
            />
          </Field>
          <Field label={t("settings.retentionLabel")} hint={t("settings.retentionHint")}>
            <input
              type="number"
              min={0}
              className="input w-full font-mono text-sm"
              value={retention}
              onChange={(e) => setRetention(Number(e.target.value))}
            />
          </Field>
        </div>

        <Field
          label={t("settings.instanceMetricsToken")}
          hint={metricsTokenSet ? t("settings.instanceMetricsTokenSetHint") : t("settings.instanceMetricsTokenHint")}
        >
          <div className="flex gap-2 max-w-md">
            <input
              className="input w-full font-mono text-sm"
              type="password"
              value={metricsToken}
              onChange={(e) => setMetricsToken(e.target.value)}
              placeholder={metricsTokenSet ? "••••••••" : ""}
            />
            {metricsTokenSet && (
              <Button
                variant="ghost"
                className="shrink-0 text-xs text-danger-fg hover:text-danger-fg"
                onClick={async () => {
                  if (confirm(t("settings.clearMetricsToken"))) {
                    await api.updateInstanceSettings({ metricsToken: "" });
                    reload();
                  }
                }}
                disabled={busy}
              >
                {t("settings.instanceMetricsClear")}
              </Button>
            )}
          </div>
        </Field>

        <div className="border-t border-foreground/[0.06] pt-6">
          <Button variant="primary" onClick={saveGeneral} disabled={busy}>
            {busy ? t("settings.saving") : t("settings.save")}
          </Button>
        </div>
      </GlassCard>

      {/* Rate limits */}
      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center gap-2">
          <Sliders className="h-5 w-5 text-accent-fg" />
          <h2 className="text-base font-bold text-foreground">{t("settings.rateLimiting")}</h2>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 max-w-2xl">
          <Field label={t("settings.instanceRlAuth")} hint={t("settings.instanceRlHint")}>
            <input
              type="number"
              min={0}
              className="input w-full font-mono text-sm"
              value={rlAuth}
              onChange={(e) => setRlAuth(Number(e.target.value))}
            />
          </Field>
          <Field label={t("settings.instanceRlApi")}>
            <input
              type="number"
              min={0}
              className="input w-full font-mono text-sm"
              value={rlApi}
              onChange={(e) => setRlApi(Number(e.target.value))}
            />
          </Field>
          <Field label={t("settings.instanceRlRedirect")}>
            <input
              type="number"
              min={0}
              className="input w-full font-mono text-sm"
              value={rlRedirect}
              onChange={(e) => setRlRedirect(Number(e.target.value))}
            />
          </Field>
        </div>
        <div className="border-t border-foreground/[0.06] pt-6">
          <Button variant="primary" onClick={saveGeneral} disabled={busy}>
            {busy ? t("settings.saving") : t("settings.save")}
          </Button>
        </div>
      </GlassCard>

      {/* Infrastructure Extension Slot */}
      <div className="space-y-3 pt-2">
        <h3 className="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {t("settings.infrastructure", "Infrastructure")}
        </h3>
        <ExtensionSlot name="settings-infra" />
      </div>
    </div>
  );
}
