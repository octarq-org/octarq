import { Alert, ExtensionSlot } from "@octarq/plugin-sdk";
import { useEffect, useRef, useState } from "react";
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

export function Login({ onLogin }: { onLogin: (u: string, orgId: number, next?: string | null) => void }) {
  const [u, setU] = useState("");
  const [p, setP] = useState("");
  const [workspace, setWorkspace] = useState("");
  const [code, setCode] = useState("");
  const [needs2FA, setNeeds2FA] = useState(false);
  const [oauthPending, setOauthPending] = useState(false);
  const [mode, setMode] = useState<"login" | "register" | "forgot">("login");
  const [forgotSent, setForgotSent] = useState(false);
  const [pendingVerifyEmail, setPendingVerifyEmail] = useState("");
  const [resendingVerify, setResendingVerify] = useState(false);
  const [verifySent, setVerifySent] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [isVerifiedNotice, setIsVerifiedNotice] = useState(false);
  const [oauthConfig, setOauthConfig] = useState<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean } | null>(null);
  const nextTargetRef = useRef<string | null>(null);
  const appName = useAppName();
  const { t } = useTranslation();

  useEffect(() => {
    api.authConfig()
      .then((cfg) => {
        setOauthConfig(cfg);
        if (!cfg.registrationEnabled) setMode((m) => (m === "register" ? "login" : m));
      })
      .catch(() => setOauthConfig(null));

    const params = new URLSearchParams(window.location.search);
    nextTargetRef.current = params.get("next");
    const verified = isVerifiedFlag(params.get("verified"));
    if (verified) setIsVerifiedNotice(true);

    const errKey = authErrorKey(params.get("error"));
    if (errKey) setErr(t(errKey));

    if (params.get("mode") === "register") {
      setMode("register");
    }

    if (params.get("twofa") === "1") {
      setOauthPending(true);
      setNeeds2FA(true);
      setMode("login");
    }

    if (verified || errKey) {
      const remaining = new URLSearchParams(window.location.search);
      remaining.delete("verified");
      remaining.delete("error");
      const qs = remaining.toString();
      window.history.replaceState({}, "", window.location.pathname + (qs ? `?${qs}` : ""));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function finishLogin(email: string) {
    const me = await api.me();
    if (nextTargetRef.current) {
      onLogin(me.email || email, me.orgId, nextTargetRef.current);
    } else {
      onLogin(me.email || email, me.orgId);
    }
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
        await api.register(u.trim(), p, workspace.trim());
        setPendingVerifyEmail(u.trim());
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
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    doSubmit();
  }

  function onEnter(e: React.KeyboardEvent) {
    if (e.key === "Enter") { e.preventDefault(); doSubmit(); }
  }

  const isUnverifiedErr = err.toLowerCase().includes("email verification required");
  const isMailUnavailableErr = err.toLowerCase().includes("cannot send email");

  return (
    <div className="relative grid h-full place-items-center overflow-y-auto bg-sidebar p-4 sm:p-8">
      <div className="relative w-full max-w-[25rem] overflow-hidden rounded-xl border border-border bg-card p-7 shadow-[var(--shadow-float)] sm:p-8">
        <div className="mb-6 text-center">
          <BrandMark size="lg" className="mx-auto mb-4" />
          <h1 className="text-xl font-semibold tracking-tight text-foreground">
            {pendingVerifyEmail
              ? t("app.registerVerifyTitle")
              : mode === "register"
              ? t("app.createAccount")
              : mode === "forgot"
              ? t("app.forgotPasswordTitle")
              : t("app.signInTo", { app: appName })}
          </h1>
          {!pendingVerifyEmail && (
            <p className="mt-2 text-[12.5px] leading-relaxed text-muted-foreground">
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
          <Alert variant="success" icon={<CheckCircle2 className="h-4 w-4 shrink-0" />} className="mb-4 rounded-lg p-3 text-xs">
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
                {mode === "login" ? t("app.emailOrUser") : t("app.email")}
              </label>
              <input
                id="login-email"
                type={mode === "login" ? "text" : "email"}
                name="email"
                className="input animate-none"
                value={u}
                onChange={(e) => setU(e.target.value)}
                onKeyDown={onEnter}
                autoComplete={mode === "login" ? "username" : "email"}
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

            <button type="submit" className="btn-primary mt-2 h-11 w-full" disabled={busy}>
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

        <p className="mt-6 border-t border-border pt-4 text-center font-mono text-[11px] text-muted-foreground">
          {window.location.host}
        </p>
      </div>
    </div>
  );
}
