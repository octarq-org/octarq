import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, useInstanceSettingsData, SavedBadge } from "./shared";

function parseUA(ua: string): { browser: string; os: string } {
  if (!ua) return { browser: "Unknown", os: "" };
  let browser = "Browser";
  let os = "";
  if (ua.includes("Edg/")) browser = "Microsoft Edge";
  else if (ua.includes("OPR/") || ua.includes("Opera")) browser = "Opera";
  else if (ua.includes("Chrome")) browser = "Chrome";
  else if (ua.includes("Firefox")) browser = "Firefox";
  else if (ua.includes("Safari") && !ua.includes("Chrome")) browser = "Safari";
  else if (ua.includes("curl")) browser = "curl / API";
  if (ua.includes("Windows")) os = "Windows";
  else if (ua.includes("Mac OS X")) os = "macOS";
  else if (ua.includes("Linux")) os = "Linux";
  else if (ua.includes("iPhone") || ua.includes("iPad")) os = "iOS";
  else if (ua.includes("Android")) os = "Android";
  return { browser, os };
}


function SessionsList({ onRevokeAll }: { onRevokeAll: () => void }) {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [revoking, setRevoking] = useState<number | null>(null);

  function load() {
    setLoading(true);
    api.sessions().then(setSessions).catch(() => setSessions([])).finally(() => setLoading(false));
  }
  useEffect(() => { load(); }, []);

  async function revoke(id: number, isSelf: boolean) {
    const msg = isSelf
      ? t("settings.logoutThisDevice")
      : t("settings.revokeSessionConfirm");
    if (!confirm(msg)) return;
    setRevoking(id);
    try {
      const r = await api.revokeSession(id);
      if (r.self) {
        window.location.href = "/";
      } else {
        load();
      }
    } catch (e: any) {
      alert(e.message || t("settings.revokeFailed"));
    } finally {
      setRevoking(null);
    }
  }

  if (loading) return <div className="text-xs text-white/40 py-4 text-center">{t("settings.loadingSessions")}</div>;
  if (sessions.length === 0) return <div className="text-xs text-white/40 py-4 text-center">{t("settings.noSessions")}</div>;

  return (
    <div className="divide-y divide-white/[0.04] rounded-xl border border-white/[0.05] overflow-hidden">
      {sessions.map((s) => {
        const ua = parseUA(s.userAgent);
        return (
          <div key={s.id} className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium text-white/85">{ua.browser}</span>
                {s.isCurrent && <Badge tone="green">{t("settings.current")}</Badge>}
                <span className="text-xs text-white/35">{ua.os}</span>
              </div>
              <div className="flex items-center gap-3 mt-1">
                <span className="text-xs text-white/40">{s.location || s.ip}</span>
                <span className="text-xs text-white/30">{t("settings.lastSeen", { time: timeAgo(s.lastSeenAt) })}</span>
                <span className="text-xs text-white/25">{t("settings.signedIn", { time: timeAgo(s.createdAt) })}</span>
              </div>
            </div>
            <Button
              variant="danger"
              onClick={() => revoke(s.id, s.isCurrent)}
              disabled={revoking === s.id}
              className="text-xs py-1 px-2.5 shrink-0"
            >
              {revoking === s.id ? "…" : s.isCurrent ? t("settings.logOut") : t("settings.revoke")}
            </Button>
          </div>
        );
      })}
    </div>
  );
}

