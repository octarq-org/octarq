import { Alert, ExtensionSlot } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { CheckCircle2 } from "lucide-react";
import { api, ApiError } from "../api";
import { useAppName } from "../brand";
import { BrandMark } from "./BrandMark";
import { useTranslation } from "../i18n";
import { authErrorKey, isVerifiedFlag } from "./authErrors";
import { LoginOAuthSection } from "./login/LoginOAuthSection";
import { LoginVerificationNotice } from "./login/LoginVerificationNotice";
import { LoginForgotSentNotice } from "./login/LoginForgotSentNotice";
import { LoginErrorBanner } from "./login/LoginErrorBanner";

export function Login({ onLogin }: { onLogin: (u: string, orgId: number) => void }) {
  const [u, setU] = useState("");
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
    onLogin(me.email || email, me.orgId);
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
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to resend verification email";
      setErr(msg);
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

        <LoginErrorBanner
          err={err}
          isMailUnavailableErr={isMailUnavailableErr}
          isUnverifiedErr={isUnverifiedErr}
          verifySent={verifySent}
          resendingVerify={resendingVerify}
          onResendVerification={handleResendVerification}
        />

        {pendingVerifyEmail ? (
          <LoginVerificationNotice
            email={pendingVerifyEmail}
            verifySent={verifySent}
            resendingVerify={resendingVerify}
            onResend={handleResendVerification}
            onBackToSignIn={() => switchMode("login")}
          />
        ) : mode === "forgot" && forgotSent ? (
          <LoginForgotSentNotice onBackToSignIn={() => switchMode("login")} />
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
                spellCheck={false}
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

        {mode !== "forgot" && !pendingVerifyEmail && !oauthPending && (
          <LoginOAuthSection oauthConfig={oauthConfig} />
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
