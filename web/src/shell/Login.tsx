import { useEffect, useState } from "react";
import { ShieldAlert } from "lucide-react";
import { api, ApiError } from "../api";
import { useAppName, brandInitial } from "../brand";
import { useTranslation } from "../i18n";

export function Login({ onLogin }: { onLogin: (u: string, orgId: number) => void }) {
  const [u, setU] = useState("admin");
  const [p, setP] = useState("");
  const [code, setCode] = useState("");
  const [needs2FA, setNeeds2FA] = useState(false);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [oauthConfig, setOauthConfig] = useState<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean } | null>(null);
  const appName = useAppName();
  const { t } = useTranslation();

  useEffect(() => {
    api.authConfig()
      .then(setOauthConfig)
      .catch(() => setOauthConfig({ googleEnabled: false, githubEnabled: false, registrationEnabled: false }));
  }, []);

  async function finishLogin(username: string) {
    const m = await api.me();
    onLogin(username, m.orgId);
  }

  async function doSubmit() {
    if (busy) return;
    setBusy(true);
    setErr("");
    try {
      if (mode === "register") {
        await api.register(u.trim(), p);
        await finishLogin(u.trim());
        return;
      }
      if (needs2FA) {
        await api.verify2FA(u, p, code.trim());
        await finishLogin(u);
        return;
      }
      const res = await api.login(u, p);
      if (res.twoFactorRequired) {
        setNeeds2FA(true);
        return;
      }
      await finishLogin(u);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : mode === "register" ? "sign up failed" : "login failed");
    } finally {
      setBusy(false);
    }
  }

  function switchMode(next: "login" | "register") {
    setMode(next);
    setErr("");
    setNeeds2FA(false);
    setCode("");
    setU(next === "register" ? "" : "admin");
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    doSubmit();
  }

  function onEnter(e: React.KeyboardEvent) {
    if (e.key === "Enter") { e.preventDefault(); doSubmit(); }
  }

  const hasOauth = oauthConfig && (oauthConfig.googleEnabled || oauthConfig.githubEnabled);

  return (
    <div className="led-aurora grid h-full place-items-center p-4">
      <div className="glass-strong w-full max-w-md rounded-2xl p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />
        
        <div className="mb-6 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
            <span className="font-display text-xl font-extrabold text-white">{brandInitial(appName)}</span>
          </div>
          <h1 className="font-display text-2xl font-bold text-white">{mode === "register" ? t("app.createAccount") : t("app.signInTo", { app: appName })}</h1>
          <p className="text-xs text-white/40 mt-1.5 leading-relaxed">{mode === "register" ? t("app.registerSubtitle") : t("app.loginSubtitle")}</p>
        </div>

        {err && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{err}</span>
          </div>
        )}

        <form onSubmit={submit} className="space-y-4">
          <div>
            <label className="label" htmlFor="login-username">{mode === "register" ? t("app.email") : t("app.username")}</label>
            <input
              id="login-username"
              type={mode === "register" ? "email" : "text"}
              name={mode === "register" ? "email" : "username"}
              className="input animate-none"
              value={u}
              onChange={(e) => setU(e.target.value)}
              onKeyDown={onEnter}
              autoComplete={mode === "register" ? "email" : "username"}
              placeholder={mode === "register" ? t("app.emailPlaceholder") : t("app.usernamePlaceholder")}
            />
          </div>

          <div>
            <label className="label" htmlFor="login-password">{t("app.password")}</label>
            <input
              id="login-password"
              type="password"
              name="password"
              className="input animate-none"
              value={p}
              onChange={(e) => setP(e.target.value)}
              onKeyDown={onEnter}
              autoComplete={mode === "register" ? "new-password" : "current-password"}
              autoFocus={!needs2FA}
              placeholder={mode === "register" ? t("app.passwordRegisterPlaceholder") : "••••••••"}
            />
          </div>

          {needs2FA && (
            <div>
              <label className="label" htmlFor="login-otp">{t("app.authCode")}</label>
              <input
                id="login-otp"
                name="otp"
                className="input animate-none"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                onKeyDown={onEnter}
                placeholder={t("app.authCodePlaceholder")}
                autoComplete="one-time-code"
                autoFocus
              />
            </div>
          )}

          <button type="submit" className="btn-primary w-full py-2.5 mt-2" disabled={busy}>
            {busy ? (mode === "register" ? t("app.creating") : t("app.signingIn")) : mode === "register" ? t("app.createAccountBtn") : needs2FA ? t("app.verifyOtp") : t("app.signIn")}
          </button>
        </form>

        {oauthConfig?.registrationEnabled && !needs2FA && (
          <p className="mt-5 text-center text-xs text-white/40">
            {mode === "register" ? (
              <>{t("app.haveAccount")}{" "}
                <button type="button" onClick={() => switchMode("login")} className="text-indigo-300 hover:underline font-medium">{t("app.signInLink")}</button>
              </>
            ) : (
              <>{t("app.noAccount")}{" "}
                <button type="button" onClick={() => switchMode("register")} className="text-indigo-300 hover:underline font-medium">{t("app.createOne")}</button>
              </>
            )}
          </p>
        )}

        {hasOauth && (
          <div className="mt-6 space-y-3">
            <div className="flex items-center gap-2 text-xs text-white/30">
              <span className="h-px flex-1 bg-white/10" />
              <span>{t("app.orContinueWith")}</span>
              <span className="h-px flex-1 bg-white/10" />
            </div>
            <div className="grid grid-cols-1 gap-2">
              {oauthConfig.googleEnabled && (
                <a
                  href="/auth/begin/google"
                  className="flex items-center justify-center gap-2 rounded-xl border border-white/10 px-3 py-2.5 text-sm text-white/70 hover:bg-white/5 transition-colors font-medium"
                >
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
                    <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
                    <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z" fill="#FBBC05"/>
                    <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
                  </svg>
                  <span>Google</span>
                </a>
              )}
              {oauthConfig.githubEnabled && (
                <a
                  href="/auth/begin/github"
                  className="flex items-center justify-center gap-2 rounded-xl border border-white/10 px-3 py-2.5 text-sm text-white/70 hover:bg-white/5 transition-colors font-medium"
                >
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/>
                  </svg>
                  <span>GitHub</span>
                </a>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

