import { useEffect, useState } from "react";
import { api, InstanceSettings as InstanceSettingsData } from "../../api";
import { Field, Toggle, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Server, ShieldAlert, KeyRound, Globe, Sliders } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useInstanceSettingsData, SavedBadge } from "./shared";

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

  const [googleId, setGoogleId] = useState("");
  const [googleSecret, setGoogleSecret] = useState("");
  const [githubId, setGithubId] = useState("");
  const [githubSecret, setGithubSecret] = useState("");
  const [allowReg, setAllowReg] = useState(true);

  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  const [ssoBusy, setSsoBusy] = useState(false);
  const [ssoSaved, setSsoSaved] = useState(false);

  useEffect(() => {
    if (settings) {
      setAppName(settings.appName ?? "");
      setRetention(settings.dataRetentionDays ?? 90);
      setRlAuth(settings.ratelimitAuthRpm ?? 60);
      setRlApi(settings.ratelimitApiRpm ?? 600);
      setRlRedirect(settings.ratelimitRedirectRpm ?? 6000);
      setMetricsTokenSet(settings.metricsTokenSet ?? false);

      setGoogleId(settings.googleClientId || "");
      setGithubId(settings.githubClientId || "");
      setAllowReg(settings.allowRegistration);
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

  async function toggleRegistration(next: boolean) {
    setAllowReg(next);
    try {
      await api.updateInstanceSettings({ allowRegistration: next });
      reload();
    } catch {
      setAllowReg(!next);
    }
  }

  async function saveSso() {
    setSsoBusy(true);
    try {
      const p: any = { googleClientId: googleId.trim(), githubClientId: githubId.trim() };
      if (googleSecret.trim()) p.googleClientSecret = googleSecret.trim();
      if (githubSecret.trim()) p.githubClientSecret = githubSecret.trim();
      await api.updateInstanceSettings(p);
      setGoogleSecret("");
      setGithubSecret("");
      setSsoSaved(true);
      setTimeout(() => setSsoSaved(false), 2000);
      reload();
    } finally {
      setSsoBusy(false);
    }
  }

  if (!settings) {
    return (
      <div className="flex h-32 items-center justify-center text-sm text-white/40">
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
            <Server className="h-5 w-5 text-indigo-400" />
            <h2 className="text-base font-bold text-white">{t("settings.generalInfo")}</h2>
          </div>
          <SavedBadge on={saved} />
        </div>
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label={t("settings.instanceAppName")} hint={t("settings.instanceAppNameHint")}>
            <input
              className="input w-full text-sm"
              value={appName}
              onChange={(e) => setAppName(e.target.value)}
              placeholder="led"
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
                className="shrink-0 text-xs text-rose-400 hover:text-rose-300"
                onClick={async () => {
                  if (confirm(t("settings.clearGoogleSecret"))) {
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

        <div className="border-t border-white/[0.06] pt-6">
          <Button variant="primary" onClick={saveGeneral} disabled={busy}>
            {busy ? t("settings.saving") : t("settings.save")}
          </Button>
        </div>
      </GlassCard>

      {/* Rate limits */}
      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center gap-2">
          <Sliders className="h-5 w-5 text-indigo-400" />
          <h2 className="text-base font-bold text-white">{t("settings.rateLimiting")}</h2>
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
        <div className="border-t border-white/[0.06] pt-6">
          <Button variant="primary" onClick={saveGeneral} disabled={busy}>
            {busy ? t("settings.saving") : t("settings.save")}
          </Button>
        </div>
      </GlassCard>

      {/* Registration & SSO */}
      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center gap-2">
          <Globe className="h-5 w-5 text-indigo-400" />
          <h2 className="text-base font-bold text-white">{t("settings.accessAndSso")}</h2>
        </div>

        <div className="flex items-center justify-between gap-4 border-b border-white/[0.06] pb-6">
          <div>
            <p className="text-sm font-medium text-white/85">{t("settings.allowPublicSignup")}</p>
            <p className="text-[11px] text-white/40 mt-0.5">
              {t("settings.allowPublicSignupDesc")}
            </p>
          </div>
          <Toggle on={allowReg} onChange={toggleRegistration} />
        </div>

        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-white/90">{t("settings.singleSignOn")}</h3>
            <SavedBadge on={ssoSaved} />
          </div>
          <p className="text-[11px] text-white/40">
            {t("settings.ssoDesc")}
          </p>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
              <p className="flex items-center gap-1.5 text-xs font-bold text-white/85">
                <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" /> {t("settings.googleSignIn")}
              </p>
              <Field label={t("settings.googleClientId")}>
                <input
                  className="input w-full text-xs"
                  value={googleId}
                  onChange={(e) => setGoogleId(e.target.value)}
                  placeholder="*.apps.googleusercontent.com"
                />
              </Field>
              <Field label={t("settings.googleClientSecret")}>
                <div className="flex gap-2">
                  <input
                    className="input w-full font-mono text-xs"
                    type="password"
                    value={googleSecret}
                    onChange={(e) => setGoogleSecret(e.target.value)}
                    placeholder={settings.googleClientSecretSet ? t("settings.secretSet") : t("settings.secretValue")}
                  />
                  {settings.googleClientSecretSet && (
                    <Button
                      variant="danger"
                      onClick={async () => {
                        if (confirm(t("settings.clearGoogleSecret"))) {
                          await api.updateInstanceSettings({ googleClientSecret: "" });
                          reload();
                        }
                      }}
                      className="px-2.5 py-1 text-xs"
                    >
                      {t("settings.clear")}
                    </Button>
                  )}
                </div>
              </Field>
              <p className="text-[10px] text-white/30">
                Callback URL: <span className="font-mono text-white/50">{"{HOST}/api/auth/google/callback"}</span>
              </p>
            </div>

            <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
              <p className="flex items-center gap-1.5 text-xs font-bold text-white/85">
                <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" /> {t("settings.githubIntegration")}
              </p>
              <Field label={t("settings.githubClientId")}>
                <input
                  className="input w-full text-xs"
                  value={githubId}
                  onChange={(e) => setGithubId(e.target.value)}
                  placeholder="Ov23li…"
                />
              </Field>
              <Field label={t("settings.githubClientSecret")}>
                <div className="flex gap-2">
                  <input
                    className="input w-full font-mono text-xs"
                    type="password"
                    value={githubSecret}
                    onChange={(e) => setGithubSecret(e.target.value)}
                    placeholder={settings.githubClientSecretSet ? t("settings.secretSet") : t("settings.secretValue")}
                  />
                  {settings.githubClientSecretSet && (
                    <Button
                      variant="danger"
                      onClick={async () => {
                        if (confirm(t("settings.clearGithubSecret"))) {
                          await api.updateInstanceSettings({ githubClientSecret: "" });
                          reload();
                        }
                      }}
                      className="px-2.5 py-1 text-xs"
                    >
                      {t("settings.clear")}
                    </Button>
                  )}
                </div>
              </Field>
              <p className="text-[10px] text-white/30">
                Callback URL: <span className="font-mono text-white/50">{"{HOST}/api/auth/github/callback"}</span>
              </p>
            </div>
          </div>
        </div>

        <div className="border-t border-white/[0.06] pt-6">
          <Button variant="primary" onClick={saveSso} disabled={ssoBusy}>
            {ssoBusy ? t("settings.savingDots") : t("settings.saveSsoOptions")}
          </Button>
        </div>
      </GlassCard>
    </div>
  );
}
