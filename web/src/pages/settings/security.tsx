import {
  Alert,
  Badge,
  Button,
  Field,
  Input,
  PageCard,
  PageRow,
  PageRows,
  PageSection,
  PageShell,
  Separator,
  confirmDialog,
  confirmPassword,
  timeAgo,
  toast,
} from "@octarq/plugin-sdk";
import { Fragment, useEffect, useState } from "react";
import { api, ApiError, type LinkedIdentity } from "../../api";
import { useTranslation } from "../../i18n";

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

  if (loading) return <PageCard><div className="text-xs text-muted-foreground py-4 text-center">{t("settings.loadingSessions")}</div></PageCard>;
  if (sessions.length === 0) return <PageCard><div className="text-xs text-muted-foreground py-4 text-center">{t("settings.noSessions")}</div></PageCard>;

  return (
    <PageRows>
      {sessions.map((s, idx) => {
        const ua = parseUA(s.userAgent);
        const browserName =
          ua.browserKey === "uaUnknown"
            ? t("settings.uaUnknown")
            : ua.browserKey === "uaBrowser"
              ? t("settings.uaBrowser")
              : ua.browser;
        return (
          <Fragment key={s.id}>
            {idx > 0 && <Separator />}
            <PageRow
              title={
                <span className="flex items-center gap-2 flex-wrap">
                  <span>{browserName}</span>
                  {s.isCurrent && <Badge tone="green">{t("settings.current")}</Badge>}
                  {ua.os && <span className="text-xs font-normal text-muted-foreground">{ua.os}</span>}
                </span>
              }
              description={
                <span className="flex flex-wrap items-center gap-3 font-mono tnum">
                  <span>{s.location || s.ip}</span>
                  <span>{t("settings.lastSeen", { time: timeAgo(s.lastSeenAt) })}</span>
                  <span>{t("settings.signedIn", { time: timeAgo(s.createdAt) })}</span>
                </span>
              }
              control={
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => revoke(s.id, s.isCurrent)}
                  disabled={revoking === s.id}
                >
                  {revoking === s.id ? "…" : s.isCurrent ? t("settings.logOut") : t("settings.revoke")}
                </Button>
              }
            />
          </Fragment>
        );
      })}
    </PageRows>
  );
}

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
      toast.error(e instanceof ApiError ? e.message : t("settings.failed"));
    } finally {
      setBusy(null);
    }
  }

  if (items === null) return <PageCard><div className="text-xs text-muted-foreground py-4 text-center">{t("settings.loadingIdentities")}</div></PageCard>;
  if (items.length === 0) return <PageCard><div className="text-xs text-muted-foreground py-4 text-center">{t("settings.noLinkedIdentities")}</div></PageCard>;

  return (
    <PageRows>
      {items.map((id, idx) => (
        <Fragment key={id.id}>
          {idx > 0 && <Separator />}
          <PageRow
            title={id.issuer}
            description={
              <span className="flex flex-wrap items-center gap-3 font-mono tnum">
                <span>{id.email}</span>
                <span>{t("settings.linkedAt", { time: timeAgo(id.createdAt) })}</span>
              </span>
            }
            control={
              <Button
                variant="danger"
                size="sm"
                onClick={() => unlink(id)}
                disabled={busy === id.id}
              >
                {busy === id.id ? "…" : t("settings.unlink")}
              </Button>
            }
          />
        </Fragment>
      ))}
    </PageRows>
  );
}

