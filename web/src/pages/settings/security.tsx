import { useEffect, useState } from "react";
import { api, ApiError, type LinkedIdentity } from "../../api";
import { Field, timeAgo, PageHeader, GlassCard, Badge, Button, toast, Alert, confirmDialog, confirmPassword } from "../../ui";
import { Shield } from "lucide-react";
import { useTranslation } from "../../i18n";

// First-match tables, in priority order. Mobile UAs embed desktop tokens
// ("iPhone OS 17_0 like Mac OS X", "Linux; Android 14"), so the mobile
// checks must precede the desktop ones or phones read as macOS/Linux.
const UA_BROWSERS: Array<[string, (ua: string) => boolean]> = [
  ["Microsoft Edge", (ua) => ua.includes("Edg/")],
  ["Opera", (ua) => ua.includes("OPR/") || ua.includes("Opera")],
  ["Chrome", (ua) => ua.includes("Chrome")],
  ["Firefox", (ua) => ua.includes("Firefox")],
  ["Safari", (ua) => ua.includes("Safari") && !ua.includes("Chrome")],
  ["curl / API", (ua) => ua.includes("curl")],
];

const UA_OSS: Array<[string, (ua: string) => boolean]> = [
  ["iOS", (ua) => ua.includes("iPhone") || ua.includes("iPad")],
  ["Android", (ua) => ua.includes("Android")],
  ["Windows", (ua) => ua.includes("Windows")],
  ["macOS", (ua) => ua.includes("Mac OS X")],
  ["Linux", (ua) => ua.includes("Linux")],
];

export function parseUA(ua: string): { browser?: string; browserKey?: "uaUnknown" | "uaBrowser"; os: string } {
  if (!ua) return { browserKey: "uaUnknown", os: "" };
  const browser = UA_BROWSERS.find(([, match]) => match(ua))?.[0];
  const os = UA_OSS.find(([, match]) => match(ua))?.[0] ?? "";
  return {
    browser,
    browserKey: browser === undefined ? "uaBrowser" : undefined,
    os,
  };
}


