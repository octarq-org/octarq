import { ReactNode, useEffect, useState } from "react";
import { api } from "../../api";
import { Field, Toggle, PageHeader, GlassCard, Button, confirmDialog } from "../../ui";
import { Mail, ChevronDown, Check } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useInstanceSettingsData } from "./shared";
import { ExtensionSlot } from "../../plugin-sdk";

// Provider glyphs — inline so they don't depend on the icon set (matches the
// SVGs the Login page uses for the OAuth buttons).
function GoogleGlyph({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" aria-hidden="true">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4" />
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853" />
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z" fill="#FBBC05" />
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335" />
    </svg>
  );
}
function GithubGlyph({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
    </svg>
  );
}

// One provider row of the Supabase-style list: an accordion whose header shows
// the provider + enabled badge, expanding to reveal its config. `builtin`
// providers (Email) are always on and can't be collapsed to "disabled".
function ProviderRow({
  icon,
  name,
  description,
  enabled,
  builtin,
  children,
}: {
  icon: ReactNode;
  name: string;
  description: string;
  enabled: boolean;
  builtin?: boolean;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const expandable = !builtin && !!children;
  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        onClick={() => expandable && setOpen((o) => !o)}
        className={`flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors ${expandable ? "hover:bg-surface-hover" : "cursor-default"}`}
        aria-expanded={expandable ? open : undefined}
      >
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-card">{icon}</span>
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">{name}</span>
          <span className="block truncate text-xs text-muted-foreground">{description}</span>
        </span>
        <span
          className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${
            enabled
              ? "bg-success-fg/10 text-success-fg"
              : "bg-muted text-muted-foreground"
          }`}
        >
          {enabled ? t("settings.providerEnabled", "Enabled") : t("settings.providerDisabled", "Disabled")}
        </span>
        {expandable && (
          <ChevronDown className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`} />
        )}
      </button>
      {expandable && open && <div className="space-y-3 border-t border-border bg-well/50 px-4 py-4">{children}</div>}
    </div>
  );
}