export function SecuritySettings() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");

  const [hasOauth, setHasOauth] = useState(false);
  const [hasSso, setHasSso] = useState(false);

  const [setup, setSetup] = useState<{ secret: string; otpauthUrl: string; qrDataUri?: string } | null>(null);
  const [enrollCode, setEnrollCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);

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
    fetch("/api/auth/methods")
      .then((r) => r.json())
      .then((m: { id: string }[]) =>
        setHasSso(m.some((x) => x.id === "sso" || x.id.startsWith("oidc"))),
      )
      .catch(() => {});
  }, []);

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
      window.location.href = "/";
    } catch (e: any) {
      const msg = e instanceof ApiError ? e.message : t("settings.failed");
      setErr(msg);
      toast.error(msg);
      setBusy(false);
    }
  }

  return (
    <PageShell title={t("settings.securityTitle")} description={t("settings.securityDescription")}>
      <PageSection
        title={t("settings.twoFATitle")}
        description={t("settings.twoFADesc")}
        action={<Badge tone={enabled ? "green" : "neutral"}>{enabled == null ? "…" : enabled ? t("settings.enabled") : t("settings.disabled")}</Badge>}
      >
        <PageCard className="space-y-4">
          {enabled && (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t("settings.twoFACoverageBase")}</p>
              {hasOauth && <p className="text-xs text-muted-foreground">{t("settings.twoFACoverageOauth")}</p>}
              {hasSso && <p className="text-xs text-muted-foreground">{t("settings.twoFACoverageSso")}</p>}
            </div>
          )}

          {err && <p className="text-sm text-danger-fg">{err}</p>}
          {msg && <p className="text-sm text-success-fg">{msg}</p>}

          {recoveryCodes && (
            <Alert variant="warning" className="p-4">
              <p className="text-xs font-bold text-warning-fg mb-2">{t("settings.saveRecoveryCodes")}</p>
              <p className="text-[11px] text-muted-foreground mb-3">{t("settings.recoveryCodesDesc")}</p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5 font-mono text-xs text-foreground">
                {recoveryCodes.map((c) => <span key={c} className="select-all whitespace-nowrap">{c}</span>)}
              </div>
            </Alert>
          )}

          {!enabled && !setup && (
            <Button variant="primary" size="sm" onClick={beginSetup} disabled={busy}>{busy ? "…" : t("settings.enable2FA")}</Button>
          )}

          {!enabled && setup && (
            <div className="space-y-3 rounded-lg border border-border bg-well p-4">
              <p className="text-xs text-muted-foreground">{t("settings.scanInstructions")}</p>
              <img
                alt={t("settings.qrAlt")}
                className="rounded-lg bg-white p-2"
                width={160}
                height={160}
                src={setup.qrDataUri}
              />
              <Field label={t("settings.setupKeyLabel")}>
                <Input className="font-mono text-xs" readOnly value={setup.secret} />
              </Field>
              <a className="block break-all text-[10px] text-accent-fg/70 hover:underline" href={setup.otpauthUrl}>{setup.otpauthUrl}</a>
              <Field label={t("settings.verificationCode")}>
                <Input className="text-sm" value={enrollCode} onChange={(e) => setEnrollCode(e.target.value)} placeholder="123456" autoComplete="one-time-code" />
              </Field>
              <div className="flex gap-2">
                <Button variant="primary" size="sm" onClick={confirmEnable} disabled={busy || !enrollCode.trim()}>{busy ? "…" : t("settings.confirmEnable")}</Button>
                <Button variant="ghost" size="sm" onClick={() => { setSetup(null); setEnrollCode(""); }}>{t("settings.cancel")}</Button>
              </div>
            </div>
          )}

          {enabled && (
            <div className="space-y-3 rounded-lg border border-border bg-well p-4">
              <p className="text-xs text-muted-foreground">{t("settings.disableInstructions")}</p>
              <Field label={t("settings.verificationCode")}>
                <Input className="text-sm" value={disableCode} onChange={(e) => setDisableCode(e.target.value)} placeholder={t("settings.disableCodePlaceholder")} autoComplete="one-time-code" />
              </Field>
              <Button variant="danger" size="sm" onClick={disable} disabled={busy || !disableCode.trim()}>{busy ? "…" : t("settings.disable2FA")}</Button>
            </div>
          )}
        </PageCard>
      </PageSection>

      <PageSection
        title={t("settings.activeSessions")}
        description={t("settings.activeSessionsDesc")}
        action={
          <Button variant="danger" size="sm" onClick={logoutAll} disabled={busy}>
            {t("settings.signOutOfAll")}
          </Button>
        }
      >
        <SessionsList />
      </PageSection>

      <PageSection title={t("settings.linkedIdentities")} description={t("settings.linkedIdentitiesDesc")}>
        <LinkedIdentities />
      </PageSection>
    </PageShell>
  );
}