export function SecuritySettings() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");

  // Enrollment state.
  const [setup, setSetup] = useState<{ secret: string; otpauthUrl: string; qrDataUri?: string } | null>(null);
  const [enrollCode, setEnrollCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);

  // Disable state.
  const [disableCode, setDisableCode] = useState("");

  // SSO state (merged from SignInSettings).
  const { s: wS } = useSettingsData();
  const { s: ssoSettings, reload: ssoReload } = useInstanceSettingsData();
  const [googleId, setGoogleId] = useState("");
  const [googleSecret, setGoogleSecret] = useState("");
  const [githubId, setGithubId] = useState("");
  const [githubSecret, setGithubSecret] = useState("");
  const [ssoBusy, setSsoBusy] = useState(false);
  const [ssoSaved, setSsoSaved] = useState(false);
  const [allowReg, setAllowReg] = useState(true);

  useEffect(() => {
    if (ssoSettings) {
      setGoogleId(ssoSettings.googleClientId || "");
      setGithubId(ssoSettings.githubClientId || "");
      setAllowReg(ssoSettings.allowRegistration);
    }
  }, [ssoSettings]);

  async function toggleRegistration(next: boolean) {
    setAllowReg(next);
    try { await api.updateInstanceSettings({ allowRegistration: next }); ssoReload(); }
    catch { setAllowReg(!next); }
  }

  async function saveSso() {
    setSsoBusy(true);
    try {
      const p: any = { googleClientId: googleId.trim(), githubClientId: githubId.trim() };
      if (googleSecret.trim()) p.googleClientSecret = googleSecret.trim();
      if (githubSecret.trim()) p.githubClientSecret = githubSecret.trim();
      await api.updateInstanceSettings(p);
      setGoogleSecret(""); setGithubSecret(""); setSsoSaved(true); setTimeout(() => setSsoSaved(false), 2000); ssoReload();
    } finally { setSsoBusy(false); }
  }

  async function load() {
    try {
      const s = await api.twoFAStatus();
      setEnabled(s.enabled);
    } catch {
      setEnabled(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function beginSetup() {
    setBusy(true); setErr(""); setMsg(""); setRecoveryCodes(null);
    try {
      setSetup(await api.twoFASetup());
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : t("settings.failedStartSetup"));
    } finally { setBusy(false); }
  }

  async function confirmEnable() {
    setBusy(true); setErr("");
    try {
      const res = await api.twoFAEnable(enrollCode.trim());
      setRecoveryCodes(res.recoveryCodes);
      setSetup(null); setEnrollCode("");
      await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : t("settings.invalidCode"));
    } finally { setBusy(false); }
  }

  async function disable() {
    setBusy(true); setErr(""); setMsg("");
    try {
      await api.twoFADisable({ code: disableCode.trim() });
      setDisableCode("");
      setMsg(t("settings.twoFADisabledMsg"));
      await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : t("settings.verificationFailed"));
    } finally { setBusy(false); }
  }

  async function logoutAll() {
    if (!confirm(t("settings.signOutEveryDevice"))) return;
    setBusy(true); setErr("");
    try {
      await api.logoutAll();
      // The current session cookie is now revoked; bounce to the login screen.
      window.location.href = "/";
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : t("settings.failed"));
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.securityTitle")} description={t("settings.securityDescription")} />

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white flex items-center gap-2"><Shield className="h-4 w-4" /> {t("settings.twoFATitle")}</h2>
          <Badge tone={enabled ? "green" : "neutral"}>{enabled == null ? "…" : enabled ? t("settings.enabled") : t("settings.disabled")}</Badge>
        </div>
        <p className="text-xs text-white/50">{t("settings.twoFADesc")}</p>

        {err && <p className="text-sm text-rose-400">{err}</p>}
        {msg && <p className="text-sm text-emerald-400">{msg}</p>}

        {recoveryCodes && (
          <div className="rounded-xl border border-amber-400/30 bg-amber-400/[0.06] p-4">
            <p className="text-xs font-bold text-amber-300 mb-2">{t("settings.saveRecoveryCodes")}</p>
            <p className="text-[11px] text-white/50 mb-3">{t("settings.recoveryCodesDesc")}</p>
            <div className="grid grid-cols-2 gap-1 font-mono text-xs text-white/80">
              {recoveryCodes.map((c) => <span key={c}>{c}</span>)}
            </div>
          </div>
        )}

        {!enabled && !setup && (
          <Button variant="primary" onClick={beginSetup} disabled={busy}>{busy ? "…" : t("settings.enable2FA")}</Button>
        )}

        {!enabled && setup && (
          <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
            <p className="text-xs text-white/60">{t("settings.scanInstructions")}</p>
            <img
              alt={t("settings.qrAlt")}
              className="rounded-lg bg-white p-2"
              width={160}
              height={160}
              src={setup.qrDataUri}
            />
            <Field label="Setup key (if you can't scan)">
              <input className="input w-full font-mono text-xs" readOnly value={setup.secret} />
            </Field>
            <a className="block break-all text-[10px] text-indigo-300/70 hover:underline" href={setup.otpauthUrl}>{setup.otpauthUrl}</a>
            <Field label="Verification code">
              <input className="input w-full text-sm" value={enrollCode} onChange={(e) => setEnrollCode(e.target.value)} placeholder="123456" autoComplete="one-time-code" />
            </Field>
            <div className="flex gap-2">
              <Button variant="primary" onClick={confirmEnable} disabled={busy || !enrollCode.trim()}>{busy ? "…" : "Confirm & Enable"}</Button>
              <Button variant="ghost" onClick={() => { setSetup(null); setEnrollCode(""); }}>Cancel</Button>
            </div>
          </div>
        )}

        {enabled && (
          <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
            <p className="text-xs text-white/60">Enter a current authenticator code (or a recovery code) to turn off 2FA.</p>
            <Field label="Verification code">
              <input className="input w-full text-sm" value={disableCode} onChange={(e) => setDisableCode(e.target.value)} placeholder="123456 or recovery code" autoComplete="one-time-code" />
            </Field>
            <Button variant="danger" onClick={disable} disabled={busy || !disableCode.trim()}>{busy ? "…" : "Disable 2FA"}</Button>
          </div>
        )}
      </GlassCard>

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">Active sessions</h2>
          <Button variant="danger" onClick={logoutAll} disabled={busy} className="text-xs py-1 px-3">
            Sign out of all
          </Button>
        </div>
        <p className="text-xs text-white/50">All devices where you're currently signed in. Revoking any session invalidates all cookies and re-authenticates you here.</p>
        <SessionsList onRevokeAll={logoutAll} />
      </GlassCard>

      {wS?.isInstanceAdmin && (
        <>
          <GlassCard className="p-6 space-y-4">
            <h2 className="text-base font-bold text-white">Access control</h2>
            <div className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm text-white/85">Allow public sign-up</p>
                <p className="text-[10px] text-white/40 mt-0.5">When on, anyone can create an account with an email and password from the sign-in page. Turn off to make this an invite-only instance.</p>
              </div>
              <Toggle on={allowReg} onChange={toggleRegistration} />
            </div>
          </GlassCard>

          <GlassCard className="p-6 space-y-6">
            <div className="flex items-center justify-between"><h2 className="text-base font-bold text-white">Single Sign-On</h2><SavedBadge on={ssoSaved} /></div>
            <p className="text-[10px] text-white/40">Let people sign in with Google or GitHub. Make sure the server callback URLs match your LED base URL. Credentials are stored encrypted.</p>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
                <p className="flex items-center gap-1.5 text-xs font-bold text-white/85"><span className="h-1.5 w-1.5 rounded-full bg-indigo-400" /> Google Sign-In</p>
                <Field label="Google Client ID"><input className="input w-full text-xs" value={googleId} onChange={(e) => setGoogleId(e.target.value)} placeholder="*.apps.googleusercontent.com" /></Field>
                <Field label="Google Client Secret">
                  <div className="flex gap-2">
                    <input className="input w-full font-mono text-xs" type="password" value={googleSecret} onChange={(e) => setGoogleSecret(e.target.value)} placeholder={ssoSettings?.googleClientSecretSet ? "•••••••• (Set)" : "Secret value"} />
                    {ssoSettings?.googleClientSecretSet && <Button variant="danger" onClick={async () => { if (confirm("Clear Google secret?")) { await api.updateInstanceSettings({ googleClientSecret: "" }); ssoReload(); } }} className="px-2.5 py-1 text-xs">Clear</Button>}
                  </div>
                </Field>
                <p className="text-[10px] text-white/30">Callback URL: <span className="font-mono text-white/50">{"{HOST}/api/auth/google/callback"}</span></p>
              </div>
              <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
                <p className="flex items-center gap-1.5 text-xs font-bold text-white/85"><span className="h-1.5 w-1.5 rounded-full bg-indigo-400" /> GitHub Integration</p>
                <Field label="GitHub Client ID"><input className="input w-full text-xs" value={githubId} onChange={(e) => setGithubId(e.target.value)} placeholder="Ov23li…" /></Field>
                <Field label="GitHub Client Secret">
                  <div className="flex gap-2">
                    <input className="input w-full font-mono text-xs" type="password" value={githubSecret} onChange={(e) => setGithubSecret(e.target.value)} placeholder={ssoSettings?.githubClientSecretSet ? "•••••••• (Set)" : "Secret value"} />
                    {ssoSettings?.githubClientSecretSet && <Button variant="danger" onClick={async () => { if (confirm("Clear GitHub secret?")) { await api.updateInstanceSettings({ githubClientSecret: "" }); ssoReload(); } }} className="px-2.5 py-1 text-xs">Clear</Button>}
                  </div>
                </Field>
              </div>
            </div>
            <div className="border-t border-white/[0.06] pt-6"><Button variant="primary" onClick={saveSso} disabled={ssoBusy}>{ssoBusy ? "Saving…" : "Save"}</Button></div>
          </GlassCard>
        </>
      )}
    </div>
  );
}
