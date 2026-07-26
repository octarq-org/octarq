import { useState } from "react";
import { useSearchParams, useNavigate } from "react-router-dom";
import { api } from "../api";
import { Button, Alert } from "../ui";
import { KeyRound, ShieldCheck } from "lucide-react";
import { useTranslation } from "../i18n";
import { BrandMark } from "../shell/BrandMark";

export default function ResetPasswordPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") || "";
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [success, setSuccess] = useState(false);
  const navigate = useNavigate();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!token) {
      setErr(t("reset.errTokenMissing") || "Reset token is missing from the link.");
      return;
    }
    if (password.length < 8) {
      setErr(t("reset.errPasswordTooShort") || "Password must be at least 8 characters.");
      return;
    }
    if (password !== confirmPassword) {
      setErr(t("reset.errPasswordMismatch") || "Passwords do not match.");
      return;
    }

    setBusy(true);
    setErr("");
    try {
      await api.resetPassword(token, password);
      setSuccess(true);
      setTimeout(() => {
        navigate("/");
        window.location.reload();
      }, 2000);
    } catch (e: any) {
      setErr(e.message || t("reset.errResetFailed") || "Failed to reset password.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="octarq-aurora grid h-screen w-full place-items-center p-4">
      <div className="glass-strong w-full max-w-md rounded-2xl p-6 relative overflow-hidden">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-2xl rounded-full pointer-events-none" />

        <div className="mb-6 text-center">
          <BrandMark size="lg" className="mx-auto mb-3" />
          <h1 className="font-display text-xl font-bold text-foreground flex items-center justify-center gap-2">
            <ShieldCheck className="h-5 w-5 text-accent-fg" />
            {t("reset.heading") || "Reset Your Password"}
          </h1>
          <p className="text-xs text-foreground/50 mt-1.5 leading-relaxed">
            {t("reset.intro") || "Enter a new password for your account below."}
          </p>
        </div>

        {success ? (
          <div className="text-center py-6 space-y-3">
            <div className="mx-auto w-12 h-12 rounded-full bg-success-bg text-success-fg border border-success-border flex items-center justify-center font-bold text-xl">
              ✓
            </div>
            <h2 className="text-base font-semibold text-foreground">
              {t("reset.successHeading") || "Password Reset Complete"}
            </h2>
            <p className="text-xs text-foreground/40">
              {t("reset.successBody") || "Your password has been updated. Redirecting to sign in..."}
            </p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            {!token && (
              <Alert variant="danger" className="text-xs p-3 rounded-xl">
                {t("reset.noTokenWarning") || "Invalid or missing password reset link."}
              </Alert>
            )}

            <div>
              <label className="label">{t("reset.newPasswordLabel") || "New Password"}</label>
              <div className="relative mt-1">
                <input
                  type="password"
                  required
                  className="input w-full pl-9 text-sm animate-none"
                  placeholder="••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                <KeyRound className="absolute left-3 top-2.5 h-4 w-4 text-foreground/50" />
              </div>
            </div>

            <div>
              <label className="label">{t("reset.confirmPasswordLabel") || "Confirm New Password"}</label>
              <div className="relative mt-1">
                <input
                  type="password"
                  required
                  className="input w-full pl-9 text-sm animate-none"
                  placeholder="••••••••"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                />
                <KeyRound className="absolute left-3 top-2.5 h-4 w-4 text-foreground/50" />
              </div>
            </div>

            {err && <p className="text-xs text-danger-fg leading-normal">{err}</p>}

            <Button type="submit" variant="primary" className="w-full mt-2" disabled={busy || !token}>
              {busy ? t("reset.resetting") || "Resetting..." : t("reset.submit") || "Reset Password"}
            </Button>
          </form>
        )}
      </div>
    </div>
  );
}
