import { useEffect, useState } from "react";
import { ShieldAlert, CheckCircle2, Mail } from "lucide-react";
import { api, ApiError } from "../api";
import { useAppName } from "../brand";
import { BrandMark } from "./BrandMark";
import { useTranslation } from "../i18n";
import { Alert } from "../ui";
import { ExtensionSlot } from "../plugin-sdk";
import { oauthBeginPath } from "./oauthRoutes";
import { authErrorKey, isVerifiedFlag } from "./authErrors";

export function Login({ onLogin }: { onLogin: (u: string, orgId: number) => void }) {
  const [u, setU] = useState("admin");
  const [p, setP] = useState("");
  const [workspace, setWorkspace] = useState("");
  const [code, setCode] = useState("");
  const [needs2FA, setNeeds2FA] = useState(false);
  // Set when the OAuth callback bounced us here with ?twofa=1: the account
  // needs its second factor, and the signed challenge proving the OAuth
  // round-trip is already waiting in an HttpOnly cookie — the page never sees
  // its value.
  const [oauthPending, setOauthPending] = useState(false);
  const [mode, setMode] = useState<"login" | "register" | "forgot">("login");
  const [forgotSent, setForgotSent] = useState(false);
  // Set from the register response's verificationRequired flag: the account
  // exists but the instance withheld the session until the email is verified.
  // Never inferred from "did we get a cookie" — the server states it.
  const [pendingVerifyEmail, setPendingVerifyEmail] = useState("");
  const [resendingVerify, setResendingVerify] = useState(false);
  const [verifySent, setVerifySent] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [isVerifiedNotice, setIsVerifiedNotice] = useState(false);
  const [oauthConfig, setOauthConfig] = useState<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean } | null>(null);
  const appName = useAppName();
  const { t } = useTranslation();

  useEffect(() => {
    api.authConfig()
      .then((cfg) => {
        setOauthConfig(cfg);
        // A ?mode=register link must not force the register form when sign-up
        // is disabled: the URL parameter is a convenience, not a bypass.
        if (!cfg.registrationEnabled) setMode((m) => (m === "register" ? "login" : m));
      })
      .catch(() => setOauthConfig(null));

    const params = new URLSearchParams(window.location.search);
    const verified = isVerifiedFlag(params.get("verified"));
    if (verified) setIsVerifiedNotice(true);

    // Show navigation/OAuth redirect error in banner.
    const errKey = authErrorKey(params.get("error"));
    if (errKey) setErr(t(errKey));

    // Marketing entry point: /signup lands here with ?mode=register and the
    // register form preselected. The registrationEnabled guard above runs once
    // config arrives; until then the form renders but submitting is rejected
    // server-side anyway.
    if (params.get("mode") === "register") {
      setMode("register");
      if (u === "admin") setU("");
    }

    // OAuth callback with a pending second factor: /admin/?twofa=1. The query
    // only carries the fact "this login still needs its second factor" — the
    // challenge itself lives in the HttpOnly cookie the callback set. Safe to
    // leave in the URL: a refresh re-reads the fact and the cookie completes
    // the login, and there is no key material to scrub from history.
    if (params.get("twofa") === "1") {
      setOauthPending(true);
      setNeeds2FA(true);
      setMode("login");
    }

    if (verified || errKey) {
      window.history.replaceState({}, "", window.location.pathname);
    }
    // Run once on mount to prevent re-triggering dismissed banners.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function finishLogin(email: string) {
    const me = await api.me();
    onLogin(email, me.orgId);
  }

  async function doSubmit() {
    if (busy) return;
    setErr("");
    setBusy(true);
    setVerifySent(false);
    setPendingVerifyEmail("");

    try {
      if (mode === "forgot") {
        await api.forgotPassword(u.trim());
        setForgotSent(true);
        return;
      }

      if (mode === "register") {
        const res = await api.register(u.trim(), p, workspace.trim());
        if (res.verificationRequired) {
          // No session was issued; sending them to the dashboard would just
          // bounce off /api/auth/me. Ask for the mailbox instead.
          setPendingVerifyEmail(u.trim());
          return;
        }
        await finishLogin(u.trim());
        return;
      }

      if (needs2FA) {
        if (oauthPending) {
          await api.verify2FAChallenge(code.trim());
        } else {
          await api.verify2FA(u, p, code.trim());
        }
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

  async function handleResendVerification() {
    if (!u.trim()) return;
    setResendingVerify(true);
    try {
      const res = await api.resendVerification(u.trim());
      if (res.mailConfigured === false) {
        setErr(t("app.verificationMailNotConfigured"));
      } else {
        setVerifySent(true);
      }
    } catch (e: any) {
      setErr(e.message || "Failed to resend verification email");
    } finally {
      setResendingVerify(false);
    }
  }

  function switchMode(next: "login" | "register" | "forgot") {
    setMode(next);
    setErr("");
    setForgotSent(false);
    setVerifySent(false);
    setPendingVerifyEmail("");
    setNeeds2FA(false);
    setOauthPending(false);
    setCode("");
    if (next === "register" || next === "forgot") {
      if (u === "admin") setU("");
    }
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    doSubmit();
  }

  function onEnter(e: React.KeyboardEvent) {
    if (e.key === "Enter") { e.preventDefault(); doSubmit(); }
  }

  const hasOauth = oauthConfig && (oauthConfig.googleEnabled || oauthConfig.githubEnabled);
  const isUnverifiedErr = err.toLowerCase().includes("email verification required");
  // The register endpoint answers 503 with this detail when verification is
  // required but no SMTP sender is configured. Show the localized notice
  // instead of the raw English API message.
  const isMailUnavailableErr = err.toLowerCase().includes("cannot send email");

  return (
    <div className="bg-background grid h-full place-items-center p-4">
      <div className="glass-strong w-full max-w-md rounded-2xl p-8">

        <div className="mb-6 text-center">
          <BrandMark size="lg" className="mx-auto mb-4" />
          <h1 className="font-display text-2xl font-bold text-foreground">
            {pendingVerifyEmail
              ? t("app.registerVerifyTitle")
              : mode === "register"
              ? t("app.createAccount")
              : mode === "forgot"
              ? t("app.forgotPasswordTitle")
              : t("app.signInTo", { app: appName })}
          </h1>
          {/* When verification is pending the explanation lives in the panel
              below, next to the resend action — no need to say it twice. */}
          {!pendingVerifyEmail && (
            <p className="text-xs text-muted-foreground mt-1.5 leading-relaxed">
              {oauthPending
                ? t("app.twoFactorOAuthDesc")
                : mode === "register"
                ? t("app.registerSubtitle")
                : mode === "forgot"
                ? t("app.forgotPasswordDesc")
                : t("app.loginSubtitle")}
            </p>
          )}
        </div>

        {isVerifiedNotice && (
          <Alert variant="success" icon={<CheckCircle2 className="h-4 w-4 shrink-0" />} className="mb-4 text-xs p-3 rounded-xl">
            <span>{t("app.emailVerifiedSuccess")}</span>
          </Alert>
        )}

        {err && (
          <div className="mb-4 p-3 rounded-xl bg-danger-fg/10 border border-danger-fg/20 text-danger-fg text-xs space-y-2">
            <div className="flex gap-2 items-center">
              <ShieldAlert className="h-4 w-4 shrink-0" />
              <span>{isMailUnavailableErr ? t("app.registerMailUnavailable") : err}</span>
            </div>
            {isUnverifiedErr && (
              <div className="pt-1">
                {verifySent ? (
                  <p className="text-success-fg font-medium">
                    ✓ {t("app.verificationSent")}
                  </p>
                ) : (
                  <button
                    type="button"
                    onClick={handleResendVerification}
                    disabled={resendingVerify}
                    className="flex items-center gap-1.5 text-xs text-accent-fg font-medium underline"
                  >
                    <Mail className="h-3.5 w-3.5" />
                    {resendingVerify
                      ? t("app.sending")
                      : t("app.resendVerificationBtn")}
                  </button>
                )}
              </div>
            )}
          </div>
        )}

        {pendingVerifyEmail ? (
          <div className="py-2 space-y-4">
            <Alert variant="info" icon={<Mail className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
              <span>{t("app.registerVerifyNotice", { email: pendingVerifyEmail })}</span>
            </Alert>
            <div className="text-center space-y-3">
              {verifySent ? (
                <p className="text-xs text-success-fg font-medium">✓ {t("app.verificationSent")}</p>
              ) : (
                <button
                  type="button"
                  onClick={handleResendVerification}
                  disabled={resendingVerify}
                  className="text-xs text-accent-fg hover:underline font-medium"
                >
                  {resendingVerify ? t("app.sending") : t("app.resendVerificationBtn")}
                </button>
              )}
              <div>
                <button
                  type="button"
                  onClick={() => switchMode("login")}
                  className="text-xs text-accent-fg hover:underline font-medium"
                >
                  {t("app.backToSignIn")}
                </button>
              </div>
            </div>
          </div>
        ) : mode === "forgot" && forgotSent ? (
          <div className="text-center py-4 space-y-4">
            <Alert variant="success" icon={<CheckCircle2 className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
              <span>{t("app.forgotSentNotice")}</span>
            </Alert>
            <button
              type="button"
              onClick={() => switchMode("login")}
              className="text-xs text-accent-fg hover:underline font-medium"
            >
              {t("app.backToSignIn")}
            </button>
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <div>
              <label className="label" htmlFor="login-email">
                {t("app.email")}
              </label>
              <input
                id="login-email"
                type="email"
                name="email"
                className="input animate-none"
                value={u}
                onChange={(e) => setU(e.target.value)}
                onKeyDown={onEnter}
                autoComplete="email"
                placeholder={t("app.emailPlaceholder")}
                required
                disabled={oauthPending}
              />
            </div>

            {mode === "register" && (
              <div>
                <label className="label" htmlFor="login-workspace">
                  {t("app.workspaceName")}
                </label>
                <input
                  id="login-workspace"
                  type="text"
                  name="workspace"
                  className="input animate-none"
                  value={workspace}
                  onChange={(e) => setWorkspace(e.target.value)}
                  onKeyDown={onEnter}
                  autoComplete="organization"
                  placeholder={t("app.workspaceNamePlaceholder")}
                />
              </div>
            )}

            {!oauthPending && mode !== "forgot" && (
              <div>
                <div className="flex items-center justify-between">
                  <label className="label" htmlFor="login-password">{t("app.password")}</label>
                  {mode === "login" && (
                    <button
                      type="button"
                      onClick={() => switchMode("forgot")}
                      className="text-xs text-accent-fg hover:underline"
                    >
                      {t("app.forgotPasswordLink")}
                    </button>
                  )}
                </div>
                <input
                  id="login-password"
                  type="password"
                  name="password"
                  className="input animate-none mt-1"
                  value={p}
                  onChange={(e) => setP(e.target.value)}
                  onKeyDown={onEnter}
                  autoComplete={mode === "register" ? "new-password" : "current-password"}
                  autoFocus={!needs2FA}
                  placeholder={mode === "register" ? t("app.passwordRegisterPlaceholder") : "••••••••"}
                  required
                />
              </div>
            )}

            {needs2FA && (mode === "login" || oauthPending) && (
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
              {busy
                ? mode === "register"
                  ? t("app.creating")
                  : mode === "forgot"
                  ? t("app.sending")
                  : t("app.signingIn")
                : mode === "register"
                ? t("app.createAccountBtn")
                : mode === "forgot"
                ? t("app.sendResetLink")
                : needs2FA
                ? t("app.verifyOtp")
                : t("app.signIn")}
            </button>
          </form>
        )}

        {mode !== "forgot" && !pendingVerifyEmail && <ExtensionSlot name="login-methods" />}

        {mode === "forgot" && !forgotSent && (
          <p className="mt-4 text-center text-xs text-muted-foreground">
            <button type="button" onClick={() => switchMode("login")} className="text-accent-fg hover:underline font-medium">
              {t("app.backToSignIn")}
            </button>
          </p>
        )}

        {mode !== "forgot" && !pendingVerifyEmail && oauthConfig?.registrationEnabled && !needs2FA && (
          <p className="mt-5 text-center text-xs text-muted-foreground">
            {mode === "register" ? (
              <>{t("app.haveAccount")}{" "}
                <button type="button" onClick={() => switchMode("login")} className="text-accent-fg hover:underline font-medium">{t("app.signInLink")}</button>
              </>
            ) : (
              <>{t("app.noAccount")}{" "}
                <button type="button" onClick={() => switchMode("register")} className="text-accent-fg hover:underline font-medium">{t("app.createOne")}</button>
              </>
            )}
          </p>
        )}

        {mode !== "forgot" && !pendingVerifyEmail && !oauthPending && hasOauth && (
          <div className="mt-6 space-y-3">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className="h-px flex-1 bg-border" />
              <span>{t("app.orContinueWith")}</span>
              <span className="h-px flex-1 bg-border" />
            </div>
            <div className="grid grid-cols-1 gap-2">
              {oauthConfig.googleEnabled && (
                <a
                  href={oauthBeginPath("google")}
                  className="flex items-center justify-center gap-2 rounded-xl border border-border px-3 py-2.5 text-sm text-foreground/70 hover:bg-surface-hover transition-colors font-medium"
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
                  href={oauthBeginPath("github")}
                  className="flex items-center justify-center gap-2 rounded-xl border border-border px-3 py-2.5 text-sm text-foreground/70 hover:bg-surface-hover transition-colors font-medium"
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

        {/* Instance identity: the host you are signing into, machine-provided so
            mono. Deliberately the ONLY place it appears on this card — the same
            fact stated twice reads as noise, not reassurance. Footer rather than
            under the title because it is a stamp, not part of the heading, and
            because this spot stays visible in every mode (login / register /
            forgot / pending-verification). */}
        <p className="mt-6 border-t border-border pt-4 text-center font-mono text-[11px] text-foreground/40">
          {window.location.host}
        </p>
      </div>
    </div>
  );
}