function SessionsList() {
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
    if (!(await confirmDialog(msg))) return;
    setRevoking(id);
    try {
      const r = await api.revokeSession(id);
      if (r.self) {
        window.location.href = "/";
      } else {
        toast.success(t("settings.saved"));
        load();
      }
    } catch (e: any) {
      toast.error(e.message || t("settings.revokeFailed"));
    } finally {
      setRevoking(null);
    }
  }

  if (loading) return <div className="text-xs text-foreground/40 py-4 text-center">{t("settings.loadingSessions")}</div>;
  if (sessions.length === 0) return <div className="text-xs text-foreground/40 py-4 text-center">{t("settings.noSessions")}</div>;

  return (
    <div className="divide-y divide-foreground/[0.04] rounded-xl border border-foreground/[0.05] overflow-hidden">
      {sessions.map((s) => {
        const ua = parseUA(s.userAgent);
        // Spelled out rather than t(`settings.${ua.browserKey}`): an
        // interpolated key is invisible to the audit's key-resolution check,
        // and worse, it registers "settings." as a dynamic prefix, which
        // exempts every settings.* key from the unreferenced-key report. Same
        // reason SCOPE_LABEL in PersonalSettings.tsx is an explicit map.
        const browserName =
          ua.browserKey === "uaUnknown"
            ? t("settings.uaUnknown")
            : ua.browserKey === "uaBrowser"
              ? t("settings.uaBrowser")
              : ua.browser;
        return (
          <div key={s.id} className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium text-foreground/85">{browserName}</span>
                {s.isCurrent && <Badge tone="green">{t("settings.current")}</Badge>}
                <span className="text-xs text-foreground/50">{ua.os}</span>
              </div>
              <div className="flex items-center gap-3 mt-1">
                <span className="font-mono text-xs text-foreground/40">{s.location || s.ip}</span>
                <span className="font-mono tnum text-xs text-foreground/50">{t("settings.lastSeen", { time: timeAgo(s.lastSeenAt) })}</span>
                <span className="font-mono tnum text-xs text-foreground/25">{t("settings.signedIn", { time: timeAgo(s.createdAt) })}</span>
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

// LinkedIdentities lists the external identities that can sign in as this
// account. There is no "link" button here on purpose: creating a binding needs
// a verified assertion, which only the identity plugin that ran the handshake
// holds, so it offers that from its own page. Removal is core's, because being
// able to cut off a way into your account must not depend on which plugins the
// build happens to include.
function LinkedIdentities() {
  const { t } = useTranslation();
  const [items, setItems] = useState<LinkedIdentity[] | null>(null);
  const [busy, setBusy] = useState<number | null>(null);

  function load() {
    api.identities().then(setItems).catch(() => setItems([]));
  }
  useEffect(() => { load(); }, []);

  async function unlink(id: LinkedIdentity) {
    if (!(await confirmDialog(t("settings.unlinkIdentityConfirm", { issuer: id.issuer })))) return;
    setBusy(id.id);
    try {
      await api.unlinkIdentity(id.id);
      toast.success(t("settings.saved"));
      load();
    } catch (e) {
      // 409 is the server refusing to leave the account with no way in.
      toast.error(e instanceof ApiError ? e.message : t("settings.failed"));
    } finally {
      setBusy(null);
    }
  }

  if (items === null) return <div className="text-xs text-foreground/40 py-4 text-center">{t("settings.loadingIdentities")}</div>;
  if (items.length === 0) return <div className="text-xs text-foreground/40 py-4 text-center">{t("settings.noLinkedIdentities")}</div>;

  return (
    <div className="divide-y divide-foreground/[0.04] rounded-xl border border-foreground/[0.05] overflow-hidden">
      {items.map((id) => (
        <div key={id.id} className="flex items-center gap-3 px-4 py-3">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-foreground/85 truncate">{id.issuer}</div>
            <div className="flex items-center gap-3 mt-1">
              <span className="font-mono text-xs text-foreground/50 truncate">{id.email}</span>
              <span className="font-mono tnum text-xs text-foreground/25">{t("settings.linkedAt", { time: timeAgo(id.createdAt) })}</span>
            </div>
          </div>
          <Button
            variant="danger"
            onClick={() => unlink(id)}
            disabled={busy === id.id}
            className="text-xs py-1 px-2.5 shrink-0"
          >
            {busy === id.id ? "…" : t("settings.unlink")}
          </Button>
        </div>
      ))}
    </div>
  );
}

export function SecuritySettings() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");

  // Which login methods exist on this instance, so the 2FA card can state the
  // truth about its coverage: password + built-in OAuth are covered, per-org
  // SSO delegates the second factor to the identity provider.
  const [hasOauth, setHasOauth] = useState(false);
  const [hasSso, setHasSso] = useState(false);

  // Enrollment state.
  const [setup, setSetup] = useState<{ secret: string; otpauthUrl: string; qrDataUri?: string } | null>(null);
  const [enrollCode, setEnrollCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);

  // Disable state.
  const [disableCode, setDisableCode] = useState("");

  async function load() {
    try {
      const s = await api.twoFAStatus();
      setEnabled(s.enabled);
    } catch {
      setEnabled(false);
    }
  }
  useEffect(() => { load(); }, []);

  useEffect(() => {
    api.authConfig()
      .then((cfg) => setHasOauth(cfg.googleEnabled || cfg.githubEnabled))
      .catch(() => {});
    // /api/auth/methods is public; plugin-sso registers its method there only
    // while configured, so its presence means this instance offers SSO.
    fetch("/api/auth/methods")
      .then((r) => r.json())
      .then((m: { id: string }[]) =>
        setHasSso(m.some((x) => x.id === "sso" || x.id.startsWith("oidc") || x.id.startsWith("saml"))),
      )
      .catch(() => {});
  }, []);

  // Both halves of the 2FA switch re-authenticate. A live session is exactly
  // what an attacker holds when the second factor is the last thing standing,
  // so it must not be enough on its own to attach one — or strip one.
  async function beginSetup() {
    setErr(""); setMsg(""); setRecoveryCodes(null);
    const password = await confirmPassword({
      message: t("settings.twoFAEnableConfirm"),
      confirmLabel: t("settings.enable2FA"),
    });
    if (password === null) return;
    setBusy(true);
    try {
      setSetup(await api.twoFASetup(password));
    } catch (e: any) {
      const msg = e instanceof ApiError ? e.message : t("settings.failedStartSetup");
      setErr(msg);
      toast.error(msg);
    } finally { setBusy(false); }
  }

  async function confirmEnable() {
    setBusy(true); setErr("");
    try {
      const res = await api.twoFAEnable(enrollCode.trim());
      setRecoveryCodes(res.recoveryCodes);
      setSetup(null); setEnrollCode("");
      toast.success(t("settings.saved"));
      await load();
    } catch (e: any) {
      const msg = e instanceof ApiError ? e.message : t("settings.invalidCode");
      setErr(msg);
      toast.error(msg);
    } finally { setBusy(false); }
  }

  async function disable() {
    setErr(""); setMsg("");
    const password = await confirmPassword({
      message: t("settings.twoFADisableConfirm"),
      confirmLabel: t("settings.disable2FA"),
    });
    if (password === null) return;
    setBusy(true);
    try {
      await api.twoFADisable({ code: disableCode.trim(), password });
      setDisableCode("");
      setMsg(t("settings.twoFADisabledMsg"));
      toast.success(t("settings.twoFADisabledMsg"));
      await load();
    } catch (e: any) {
      const msg = e instanceof ApiError ? e.message : t("settings.verificationFailed");
      setErr(msg);
      toast.error(msg);
    } finally { setBusy(false); }
  }

  async function logoutAll() {
    if (!(await confirmDialog(t("settings.signOutEveryDevice")))) return;
    setBusy(true); setErr("");
    try {
      await api.logoutAll();
      // The current session cookie is now revoked; bounce to the login screen.
      window.location.href = "/";
    } catch (e: any) {
      const msg = e instanceof ApiError ? e.message : t("settings.failed");
      setErr(msg);
      toast.error(msg);
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.securityTitle")} description={t("settings.securityDescription")} />

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-foreground flex items-center gap-2"><Shield className="h-4 w-4" /> {t("settings.twoFATitle")}</h2>
          <Badge tone={enabled ? "green" : "neutral"}>{enabled == null ? "…" : enabled ? t("settings.enabled") : t("settings.disabled")}</Badge>
        </div>
        <p className="text-xs text-foreground/50">{t("settings.twoFADesc")}</p>

        {enabled && (
          <div className="space-y-1">
            <p className="text-[11px] text-foreground/45">{t("settings.twoFACoverageBase")}</p>
            {hasOauth && <p className="text-[11px] text-foreground/45">{t("settings.twoFACoverageOauth")}</p>}
            {hasSso && <p className="text-[11px] text-foreground/45">{t("settings.twoFACoverageSso")}</p>}
          </div>
        )}

        {err && <p className="text-sm text-danger-fg">{err}</p>}
        {msg && <p className="text-sm text-success-fg">{msg}</p>}

        {recoveryCodes && (
          <Alert variant="warning" className="p-4">
            <p className="text-xs font-bold text-warning-fg mb-2">{t("settings.saveRecoveryCodes")}</p>
            <p className="text-[11px] text-foreground/50 mb-3">{t("settings.recoveryCodesDesc")}</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5 font-mono text-xs text-foreground/80">
              {recoveryCodes.map((c) => <span key={c} className="select-all whitespace-nowrap">{c}</span>)}
            </div>
          </Alert>
        )}

        {!enabled && !setup && (
          <Button variant="primary" onClick={beginSetup} disabled={busy}>{busy ? "…" : t("settings.enable2FA")}</Button>
        )}

        {!enabled && setup && (
          <div className="space-y-3 rounded-xl border border-foreground/[0.05] bg-well p-4">
            <p className="text-xs text-foreground/60">{t("settings.scanInstructions")}</p>
            <img
              alt={t("settings.qrAlt")}
              className="rounded-lg bg-white p-2"
              width={160}
              height={160}
              src={setup.qrDataUri}
            />
            <Field label={t("settings.setupKeyLabel")}>
              <input className="input w-full font-mono text-xs" readOnly value={setup.secret} />
            </Field>
            <a className="block break-all text-[10px] text-accent-fg/70 hover:underline" href={setup.otpauthUrl}>{setup.otpauthUrl}</a>
            <Field label={t("settings.verificationCode")}>
              <input className="input w-full text-sm" value={enrollCode} onChange={(e) => setEnrollCode(e.target.value)} placeholder="123456" autoComplete="one-time-code" />
            </Field>
            <div className="flex gap-2">
              <Button variant="primary" onClick={confirmEnable} disabled={busy || !enrollCode.trim()}>{busy ? "…" : t("settings.confirmEnable")}</Button>
              <Button variant="ghost" onClick={() => { setSetup(null); setEnrollCode(""); }}>{t("settings.cancel")}</Button>
            </div>
          </div>
        )}

        {enabled && (
          <div className="space-y-3 rounded-xl border border-foreground/[0.05] bg-well p-4">
            <p className="text-xs text-foreground/60">{t("settings.disableInstructions")}</p>
            <Field label={t("settings.verificationCode")}>
              <input className="input w-full text-sm" value={disableCode} onChange={(e) => setDisableCode(e.target.value)} placeholder={t("settings.disableCodePlaceholder")} autoComplete="one-time-code" />
            </Field>
            <Button variant="danger" onClick={disable} disabled={busy || !disableCode.trim()}>{busy ? "…" : t("settings.disable2FA")}</Button>
          </div>
        )}
      </GlassCard>

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-foreground">{t("settings.activeSessions")}</h2>
          <Button variant="danger" onClick={logoutAll} disabled={busy} className="text-xs py-1 px-3">
            {t("settings.signOutOfAll")}
          </Button>
        </div>
        <p className="text-xs text-foreground/50">{t("settings.activeSessionsDesc")}</p>
        <SessionsList />
      </GlassCard>

      <GlassCard className="p-6 space-y-4">
        <h2 className="text-base font-bold text-foreground">{t("settings.linkedIdentities")}</h2>
        <p className="text-xs text-foreground/50">{t("settings.linkedIdentitiesDesc")}</p>
        <LinkedIdentities />
      </GlassCard>


    </div>
  );
}
