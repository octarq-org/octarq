import { useState } from "react";
import { useSearchParams, useNavigate } from "react-router-dom";
import { api } from "../api";
import { useAppName, brandInitial } from "../brand";
import { Button } from "../ui";
import { Sparkles, KeyRound } from "lucide-react";
import { useTranslation } from "../i18n";

export default function InviteAcceptPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") || "";
  const appName = useAppName();
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [success, setSuccess] = useState(false);
  const navigate = useNavigate();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!token) {
      setErr(t("invite.errTokenMissing"));
      return;
    }
    if (password.length < 6) {
      setErr(t("invite.errPasswordTooShort"));
      return;
    }
    if (password !== confirmPassword) {
      setErr(t("invite.errPasswordMismatch"));
      return;
    }

    setBusy(true);
    setErr("");
    try {
      await api.acceptInvite(token, password);
      setSuccess(true);
      setTimeout(() => {
        navigate("/");
        window.location.reload();
      }, 2000);
    } catch (e: any) {
      setErr(e.message || t("invite.errAcceptFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="led-aurora grid h-screen w-full place-items-center p-4">
      <div className="glass-strong w-full max-w-md rounded-2xl p-6 relative overflow-hidden">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-2xl rounded-full pointer-events-none" />

        <div className="mb-6 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
            <span className="font-display text-xl font-extrabold text-white">{brandInitial(appName)}</span>
          </div>
          <h1 className="font-display text-xl font-bold text-white flex items-center justify-center gap-2">
            <Sparkles className="h-5 w-5 text-indigo-400 animate-pulse" />
            {t("invite.heading")}
          </h1>
          <p className="text-xs text-white/50 mt-1.5 leading-relaxed">
            {t("invite.intro")}
          </p>
        </div>

        {success ? (
          <div className="text-center py-6 space-y-3">
            <div className="mx-auto w-12 h-12 rounded-full bg-emerald-500/10 text-emerald-400 flex items-center justify-center font-bold text-xl">
              ✓
            </div>
            <h2 className="text-base font-semibold text-white">{t("invite.successHeading")}</h2>
            <p className="text-xs text-white/40">{t("invite.successBody")}</p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            {!token && (
              <div className="p-3 bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs rounded-xl">
                {t("invite.noTokenWarning")}
              </div>
            )}

            <div>
              <label className="label">{t("invite.newPasswordLabel")}</label>
              <div className="relative mt-1">
                <input
                  type="password"
                  required
                  className="input w-full pl-9 text-sm animate-none"
                  placeholder={t("invite.newPasswordPlaceholder")}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                <KeyRound className="absolute left-3 top-2.5 h-4 w-4 text-white/30" />
              </div>
            </div>

            <div>
              <label className="label">{t("invite.confirmPasswordLabel")}</label>
              <div className="relative mt-1">
                <input
                  type="password"
                  required
                  className="input w-full pl-9 text-sm animate-none"
                  placeholder={t("invite.confirmPasswordPlaceholder")}
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                />
                <KeyRound className="absolute left-3 top-2.5 h-4 w-4 text-white/30" />
              </div>
            </div>

            {err && <p className="text-xs text-rose-400 leading-normal">{err}</p>}

            <Button type="submit" variant="primary" className="w-full mt-2" disabled={busy || !token}>
              {busy ? t("invite.activating") : t("invite.activate")}
            </Button>
          </form>
        )}
      </div>
    </div>
  );
}
