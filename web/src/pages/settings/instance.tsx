import { useEffect, useState } from "react";
import { api, InstanceSettings as InstanceSettingsData } from "../../api";
import { Field, PageHeader, GlassCard, Button, toast, confirmDialog } from "../../ui";
import { Server, Sliders, DatabaseBackup, Cpu } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useInstanceSettingsData, InstanceAdminOnly, SavedBadge } from "./shared";
import { ExtensionSlot } from "../../plugin-sdk";

export function InstanceSettings() {
  const { t } = useTranslation();
  const { s: settings, reload, forbidden } = useInstanceSettingsData();

  const [appName, setAppName] = useState("");
  const [baseDomain, setBaseDomain] = useState("");
  const [retention, setRetention] = useState(90);
  const [rlAuth, setRlAuth] = useState(60);
  const [rlApi, setRlApi] = useState(600);
  const [rlRedirect, setRlRedirect] = useState(6000);
  const [metricsToken, setMetricsToken] = useState("");
  const [metricsTokenSet, setMetricsTokenSet] = useState(false);

  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [backingUp, setBackingUp] = useState(false);
  const [build, setBuild] = useState<{ version: string; commit: string; builtAt: string } | null>(null);

  useEffect(() => {
    api.instanceBuild().then(setBuild).catch(() => {});
  }, []);

  useEffect(() => {
    if (settings) {
      setAppName(settings.appName ?? "");
      setBaseDomain(settings.baseDomain ?? "");
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
        baseDomain,
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

  async function handleBackup() {
    setBackingUp(true);
    try {
      const { blob, filename } = await api.downloadBackup();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err: any) {
      toast.error(err?.message || t("settings.backupFailed"));
    } finally {
      setBackingUp(false);
    }
  }

  if (forbidden) return <InstanceAdminOnly />;
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
          <Field label={t("settings.instanceBaseDomain")} hint={t("settings.instanceBaseDomainHint")}>
            <input
              className="input w-full font-mono text-sm"
              value={baseDomain}
              onChange={(e) => setBaseDomain(e.target.value)}
              placeholder="e.g. app.example.com"
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
                  if (await confirmDialog(t("settings.clearMetricsToken"))) {
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

      {/* Build stamp. Authenticated endpoint, values from the binary via
          ldflags; dev/unknown are legit values for a non-git build, so they
          render as-is. builtAt is the raw UTC string — no local-time masquerade. */}
      {build && (
        <GlassCard className="p-6 space-y-4">
          <div className="flex items-center gap-2">
            <Cpu className="h-5 w-5 text-accent-fg" />
            <h2 className="text-base font-bold text-foreground">{t("settings.buildTitle")}</h2>
          </div>
          <div className="grid grid-cols-1 gap-4 max-w-2xl sm:grid-cols-3">
            <Field label={t("settings.buildVersion")}>
              <div className="font-mono text-sm text-foreground/80">{build.version}</div>
            </Field>
            <Field label={t("settings.buildCommit")}>
              <div className="font-mono text-sm text-foreground/80">{build.commit}</div>
            </Field>
            <Field label={t("settings.buildBuiltAt")}>
              <div className="font-mono tnum text-sm text-foreground/80">{build.builtAt}</div>
            </Field>
          </div>
        </GlassCard>
      )}

      {/* Database backup. The endpoint has shipped since the backup work but had
          no UI — an operator's only way to reach it was the CLI (`octarq backup`)
          or a hand-written curl. Instance-admin only, and deliberately blunt
          about what the file is: a full dump of every workspace on this
          instance, encrypted secrets included. */}
      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center gap-2">
          <DatabaseBackup className="h-5 w-5 text-accent-fg" />
          <h2 className="text-base font-bold text-foreground">{t("settings.backupTitle")}</h2>
        </div>
        <p className="text-xs text-foreground/50">{t("settings.backupDesc")}</p>
        <div className="border-t border-foreground/[0.06] pt-4">
          <Button variant="outline" onClick={handleBackup} disabled={backingUp}>
            {backingUp ? t("settings.backupPreparing") : t("settings.backupDownload")}
          </Button>
        </div>
      </GlassCard>

      {/* Operator-level extension slot. Named "System Settings", NOT
          "Infrastructure": that word already labels the plugin CATEGORY
          (plugin.CategoryInfrastructure, workspace-scoped assets like domains
          and DNS), and the same word for two different scopes read as one
          feature split across two pages. */}
      <div className="space-y-3 pt-2">
        <h3 className="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {t("settings.systemSettings", "System Settings")}
        </h3>
        <ExtensionSlot name="settings-infra" />
      </div>
    </div>
  );
}
