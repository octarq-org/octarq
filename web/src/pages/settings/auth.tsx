import { useEffect, useState } from "react";
import { api } from "../../api";
import { Field, Toggle, PageHeader, GlassCard, Button } from "../../ui";
import { Globe, KeyRound } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useInstanceSettingsData, SavedBadge } from "./shared";
import { ExtensionSlot } from "../../plugin-sdk";

export function AuthenticationSettings() {
  const { t } = useTranslation();
  const { s: settings, reload } = useInstanceSettingsData();

  const [allowReg, setAllowReg] = useState(true);
  const [googleId, setGoogleId] = useState("");
  const [googleSecret, setGoogleSecret] = useState("");
  const [githubId, setGithubId] = useState("");
  const [githubSecret, setGithubSecret] = useState("");

  const [ssoBusy, setSsoBusy] = useState(false);
  const [ssoSaved, setSsoSaved] = useState(false);

  useEffect(() => {
    if (settings) {
      setAllowReg(settings.allowRegistration);
      setGoogleId(settings.googleClientId || "");
      setGithubId(settings.githubClientId || "");
    }
  }, [settings]);

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
      <div className="flex h-32 items-center justify-center text-sm text-foreground/40">
        {t("settings.loadingInstanceSettings")}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.accessAndSso")}
        description={t("settings.ssoDesc")}
      />

      {/* Registration & Social Login */}
      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center gap-2">
          <Globe className="h-5 w-5 text-accent-fg" />
          <h2 className="text-base font-bold text-foreground">{t("settings.accessAndSso")}</h2>
        </div>

        <div className="flex items-center justify-between gap-4 border-b border-foreground/[0.06] pb-6">
          <div>
            <p className="text-sm font-medium text-foreground/85">{t("settings.allowPublicSignup")}</p>
            <p className="text-[11px] text-foreground/40 mt-0.5">
              {t("settings.allowPublicSignupDesc")}
            </p>
          </div>
          <Toggle on={allowReg} onChange={toggleRegistration} />
        </div>

        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-foreground/90">{t("settings.singleSignOn")}</h3>
            <SavedBadge on={ssoSaved} />
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div className="space-y-3 rounded-xl border border-foreground/[0.05] bg-well p-4">
              <p className="flex items-center gap-1.5 text-xs font-bold text-foreground/85">
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
              <p className="text-[10px] text-foreground/50">
                Callback URL: <span className="font-mono text-foreground/50">{"{HOST}/api/auth/google/callback"}</span>
              </p>
            </div>

            <div className="space-y-3 rounded-xl border border-foreground/[0.05] bg-well p-4">
              <p className="flex items-center gap-1.5 text-xs font-bold text-foreground/85">
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
              <p className="text-[10px] text-foreground/50">
                Callback URL: <span className="font-mono text-foreground/50">{"{HOST}/api/auth/github/callback"}</span>
              </p>
            </div>
          </div>
        </div>

        <div className="border-t border-foreground/[0.06] pt-6">
          <Button variant="primary" onClick={saveSso} disabled={ssoBusy}>
            {ssoBusy ? t("settings.savingDots") : t("settings.saveSsoOptions")}
          </Button>
        </div>
      </GlassCard>

      {/* Pro OIDC Extension Slot */}
      <ExtensionSlot name="settings-auth" />
    </div>
  );
}