export function AuthenticationSettings() {
  const { t } = useTranslation();
  const { s: settings, reload } = useInstanceSettingsData();

  const [allowReg, setAllowReg] = useState(true);
  const [requireVerify, setRequireVerify] = useState(false);
  const [googleId, setGoogleId] = useState("");
  const [googleSecret, setGoogleSecret] = useState("");
  const [githubId, setGithubId] = useState("");
  const [githubSecret, setGithubSecret] = useState("");
  const [saving, setSaving] = useState<"" | "google" | "github">("");
  const [saved, setSaved] = useState<"" | "google" | "github">("");

  useEffect(() => {
    if (settings) {
      setAllowReg(settings.allowRegistration);
      setRequireVerify(!!settings.requireEmailVerification);
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

  async function toggleRequireVerification(next: boolean) {
    setRequireVerify(next);
    try {
      await api.updateInstanceSettings({ requireEmailVerification: next });
      reload();
    } catch {
      setRequireVerify(!next);
    }
  }

  // Per-provider save (Supabase saves each provider independently).
  async function saveProvider(which: "google" | "github", body: Record<string, string>) {
    setSaving(which);
    try {
      await api.updateInstanceSettings(body);
      if (which === "google") setGoogleSecret("");
      else setGithubSecret("");
      setSaved(which);
      setTimeout(() => setSaved(""), 2000);
      reload();
    } finally {
      setSaving("");
    }
  }

  async function clearSecret(field: "googleClientSecret" | "githubClientSecret", confirmKey: string) {
    if (!(await confirmDialog(t(confirmKey)))) return;
    await api.updateInstanceSettings({ [field]: "" } as Record<string, string>);
    reload();
  }

  if (!settings) {
    return (
      <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
        {t("settings.loadingInstanceSettings")}
      </div>
    );
  }

  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const googleEnabled = !!settings.googleClientId && !!settings.googleClientSecretSet;
  const githubEnabled = !!settings.githubClientId && !!settings.githubClientSecretSet;

  // A read-only callback URL field with copy affordance — providers need the
  // exact redirect URI registered on their side.
  const CallbackField = ({ path }: { path: string }) => (
    <Field label={t("settings.callbackUrl", "Callback URL")} hint={t("settings.callbackUrlHint", "Register this exact URL with the provider.")}>
      <input readOnly className="input w-full cursor-text bg-well font-mono text-xs text-foreground/80" value={`${origin}${path}`} />
    </Field>
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.authTitle", "Authentication")}
        description={t("settings.authDesc", "Configure how people sign in to this instance.")}
      />

      {/* Account creation policy */}
      <GlassCard className="flex items-center justify-between gap-4 p-5">
        <div>
          <p className="text-sm font-medium text-foreground">{t("settings.allowPublicSignup")}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("settings.allowPublicSignupDesc")}</p>
        </div>
        <Toggle on={allowReg} onChange={toggleRegistration} />
      </GlassCard>

      {/* Email verification gate */}
      <GlassCard className="flex items-center justify-between gap-4 p-5">
        <div>
          <p className="text-sm font-medium text-foreground">{t("settings.requireEmailVerification", "Require Email Verification")}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("settings.requireEmailVerificationDesc", "Block sign-in for users until their email address has been verified.")}</p>
        </div>
        <Toggle on={requireVerify} onChange={toggleRequireVerification} />
      </GlassCard>

      {/* Provider list (Supabase-style accordion) */}
      <div>
        <h3 className="mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {t("settings.authProviders", "Auth providers")}
        </h3>
        <GlassCard className="overflow-hidden !p-0">
          <ProviderRow
            icon={<Mail className="h-4 w-4 text-accent-fg" strokeWidth={1.75} />}
            name={t("settings.providerEmail", "Email")}
            description={t("settings.providerEmailDesc", "Built-in username & password sign-in")}
            enabled
            builtin
          />

          <ProviderRow
            icon={<GoogleGlyph className="h-4 w-4" />}
            name="Google"
            description={t("settings.providerGoogleDesc", "OAuth sign-in with a Google account")}
            enabled={googleEnabled}
          >
            <Field label={t("settings.googleClientId")}>
              <input className="input w-full text-xs" value={googleId} onChange={(e) => setGoogleId(e.target.value)} placeholder="*.apps.googleusercontent.com" />
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
                  <Button variant="danger" onClick={() => clearSecret("googleClientSecret", "settings.clearGoogleSecret")} className="px-2.5 py-1 text-xs">
                    {t("settings.clear")}
                  </Button>
                )}
              </div>
            </Field>
            <CallbackField path="/api/auth/google/callback" />
            <div className="flex items-center gap-3 pt-1">
              <Button
                variant="primary"
                disabled={saving === "google"}
                onClick={() => {
                  const body: Record<string, string> = { googleClientId: googleId.trim() };
                  if (googleSecret.trim()) body.googleClientSecret = googleSecret.trim();
                  saveProvider("google", body);
                }}
              >
                {saving === "google" ? t("settings.savingDots") : t("settings.save")}
              </Button>
              {saved === "google" && (
                <span className="flex items-center gap-1 text-xs text-success-fg">
                  <Check className="h-3.5 w-3.5" /> {t("settings.saved", "Saved")}
                </span>
              )}
            </div>
          </ProviderRow>

          <ProviderRow
            icon={<GithubGlyph className="h-4 w-4 text-foreground" />}
            name="GitHub"
            description={t("settings.providerGithubDesc", "OAuth sign-in with a GitHub account")}
            enabled={githubEnabled}
          >
            <Field label={t("settings.githubClientId")}>
              <input className="input w-full text-xs" value={githubId} onChange={(e) => setGithubId(e.target.value)} placeholder="Ov23li…" />
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
                  <Button variant="danger" onClick={() => clearSecret("githubClientSecret", "settings.clearGithubSecret")} className="px-2.5 py-1 text-xs">
                    {t("settings.clear")}
                  </Button>
                )}
              </div>
            </Field>
            <CallbackField path="/api/auth/github/callback" />
            <div className="flex items-center gap-3 pt-1">
              <Button
                variant="primary"
                disabled={saving === "github"}
                onClick={() => {
                  const body: Record<string, string> = { githubClientId: githubId.trim() };
                  if (githubSecret.trim()) body.githubClientSecret = githubSecret.trim();
                  saveProvider("github", body);
                }}
              >
                {saving === "github" ? t("settings.savingDots") : t("settings.save")}
              </Button>
              {saved === "github" && (
                <span className="flex items-center gap-1 text-xs text-success-fg">
                  <Check className="h-3.5 w-3.5" /> {t("settings.saved", "Saved")}
                </span>
              )}
            </div>
          </ProviderRow>

          {/* Pro providers (Enterprise OIDC, …) inject their own rows here so they
              sit in the same list. Empty in the OSS build. */}
          <ExtensionSlot name="settings-auth" />
        </GlassCard>
      </div>
    </div>
  );
}
