import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview } from "../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { PluginInfo } from "../api";
import LLMProvidersSettings from "./LLMProviders";

export default function SettingsPage() {
  return (
    <ScreenWrap>
      <Routes>
        <Route path="/" element={<Navigate to="/settings/general" replace />} />
        <Route path="/general" element={<GeneralSettings />} />
        <Route path="/plugins" element={<PluginsSettings />} />
        <Route path="/security" element={<SecuritySettings />} />
        <Route path="/webhooks" element={<WebhooksSettings />} />
        <Route path="/billing" element={<BillingPlanSettings />} />
        <Route path="/license" element={<LicenseSettings />} />
        <Route path="/notifications" element={<NotificationChannels />} />
        <Route path="/members" element={<OrgMembersManager />} />
      </Routes>
    </ScreenWrap>
  );
}

// PluginsSettings lets an owner/admin turn Pro plugins on or off for this
// workspace. Plugins are opt-in: everything is disabled until enabled here, and
// a disabled plugin's sidebar items and API routes are both hidden.
const PLUGIN_META: Record<string, { label: string; description: string }> = {
  ai: { label: "AI Inbox", description: "LLM-powered email summaries, sorting, and OTP extraction." },
  infra: { label: "Infrastructure", description: "VPS server monitoring and the SSH credentials vault." },
  finance: { label: "Bookkeeping", description: "Subscription tracking and expense bookkeeping." },
  product: { label: "Storefront", description: "Sell products — catalog, pricing tiers, and downloads." },
  billing: { label: "Billing", description: "Stripe / Polar checkout webhooks and price mapping." },
  issuer: { label: "License Issuance", description: "Cryptographic per-product license signing and registry." },
  portal: { label: "Customer Portal", description: "Self-serve portal for your customers' licenses and devices." },
};

function PluginsSettings() {
  const [plugins, setPlugins] = useState<PluginInfo[] | null>(null);
  const [err, setErr] = useState("");

  function load() {
    api.plugins().then(setPlugins).catch((e: ApiError) => setErr(e.message || "Failed to load plugins"));
  }
  useEffect(load, []);

  async function toggle(name: string, enabled: boolean) {
    setErr("");
    // optimistic; revert on failure
    setPlugins((prev) => prev?.map((p) => (p.name === name ? { ...p, enabled } : p)) ?? prev);
    try {
      await api.updatePlugin(name, enabled);
    } catch (e) {
      setPlugins((prev) => prev?.map((p) => (p.name === name ? { ...p, enabled: !enabled } : p)) ?? prev);
      setErr(e instanceof ApiError ? e.message : "Failed to update plugin");
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="Plugins" description="Enable the Pro features this workspace uses. Everything is off by default." />

      {err && (
        <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
          <ShieldAlert className="h-4 w-4 shrink-0" /><span>{err}</span>
        </div>
      )}

      {plugins === null ? (
        <GlassCard className="p-6 text-sm text-white/50">Loading plugins…</GlassCard>
      ) : plugins.length === 0 ? (
        <GlassCard className="p-6 text-sm text-white/55">
          No plugins are available in this build. Pro plugins ship with <span className="text-white/80">Octarq</span>.
        </GlassCard>
      ) : (
        <div className="space-y-3">
          {plugins.map((p) => {
            const meta = PLUGIN_META[p.name] ?? { label: p.name, description: "" };
            return (
              <GlassCard key={p.name} className="p-5 flex items-center justify-between gap-4">
                <div className="flex items-start gap-3 min-w-0">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-white/[0.04] text-indigo-300">
                    <Puzzle className="h-4.5 w-4.5" />
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <h3 className="text-sm font-bold text-white">{meta.label}</h3>
                      {p.enabled ? <Badge tone="green">On</Badge> : <Badge tone="neutral">Off</Badge>}
                    </div>
                    {meta.description && <p className="text-xs text-white/45 mt-0.5">{meta.description}</p>}
                    {p.menus.length > 0 && (
                      <p className="text-[10px] text-white/30 mt-1">
                        Adds: {p.menus.map((m) => m.label).join(" · ")}
                      </p>
                    )}
                  </div>
                </div>
                <Toggle on={p.enabled} onChange={(v) => toggle(p.name, v)} />
              </GlassCard>
            );
          })}
        </div>
      )}
    </div>
  );
}

// LicenseSettings is where a customer pastes the led-pro key they bought. The
// backing API is the led-pro `licensing` plugin; in the OSS build it 404s and we
// show a neutral note instead.
function LicenseSettings() {
  const [status, setStatus] = useState<LicenseStatus | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);

  function load() {
    api.license()
      .then(setStatus)
      .catch((e: ApiError) => {
        if (e.status === 404) setUnavailable(true);
        else setMsg({ kind: "err", text: e.message });
      });
  }
  useEffect(load, []);

  async function activate() {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api.activateLicense(token.trim());
      setToken("");
      setMsg({
        kind: "ok",
        text: r.envOverride
          ? `Saved a ${r.tier} license, but LED_PRO_LICENSE is set in the environment and takes precedence — unset it and restart to use this key.`
          : `Saved a ${r.tier} license for ${r.email}. Restart led to apply it.`,
      });
      load();
    } catch (e) {
      setMsg({ kind: "err", text: (e as ApiError).message });
    } finally {
      setBusy(false);
    }
  }

  async function deactivate() {
    if (!confirm("Remove the saved license? Pro features lock after the next restart.")) return;
    setBusy(true);
    setMsg(null);
    try {
      await api.deactivateLicense();
      setMsg({ kind: "ok", text: "License removed. Restart led to apply." });
      load();
    } catch (e) {
      setMsg({ kind: "err", text: (e as ApiError).message });
    } finally {
      setBusy(false);
    }
  }

  if (unavailable) {
    return (
      <div>
        <PageHeader title="License" description="Manage your led-pro license key" />
        <GlassCard className="p-6 text-sm text-white/55">
          This is the open-source build of led — there are no Pro features to license.
          Pro and Elite are part of <span className="text-white/80">Octarq</span>.{" "}
          <a className="text-indigo-300 hover:underline" href="https://octarq.com/pricing/" target="_blank" rel="noreferrer">
            See the plans →
          </a>
        </GlassCard>
      </div>
    );
  }

  return (
    <div>
      <PageHeader title="License" description="Activate Pro / Elite with the key you bought" />

      {status && (
        <GlassCard className="mb-4 p-5">
          <div className="flex items-center gap-3">
            <KeyRound className="h-5 w-5 text-indigo-300" />
            {status.licensed ? (
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone="green">{(status.tier || "").toUpperCase()}</Badge>
                <span className="text-sm text-white/70">{status.email}</span>
                <span className="text-xs text-white/40">
                  {status.expiresAt ? `expires ${status.expiresAt.slice(0, 10)}` : "never expires"} · from {status.source}
                </span>
              </div>
            ) : (
              <span className="text-sm text-white/60">No active license — Pro features are locked.</span>
            )}
          </div>
          {status.envOverride && (
            <p className="mt-3 text-xs text-amber-300/90">
              A license is set via the <code>LED_PRO_LICENSE</code> environment variable, which
              overrides any key saved here.
            </p>
          )}
        </GlassCard>
      )}

      <GlassCard className="p-5">
        <Field label="License key" hint="Paste the key from your purchase email or claim page.">
          <textarea
            className="input w-full font-mono text-xs"
            rows={4}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="eyJ…  .  MEUCIQ…"
          />
        </Field>
        <div className="mt-3 flex items-center gap-2">
          <Button variant="primary" onClick={activate} disabled={busy || token.trim() === ""}>
            {busy ? "Saving…" : "Activate"}
          </Button>
          {status?.licensed && status.source === "file" && (
            <Button variant="danger" onClick={deactivate} disabled={busy}>
              Remove license
            </Button>
          )}
        </div>
        {msg && (
          <p className={`mt-3 text-sm ${msg.kind === "ok" ? "text-emerald-300" : "text-rose-300"}`}>{msg.text}</p>
        )}
        <p className="mt-4 text-xs text-white/35">
          Changes take effect on the next restart — the server reads the license at startup.
        </p>
      </GlassCard>
    </div>
  );
}

// ── Settings module pages (split out of the old monolithic General Settings) ──

// useSettingsData loads the shared workspace settings object once.
function useSettingsData() {
  const [s, setS] = useState<SettingsData | null>(null);
  const reload = () => api.settings().then(setS);
  useEffect(() => { reload(); }, []);
  return { s, reload };
}

function SavedBadge({ on }: { on: boolean }) {
  return on ? <Badge tone="green">✓ Saved</Badge> : null;
}

function GeneralSettings() {
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspaceBusy, setWorkspaceBusy] = useState(false);
  const [workspaceSaved, setWorkspaceSaved] = useState(false);
  const [retention, setRetention] = useState(90);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  const [role, setRole] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const [purging, setPurging] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");

  useEffect(() => {
    api.me().then((me) => api.orgs().then((orgs) => {
      const o = orgs.find((x) => x.id === me.orgId);
      if (o) {
        setWorkspaceName(o.name);
        setRole(o.role || "member");
      }
    })).catch(() => {});
    api.settings().then((v) => {
      setRetention(v.dataRetentionDays ?? 90);
    });
  }, []);

  async function renameWorkspace(e: React.FormEvent) {
    e.preventDefault();
    if (!workspaceName.trim()) return;
    setWorkspaceBusy(true);
    try {
      await api.updateOrg({ name: workspaceName });
      setWorkspaceSaved(true);
      setTimeout(() => window.location.reload(), 800);
    } catch (err: any) { alert(err.message || "rename failed"); } finally { setWorkspaceBusy(false); }
  }

  async function save() {
    setBusy(true);
    try {
      await api.updateSettings({ dataRetentionDays: retention });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } finally { setBusy(false); }
  }

  async function handleExport() {
    setExporting(true);
    try {
      const data = await api.exportAccountData();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `workspace-data-${new Date().toISOString().split("T")[0]}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err: any) {
      alert(err.message || "Export failed");
    } finally {
      setExporting(false);
    }
  }

  async function handlePurge() {
    if (deleteConfirmationText !== "DELETE MY DATA") {
      alert("Please type 'DELETE MY DATA' exactly to confirm.");
      return;
    }
    setPurging(true);
    try {
      await api.purgeAccountData();
      alert("Your workspace and all of its data have been deleted.");
      setShowDeleteModal(false);
      setDeleteConfirmationText("");
      window.location.reload();
    } catch (err: any) {
      alert(err.message || "Couldn't delete the workspace. Please try again.");
    } finally {
      setPurging(false);
    }
  }

  const isAdminOrOwner = role === "admin" || role === "owner";

  return (
    <div className="space-y-6">
      <PageHeader title="General" description="Your workspace name, data settings, and privacy controls." />

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">Workspace Profile</h2>
          {workspaceSaved && <Badge tone="green">✓ Updated</Badge>}
        </div>
        <form onSubmit={renameWorkspace} className="max-w-md">
          <Field label="Workspace Name" hint="Shown in the workspace switcher and header.">
            <div className="flex gap-2">
              <input className="input flex-1 text-sm" value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)} placeholder="Acme Production" required />
              <Button type="submit" variant="primary" disabled={workspaceBusy || !workspaceName.trim()} className="shrink-0">
                {workspaceBusy ? "Updating…" : "Update"}
              </Button>
            </div>
          </Field>
        </form>
      </GlassCard>

      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">Data retention</h2>
          <SavedBadge on={saved} />
        </div>
        <Field label="Keep click history for (days)" hint="Older click history is removed automatically. Set to 0 to keep it forever.">
          <input type="number" min={0} className="input w-32 font-mono text-sm" value={retention} onChange={(e) => setRetention(Number(e.target.value))} />
        </Field>
        <div className="border-t border-white/[0.06] pt-6">
          <Button variant="primary" onClick={save} disabled={busy}>{busy ? "Saving…" : "Save"}</Button>
        </div>
      </GlassCard>

      {isAdminOrOwner && (
        <GlassCard className="p-6 space-y-4">
          <div>
            <h2 className="text-base font-bold text-white">Export Workspace Data</h2>
            <p className="text-xs text-white/50 mt-1">
              Download a complete copy of everything in this workspace including links, domains, mailboxes, and settings.
            </p>
          </div>
          <div className="pt-2">
            <Button variant="outline" onClick={handleExport} disabled={exporting}>
              {exporting ? "Preparing…" : "Download my data"}
            </Button>
          </div>
        </GlassCard>
      )}

      {isAdminOrOwner && (
        <>
          <GlassCard className="p-6 border-red-500/20 bg-red-950/5 space-y-6">
            <div className="flex items-center gap-2 text-rose-400">
              <ShieldAlert size={20} />
              <h2 className="text-base font-bold">Danger Zone</h2>
            </div>
            <p className="text-xs text-white/60">
              Permanently delete the workspace and all of its links, domains, mailboxes, and history.
              This action cannot be undone and will immediately delete all configuration data.
            </p>
            <div className="pt-2">
              <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
                Delete workspace
              </Button>
            </div>
          </GlassCard>
        </>
      )}

      {showDeleteModal && (
        <Modal title="Delete this workspace?" onClose={() => { setShowDeleteModal(false); setDeleteConfirmationText(""); }}>
          <div className="space-y-4">
            <p className="text-sm text-white/70">
              This permanently deletes the workspace and everything in it — links, domains, mailboxes, and history. This can't be undone.
            </p>
            <p className="text-sm text-white/70">
              Please type <span className="font-mono font-bold text-red-400 select-all">DELETE MY DATA</span> to confirm this action.
            </p>
            <input
              type="text"
              className="input w-full text-sm font-mono text-center border-red-500/30 focus:border-red-500/60"
              value={deleteConfirmationText}
              onChange={(e) => setDeleteConfirmationText(e.target.value)}
              placeholder="DELETE MY DATA"
            />
            <div className="flex justify-end gap-3 pt-2">
              <Button variant="ghost" onClick={() => { setShowDeleteModal(false); setDeleteConfirmationText(""); }}>
                Cancel
              </Button>
              <Button
                variant="danger"
                disabled={deleteConfirmationText !== "DELETE MY DATA" || purging}
                onClick={handlePurge}
              >
                {purging ? "Deleting…" : "Permanently delete"}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

// SecuritySettings manages operator-account security: TOTP two-factor
// enrollment, "log out everywhere" session revocation, and SSO configuration.

// Minimal UA parser — no external deps.
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
      ? "Log out from this device?"
      : "Revoke this session? That device will be signed out immediately.";
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
      alert(e.message || "Revoke failed");
    } finally {
      setRevoking(null);
    }
  }

  if (loading) return <div className="text-xs text-white/40 py-4 text-center">Loading sessions…</div>;
  if (sessions.length === 0) return <div className="text-xs text-white/40 py-4 text-center">No session records found.</div>;

  return (
    <div className="divide-y divide-white/[0.04] rounded-xl border border-white/[0.05] overflow-hidden">
      {sessions.map((s) => {
        const ua = parseUA(s.userAgent);
        return (
          <div key={s.id} className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium text-white/85">{ua.browser}</span>
                {s.isCurrent && <Badge tone="green">Current</Badge>}
                <span className="text-xs text-white/35">{ua.os}</span>
              </div>
              <div className="flex items-center gap-3 mt-1">
                <span className="text-xs text-white/40">{s.location || s.ip}</span>
                <span className="text-xs text-white/30">Last seen {timeAgo(s.lastSeenAt)}</span>
                <span className="text-xs text-white/25">Signed in {timeAgo(s.createdAt)}</span>
              </div>
            </div>
            <Button
              variant="danger"
              onClick={() => revoke(s.id, s.isCurrent)}
              disabled={revoking === s.id}
              className="text-xs py-1 px-2.5 shrink-0"
            >
              {revoking === s.id ? "…" : s.isCurrent ? "Log out" : "Revoke"}
            </Button>
          </div>
        );
      })}
    </div>
  );
}

function SecuritySettings() {
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
  const { s: ssoSettings, reload: ssoReload } = useSettingsData();
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
    try { await api.updateSettings({ allowRegistration: next }); ssoReload(); }
    catch { setAllowReg(!next); }
  }

  async function saveSso() {
    setSsoBusy(true);
    try {
      const p: any = { googleClientId: googleId.trim(), githubClientId: githubId.trim() };
      if (googleSecret.trim()) p.googleClientSecret = googleSecret.trim();
      if (githubSecret.trim()) p.githubClientSecret = githubSecret.trim();
      await api.updateSettings(p);
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
      setErr(e instanceof ApiError ? e.message : "failed to start setup");
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
      setErr(e instanceof ApiError ? e.message : "invalid code");
    } finally { setBusy(false); }
  }

  async function disable() {
    setBusy(true); setErr(""); setMsg("");
    try {
      await api.twoFADisable({ code: disableCode.trim() });
      setDisableCode("");
      setMsg("Two-factor authentication disabled.");
      await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : "verification failed");
    } finally { setBusy(false); }
  }

  async function logoutAll() {
    if (!confirm("Sign out of every device? You'll need to sign in again.")) return;
    setBusy(true); setErr("");
    try {
      await api.logoutAll();
      // The current session cookie is now revoked; bounce to the login screen.
      window.location.href = "/";
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : "failed");
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="Security" description="Two-factor authentication, session management, and Single Sign-On." />

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white flex items-center gap-2"><Shield className="h-4 w-4" /> Two-Factor Authentication (TOTP)</h2>
          <Badge tone={enabled ? "green" : "neutral"}>{enabled == null ? "…" : enabled ? "Enabled" : "Disabled"}</Badge>
        </div>
        <p className="text-xs text-white/50">Require a time-based one-time code from an authenticator app (Google Authenticator, 1Password, Authy) in addition to your password.</p>

        {err && <p className="text-sm text-rose-400">{err}</p>}
        {msg && <p className="text-sm text-emerald-400">{msg}</p>}

        {recoveryCodes && (
          <div className="rounded-xl border border-amber-400/30 bg-amber-400/[0.06] p-4">
            <p className="text-xs font-bold text-amber-300 mb-2">Save your recovery codes</p>
            <p className="text-[11px] text-white/50 mb-3">Each code can be used once if you lose your authenticator. They will not be shown again.</p>
            <div className="grid grid-cols-2 gap-1 font-mono text-xs text-white/80">
              {recoveryCodes.map((c) => <span key={c}>{c}</span>)}
            </div>
          </div>
        )}

        {!enabled && !setup && (
          <Button variant="primary" onClick={beginSetup} disabled={busy}>{busy ? "…" : "Enable 2FA"}</Button>
        )}

        {!enabled && setup && (
          <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
            <p className="text-xs text-white/60">Add this account to your authenticator app, then enter the 6-digit code to confirm.</p>
            <img
              alt="TOTP QR code"
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
                {ssoSettings?.googleClientSecretSet && <Button variant="danger" onClick={async () => { if (confirm("Clear Google secret?")) { await api.updateSettings({ googleClientSecret: "" }); ssoReload(); } }} className="px-2.5 py-1 text-xs">Clear</Button>}
              </div>
            </Field>
          </div>
          <div className="space-y-3 rounded-xl border border-white/[0.05] bg-black/20 p-4">
            <p className="flex items-center gap-1.5 text-xs font-bold text-white/85"><span className="h-1.5 w-1.5 rounded-full bg-indigo-400" /> GitHub Integration</p>
            <Field label="GitHub Client ID"><input className="input w-full text-xs" value={githubId} onChange={(e) => setGithubId(e.target.value)} placeholder="Ov23li…" /></Field>
            <Field label="GitHub Client Secret">
              <div className="flex gap-2">
                <input className="input w-full font-mono text-xs" type="password" value={githubSecret} onChange={(e) => setGithubSecret(e.target.value)} placeholder={ssoSettings?.githubClientSecretSet ? "•••••••• (Set)" : "Secret value"} />
                {ssoSettings?.githubClientSecretSet && <Button variant="danger" onClick={async () => { if (confirm("Clear GitHub secret?")) { await api.updateSettings({ githubClientSecret: "" }); ssoReload(); } }} className="px-2.5 py-1 text-xs">Clear</Button>}
              </div>
            </Field>
          </div>
        </div>
        <div className="border-t border-white/[0.06] pt-6"><Button variant="primary" onClick={saveSso} disabled={ssoBusy}>{ssoBusy ? "Saving…" : "Save"}</Button></div>
      </GlassCard>
    </div>
  );
}

export function LinkSettings() {
  const { s } = useSettingsData();
  const [reservedSlugs, setReservedSlugs] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => { if (s) { setReservedSlugs(s.reservedSlugs); } }, [s]);

  async function save() {
    setBusy(true);
    try { await api.updateSettings({ reservedSlugs }); setSaved(true); setTimeout(() => setSaved(false), 2000); }
    finally { setBusy(false); }
  }
  if (!s) return <div className="text-sm text-white/40">loading…</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white/90">Short Links Settings</h2>
        <SavedBadge on={saved} />
      </div>
      <Field label="Reserved Short Link Slugs" hint={`Slugs users cannot register. Built-in: ${s.builtinReserved.join(", ")}.`}>
        <textarea className="input w-full font-mono text-xs" rows={3} value={reservedSlugs} onChange={(e) => setReservedSlugs(e.target.value)} placeholder="pricing&#10;login&#10;about" />
      </Field>
      <div className="border-t border-white/[0.06] pt-4 flex justify-end">
        <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>{busy ? "Saving…" : "Save Settings"}</Button>
      </div>
    </div>
  );
}

export function MailSettings() {
  const { s } = useSettingsData();
  const [reservedMailboxes, setReservedMailboxes] = useState("");
  const [inboundToken, setInboundToken] = useState("");
  const [catchAll, setCatchAll] = useState(false);
  const [autoWrap, setAutoWrap] = useState(false);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => { if (s) { setReservedMailboxes(s.reservedMailboxes); setInboundToken(s.inboundToken || ""); setCatchAll(s.catchAll || false); setAutoWrap(s.autoWrapLinks || false); } }, [s]);

  async function save() {
    setBusy(true);
    try { await api.updateSettings({ reservedMailboxes, inboundToken, catchAll, autoWrapLinks: autoWrap }); setSaved(true); setTimeout(() => setSaved(false), 2000); }
    finally { setBusy(false); }
  }
  if (!s) return <div className="text-sm text-white/40">loading…</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white/90">Inbound Mailboxes Settings</h2>
        <SavedBadge on={saved} />
      </div>
      <Field label="Reserved Inbound Mailbox Prefixes" hint="Prefixes catch-all won't auto-provision (e.g. admin, postmaster).">
        <textarea className="input w-full font-mono text-xs" rows={2} value={reservedMailboxes} onChange={(e) => setReservedMailboxes(e.target.value)} placeholder="admin&#10;postmaster" />
      </Field>
      <Field label="Inbound Webhook URL" hint="Point the Cloudflare Email Worker at this exact URL — the token is in the path, so no header is needed.">
        <input
          readOnly
          className="input w-full font-mono text-xs"
          value={`${location.origin}/api/v1/webhook/${s?.orgSlug || ""}/email/inbound/${inboundToken}`}
          onFocus={(e) => e.currentTarget.select()}
        />
      </Field>
      <Field label="Inbound token" hint="Your workspace's secret, embedded in the URL above. Clear this box and Save to generate a new one (the old URL stops working).">
        <input className="input w-full font-mono text-xs" value={inboundToken} onChange={(e) => setInboundToken(e.target.value)} placeholder="(leave empty and save to generate a new one)" />
      </Field>
      <div className="flex items-center gap-3 border-t border-white/[0.04] pt-4">
        <Toggle on={catchAll} onChange={setCatchAll} />
        <div>
          <span className="block select-none text-xs font-semibold text-white/70">Enable Catch-All routing</span>
          <span className="select-none text-[10px] text-white/40">Auto-provision a local inbox when mail arrives for an unknown managed alias.</span>
        </div>
      </div>
      <div className="flex items-center gap-3 border-t border-white/[0.04] pt-4">
        <Toggle on={autoWrap} onChange={setAutoWrap} />
        <div>
          <span className="block select-none text-xs font-semibold text-white/70">Auto Wrap Outbound Links</span>
          <span className="select-none text-[10px] text-white/40">When sending mail, detect external URLs and wrap them as short links for click analytics.</span>
        </div>
      </div>
      <div className="border-t border-white/[0.06] pt-4 flex justify-end">
        <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>{busy ? "Saving…" : "Save Settings"}</Button>
      </div>
    </div>
  );
}


function WebhooksSettings() {
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [show, setShow] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [all, setAll] = useState(true);
  const [evClick, setEvClick] = useState(false);
  const [evEmail, setEvEmail] = useState(false);
  const [busy, setBusy] = useState(false);

  function load() { api.webhooks().then(setWebhooks).catch(() => {}); }
  useEffect(load, []);

  async function del(id: number) { if (!confirm("Delete this webhook endpoint?")) return; await api.deleteWebhook(id); setWebhooks((w) => w.filter((h) => h.id !== id)); }
  async function toggle(h: any) { const u = await api.updateWebhook(h.id, { enabled: !h.enabled }); setWebhooks((w) => w.map((x) => x.id === h.id ? u : x)); }
  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !url.trim()) return;
    setBusy(true);
    try {
      let events = "*";
      if (!all) { const l: string[] = []; if (evClick) l.push("link.click"); if (evEmail) l.push("email.receive"); events = l.join(",") || "*"; }
      const created = await api.createWebhook({ name: name.trim(), url: url.trim(), secret: secret.trim() || undefined, events, enabled: true } as any);
      setWebhooks((w) => [created, ...w]); setShow(false); setName(""); setUrl(""); setSecret("");
    } catch (err: any) { alert(err.message || "create failed"); } finally { setBusy(false); }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="Webhooks" description="Send click and email events to your own systems in real time. Every request is signed so you can verify it came from led." />
      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">Outbound Event Webhooks</h2>
          <Button variant="ghost" onClick={() => { setName(""); setUrl(""); setSecret(""); setAll(true); setEvClick(false); setEvEmail(false); setShow(true); }} className="flex items-center gap-1.5 px-3 py-1 text-xs">
            <Plus className="h-3 w-3" /> Add Webhook
          </Button>
        </div>
        {webhooks.length === 0 ? (
          <div className="select-none rounded border border-dashed border-white/[0.06] py-4 text-center text-xs text-white/40">No outbound webhooks configured.</div>
        ) : (
          <div className="space-y-3.5">
            {webhooks.map((w) => (
              <div key={w.id} className="flex flex-col justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-sm md:flex-row md:items-center">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-white/80">{w.name}</span>
                    <span className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono text-[9px] uppercase text-white/45">{w.events === "*" ? "all events" : w.events}</span>
                  </div>
                  <div className="select-all truncate font-mono text-xs text-white/45">{w.url}</div>
                  <div className="select-all font-mono text-[10px] text-zinc-500">Secret: {w.secret}</div>
                </div>
                <div className="flex shrink-0 items-center gap-3 self-end md:self-auto">
                  <Toggle on={w.enabled} onChange={() => toggle(w)} />
                  <Button variant="danger" onClick={() => del(w.id)} className="flex items-center gap-1 px-2.5 py-1 text-xs"><Trash2 className="h-3 w-3" /> Delete</Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      {show && (
        <Modal title="Add Webhook Endpoint" onClose={() => setShow(false)}>
          <form onSubmit={create} className="space-y-4">
            <Field label="Endpoint Name"><input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="n8n automation" required autoFocus /></Field>
            <Field label="Endpoint URL"><input className="input w-full font-mono text-xs" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://your-server.com/webhooks/led" required /></Field>
            <Field label="Signing Secret (Optional)" hint="Signs the payload in X-Led-Signature. Leave empty to auto-generate.">
              <input className="input w-full font-mono text-xs" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder="Custom signing secret" />
            </Field>
            <Field label="Event Subscriptions">
              <div className="mt-1 space-y-2">
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <input type="checkbox" checked={all} onChange={(e) => { setAll(e.target.checked); if (e.target.checked) { setEvClick(false); setEvEmail(false); } }} />
                  <span>All Events (*)</span>
                </label>
                {!all && (
                  <div className="space-y-2 pl-6">
                    <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300"><input type="checkbox" checked={evClick} onChange={(e) => setEvClick(e.target.checked)} /> <span>link.click</span></label>
                    <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300"><input type="checkbox" checked={evEmail} onChange={(e) => setEvEmail(e.target.checked)} /> <span>email.receive</span></label>
                  </div>
                )}
              </div>
            </Field>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setShow(false)}>Cancel</Button>
              <Button type="submit" variant="primary" disabled={busy}>{busy ? "Adding…" : "Add"}</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

function NotificationChannels() {
  const [channels, setChannels] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      setChannels(await api.notificationChannels());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    load();
  }, []);

  async function remove(id: number) {
    if (!confirm("Delete this notification channel?")) return;
    await api.deleteNotificationChannel(id);
    load();
  }

  async function test(id: number) {
    try {
      await api.testNotificationChannel(id);
      alert("Test alert sent successfully!");
    } catch (err: any) {
      alert("Test failed: " + err.message);
    }
  }

  async function toggleEnabled(c: any) {
    await api.updateNotificationChannel(c.id, { enabled: !c.enabled });
    load();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Alerts"
        description="Get notified in Telegram, Slack, or your own webhook when important events happen."
        action={
          <Button variant="primary" onClick={() => setEditing({ type: "telegram", config: "{}" })}>
            + Add Channel
          </Button>
        }
      />
      <GlassCard className="p-6">

      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">loading…</div>
      ) : channels.length === 0 ? (
        <Empty>
          <Bell className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No notification channels configured.</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {channels.map((c) => {
            const channelTypeTone = c.type === "telegram" ? "cyan" : "violet";

            return (
              <div key={c.id} className="flex items-center gap-3 p-4 group">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-semibold text-sm text-white">{c.name}</span>
                    <Badge tone={channelTypeTone} className="uppercase tracking-wider text-[9px]">
                      {c.type}
                    </Badge>
                    {!c.enabled && <Badge tone="neutral">disabled</Badge>}
                  </div>
                  <div className="text-[11px] text-white/35 mt-1">Added {timeAgo(c.createdAt)}</div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="subtle"
                    onClick={() => toggleEnabled(c)}
                    className="text-xs py-1 px-2.5"
                  >
                    {c.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button variant="outline" onClick={() => test(c.id)} className="text-xs py-1 px-2.5">
                    Test
                  </Button>
                  <Button variant="ghost" onClick={() => setEditing(c)} className="text-xs py-1 px-2.5">
                    <Pencil className="h-3 w-3" />
                  </Button>
                  <Button
                    variant="danger"
                    onClick={() => remove(c.id)}
                    className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {editing && (
        <EditNotificationChannel
          channel={editing.id ? editing : null}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
    </GlassCard>
    </div>
  );
}

function EditNotificationChannel({ channel, onClose, onSaved }: { channel: any; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(channel?.name || "");
  const [type, setType] = useState(channel?.type || "telegram");
  const [enabled, setEnabled] = useState(channel?.id ? channel.enabled : true);

  const initialCfg = channel?.id ? JSON.parse(channel.config) : {};
  const [botToken, setBotToken] = useState(initialCfg.botToken || "");
  const [chatId, setChatId] = useState(initialCfg.chatId || "");
  const [webhookUrl, setWebhookUrl] = useState(initialCfg.url || "");

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function save() {
    setBusy(true);
    setError("");
    let configStr = "{}";
    if (type === "telegram") {
      configStr = JSON.stringify({ botToken, chatId });
    } else if (type === "webhook") {
      configStr = JSON.stringify({ url: webhookUrl });
    }

    try {
      if (channel?.id) {
        await api.updateNotificationChannel(channel.id, {
          name,
          type,
          config: configStr,
          enabled,
        });
      } else {
        await api.createNotificationChannel({
          name,
          type,
          config: configStr,
          enabled,
        });
      }
      onSaved();
    } catch (err: any) {
      setError(err.message || "Failed to save");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={channel ? "Edit Alert Channel" : "Create Alert Channel"} onClose={onClose}>
      <form onSubmit={(e) => { e.preventDefault(); save(); }} className="space-y-4">
        <Field label="Channel Name" hint="A memorable identifier for this trigger">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Dev Team Slack"
            required
            autoFocus
          />
        </Field>
        
        <Field label="Channel Integration Type">
          <select className="input w-full" value={type} onChange={(e) => setType(e.target.value)}>
            <option value="telegram">Telegram Bot webhook</option>
            <option value="webhook">Custom HTTP POST Webhook</option>
          </select>
        </Field>

        {type === "telegram" && (
          <>
            <Field label="Bot Authentication Token" hint="Token issued by Telegram @BotFather">
              <input className="input w-full font-mono text-xs" value={botToken} onChange={(e) => setBotToken(e.target.value)} required />
            </Field>
            <Field label="Telegram Chat ID" hint="Channel group ID or user chat ID to forward alerts">
              <input className="input w-full font-mono text-xs" value={chatId} onChange={(e) => setChatId(e.target.value)} required />
            </Field>
          </>
        )}

        {type === "webhook" && (
          <Field label="Custom HTTP Target URL" hint="Receives JSON payload POST: { text: 'string' }">
            <input className="input w-full font-mono text-xs" value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} placeholder="https://my-webhook.com/alerts" required />
          </Field>
        )}

        {error && <div className="text-rose-400 text-xs font-semibold">{error}</div>}

        <div className="flex items-center gap-3 pt-2">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/60 select-none">Channel Enabled</span>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name}>
            {busy ? "Saving..." : "Save Channel"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function OrgMembersManager() {
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function load() {
    setLoading(true);
    try {
      setMembers(await api.orgMembers());
    } catch (e: any) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault();
    if (!email) return;
    setBusy(true);
    setErr("");
    try {
      await api.addOrgMember({ email, role });
      setEmail("");
      setRole("member");
      load();
    } catch (e: any) {
      setErr(e.message || "Failed to add member");
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove(userId: number) {
    if (!confirm("Remove this member from the workspace? They will lose access instantly.")) return;
    try {
      await api.deleteOrgMember(userId);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove member");
    }
  }

  const getRoleTone = (r: string) => {
    if (r === "owner") return "green";
    if (r === "admin") return "indigo";
    return "neutral";
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Workspace Members"
        description="Invite teammates and manage their roles in this workspace."
      />
      <GlassCard className="p-6 space-y-6">

      <form onSubmit={handleAdd} className="bg-black/25 p-4 rounded-xl border border-white/[0.05] flex flex-wrap sm:flex-nowrap gap-4 items-end">
        <div className="flex-1 min-w-[200px]">
          <label className="label text-xs">Invite Colleague by Email</label>
          <input
            className="input w-full text-sm mt-1"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            required
          />
        </div>
        <div className="w-32">
          <label className="label text-xs">Access Role</label>
          <select className="input w-full text-xs mt-1" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
            <option value="owner">Owner</option>
          </select>
        </div>
        <Button variant="primary" className="py-2 text-xs shrink-0" disabled={busy || !email}>
          {busy ? "Inviting..." : "Invite Member"}
        </Button>
      </form>
      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}

      {loading ? (
        <div className="text-white/40 text-sm py-4 text-center">loading members list…</div>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {(members || []).map((m) => (
            <div key={m.userId} className="flex justify-between items-center p-4">
              <div className="flex items-center gap-2.5">
                <span className="font-semibold text-sm text-white">{m.email}</span>
                <Badge tone={getRoleTone(m.role)} className="capitalize text-[10px] tracking-wide font-semibold px-2">
                  {m.role}
                </Badge>
              </div>
              <Button
                variant="danger"
                onClick={() => handleRemove(m.userId)}
                className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
              >
                Remove
              </Button>
            </div>
          ))}
        </div>
      )}
    </GlassCard>
    </div>
  );
}

// GlobalCloudflareToken is the fallback Cloudflare API token used by sync when a
// domain has no dedicated credentials. It lived in the old General Settings; it
// belongs with DNS Providers.
function GlobalCloudflareToken() {
  const { s, reload } = useSettingsData();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  if (!s) return null;

  async function save() {
    if (!token.trim()) return;
    setBusy(true);
    try { await api.updateSettings({ cloudflareToken: token.trim() }); setToken(""); setSaved(true); setTimeout(() => setSaved(false), 2000); reload(); }
    finally { setBusy(false); }
  }

  return (
    <GlassCard className="mb-4 p-6 space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-white/80">Global Cloudflare API Token</h3>
        <SavedBadge on={saved} />
      </div>
      <Field label="API token" hint={s.cloudflareTokenSet ? "A global token is set. Enter a new one to overwrite." : "Fallback token for sync when a domain has no dedicated key. Needs Zone:Read + DNS:Edit."}>
        <div className="flex gap-2">
          <input type="password" className="input w-full font-mono text-xs" value={token} onChange={(e) => setToken(e.target.value)} placeholder={s.cloudflareTokenSet ? "•••••••• (set)" : "Cloudflare API token"} />
          <Button variant="primary" onClick={save} disabled={busy || !token.trim()}>{busy ? "Saving…" : "Save"}</Button>
          {s.cloudflareTokenSet && <Button variant="danger" onClick={async () => { if (confirm("Clear stored token?")) { await api.updateSettings({ cloudflareToken: "" }); reload(); } }} className="px-3 py-1 text-xs">Clear</Button>}
        </div>
      </Field>
    </GlassCard>
  );
}

export function ProviderAccounts({ embed }: { embed?: boolean }) {
  const [accounts, setAccounts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      setAccounts(await api.providerAccounts());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function remove(id: number) {
    if (!confirm("Remove this provider account? Managed domains using these credentials will fail sync.")) return;
    try {
      await api.deleteProviderAccount(id);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove");
    }
  }


  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-white/70">
          DNS API Accounts
          <div className="text-[10px] text-white/35 font-normal mt-0.5">DNS provider API keys (Cloudflare/DNSPod) to verify domains.</div>
        </div>
        <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setCreating(true)}>
          + Add Provider
        </Button>
      </div>

      <GlobalCloudflareToken />

      {loading ? (
        <div className="text-white/40 text-sm py-4 text-center">loading…</div>
      ) : accounts.length === 0 ? (
        <Empty>
          <Cloud className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No DNS providers configured yet.</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {accounts.map(a => (
            <div key={a.id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-semibold text-sm text-white">{a.name}</div>
                <div className="text-xs text-white/40 mt-1">
                  <Badge tone={a.type === "cloudflare" ? "indigo" : "cyan"} className="uppercase tracking-wider text-[9px]">
                    {a.type}
                  </Badge>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="subtle" onClick={() => setEditing(a)} className="text-xs py-1 px-2.5">
                  Edit
                </Button>
                <Button
                  variant="danger"
                  onClick={() => remove(a.id)}
                  className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      {(creating || editing) && (
        <ProviderAccountModal
          account={editing}
          onClose={() => { setCreating(false); setEditing(null); }}
          onSaved={() => { setCreating(false); setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function ProviderAccountModal({ account, onClose, onSaved }: { account: any; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(account?.name || "");
  const [type, setType] = useState(account?.type || "cloudflare");
  const [config, setConfig] = useState<string>("");
  const [types, setTypes] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.dnsProviders().then(setTypes).catch(() => setTypes(["cloudflare", "dnspod"]));
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setErr("");
    try {
      let cfgObj: any = {};
      if (config.trim()) {
        try { cfgObj = JSON.parse(config.trim()); }
        catch { cfgObj = type === "cloudflare" ? { apiToken: config.trim() } : { token: config.trim() }; }
      }
      if (account) {
        await api.updateProviderAccount(account.id, { name, type, config: cfgObj });
      } else {
        await api.createProviderAccount({ name, type, config: cfgObj });
      }
      onSaved();
    } catch (e: any) {
      setErr(e.message || "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={account ? "Edit Provider Account" : "Register Provider Account"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Provider Label Name">
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Acme Production DNS" required autoFocus />
        </Field>
        {!account && (
          <Field label="DNS Provider Type">
            <select className="input w-full text-sm" value={type} onChange={e => setType(e.target.value)}>
              {types.map(t => <option key={t} value={t} className="capitalize">{t}</option>)}
            </select>
          </Field>
        )}
        <Field label="API Keys / Credentials" hint={account ? "Leave blank to keep existing keys. Cloudflare takes Token. DNSPod takes 'ID,Token'." : "For Cloudflare, paste API Token. For DNSPod, paste 'ID,Token' format."}>
          <input className="input w-full font-mono text-xs" type="password" value={config} onChange={e => setConfig(e.target.value)} placeholder={account ? "••••••••" : "API Token / Key string..."} required={!account} />
        </Field>
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? "Saving..." : "Save Connection"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export function SMTPSenders() {
  const [senders, setSenders] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      setSenders(await api.smtpSenders());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function remove(id: number) {
    if (!confirm("Remove this SMTP sender config? Outbound mail relying on it will fail to send.")) return;
    try {
      await api.deleteSMTPSender(id);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove");
    }
  }


  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-white/70">
          SMTP Outgoing Gateways
          <div className="text-[10px] text-white/35 font-normal mt-0.5">SMTP servers used to send emails from your mailboxes.</div>
        </div>
        <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setCreating(true)}>
          + Add SMTP
        </Button>
      </div>
      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">loading…</div>
      ) : senders.length === 0 ? (
        <Empty>
          <Send className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No SMTP outgoing senders configured yet.</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {senders.map(s => (
            <div key={s.id} className="flex items-center justify-between p-4 group">
              <div>
                <div className="font-semibold text-sm text-white">{s.name}</div>
                <div className="text-xs text-white/40 mt-1 font-mono">
                  {s.fromEmail} via {s.host}:{s.port}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="subtle" onClick={() => setEditing(s)} className="text-xs py-1 px-2.5">
                  Edit SMTP
                </Button>
                <Button
                  variant="danger"
                  onClick={() => remove(s.id)}
                  className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      {(creating || editing) && (
        <SMTPSenderModal
          sender={editing}
          onClose={() => { setCreating(false); setEditing(null); }}
          onSaved={() => { setCreating(false); setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function SMTPSenderModal({ sender, onClose, onSaved }: { sender: any; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(sender?.name || "");
  const [host, setHost] = useState(sender?.host || "");
  const [port, setPort] = useState(sender?.port?.toString() || "");
  const [user, setUser] = useState(sender?.user || "");
  const [pass, setPass] = useState("");
  const [fromEmail, setFromEmail] = useState(sender?.fromEmail || "");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setErr("");
    try {
      const payload = { name, host, port: parseInt(port, 10), user, pass, fromEmail };
      if (sender) {
        await api.updateSMTPSender(sender.id, payload);
      } else {
        await api.createSMTPSender(payload);
      }
      onSaved();
    } catch (e: any) {
      setErr(e.message || "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={sender ? "Modify SMTP Relay" : "Configure SMTP Relay"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Sender Connection Name">
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Corporate SMTP" required autoFocus />
        </Field>
        
        <div className="flex gap-4">
          <div className="flex-[3]">
            <Field label="SMTP Host Server address">
              <input className="input w-full font-mono text-xs" value={host} onChange={e => setHost(e.target.value)} placeholder="smtp.mailgun.org" required />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="SMTP Port">
              <input type="number" className="input w-full font-mono text-xs" value={port} onChange={e => setPort(e.target.value)} placeholder="587" required />
            </Field>
          </div>
        </div>

        <Field label="SMTP Connection Username">
          <input className="input w-full font-mono text-xs" value={user} onChange={e => setUser(e.target.value)} placeholder="e.g. postmaster@domain.com" required />
        </Field>
        
        <Field label="SMTP Connection Password" hint={sender ? "Leave blank to preserve current secure key." : ""}>
          <input type="password" className="input w-full font-mono text-xs" value={pass} onChange={e => setPass(e.target.value)} placeholder="••••••••" required={!sender} />
        </Field>
        
        <Field label="Default From Address" hint="Outgoing address used if none specified.">
          <input className="input w-full font-mono text-xs" value={fromEmail} onChange={e => setFromEmail(e.target.value)} placeholder="noreply@domain.com" required />
        </Field>
        
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim() || !host.trim() || !port || !user.trim()}>
            {busy ? "Saving..." : "Save Relay"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function BillingPlanSettings() {
  const [status, setStatus] = useState<LicenseStatus | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    api.license()
      .then(setStatus)
      .catch((e: ApiError) => {
        if (e.status === 404) setUnavailable(true);
      });
    api.overview(false)
      .then(setOverview)
      .catch(() => {});
  }, []);

  const plans = [
    {
      name: "Starter (OSS)",
      price: "$0",
      period: "forever",
      description: "Ideal for personal sites, hobby projects, and open-source hosting.",
      features: [
        "Core Domain Mapping",
        "Unlimited Redirection Links",
        "Basic Click Analytics",
        "Standard Email Routing",
        "Community Support",
      ],
      current: unavailable || !status?.licensed,
    },
    {
      name: "Octarq Pro",
      price: "$29",
      period: "month",
      description: "Perfect for growing projects, creators, and commercial workloads.",
      features: [
        "Everything in Starter",
        "Direct VPS Control Panel",
        "Secure SSH Credentials Vault",
        "Outbound SMTP Relay",
        "Storefront Product Catalog",
        "Cryptographic License Issuance",
      ],
      current: !unavailable && status?.licensed && status.tier?.toLowerCase() === "pro",
      popular: true,
    },
    {
      name: "Octarq Elite",
      price: "$99",
      period: "month",
      description: "Full AI automation, dedicated resources, and advanced compliance.",
      features: [
        "Everything in Pro",
        "AI Inbox Automation & Summaries",
        "Semantic OTP Code Routing",
        "Multiple LLM Provider Keys",
        "Comprehensive Audit Logging",
        "Priority Support Escalation",
      ],
      current: !unavailable && status?.licensed && status.tier?.toLowerCase() === "elite",
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Billing & Plan"
        description="Monitor your subscription package, license details, and usage metrics."
      />

      {/* Active plan card & metrics */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Active Plan</span>
            {unavailable ? (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2">
                  Open Source
                  <Badge tone="neutral">OSS Build</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1">No Pro/Elite license active</p>
              </div>
            ) : status?.licensed ? (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2 capitalize">
                  {status.tier} Tier
                  <Badge tone="green">Active</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1 truncate" title={status.email}>
                  Licensed to {status.email}
                </p>
                <p className="text-[10px] text-white/40 mt-0.5">
                  {status.expiresAt ? `Expires ${status.expiresAt.slice(0, 10)}` : "Lifetime / Never expires"}
                </p>
              </div>
            ) : (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2">
                  Unlicensed
                  <Badge tone="red">Locked</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1">Activate a license key to unlock features</p>
              </div>
            )}
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4 flex flex-col gap-2">
            {!unavailable && status?.licensed ? (
              <a
                href="https://app.octarq.com"
                target="_blank"
                rel="noreferrer"
                className="w-full text-center text-xs font-semibold py-2 px-3 rounded-xl bg-indigo-500 hover:bg-indigo-600 text-white transition-colors"
              >
                Manage Subscription on Octarq
              </a>
            ) : (
              <a
                href="https://octarq.com/pricing/"
                target="_blank"
                rel="noreferrer"
                className="w-full text-center text-xs font-semibold py-2 px-3 rounded-xl bg-white/10 hover:bg-white/15 text-white transition-colors"
              >
                View Premium Plans
              </a>
            )}
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Redirection Links</span>
            <h3 className="text-xl font-bold text-white">
              {overview ? overview.links : "—"} Links
            </h3>
            <p className="text-xs text-white/50 mt-1">
              {overview ? `${overview.activeLinks} active redirects` : "Loading metrics…"}
            </p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> No limits applied
            </span>
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Managed Domains</span>
            <h3 className="text-xl font-bold text-white">
              {overview ? overview.domains : "—"} Domains
            </h3>
            <p className="text-xs text-white/50 mt-1">
              {overview ? `${overview.linkDomains} for links · ${overview.mailDomains} for mail` : "Loading metrics…"}
            </p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> Unlimited domains
            </span>
          </div>
        </GlassCard>
      </div>

      {/* Plans comparison */}
      <div className="space-y-4">
        <div>
          <h3 className="text-base font-bold text-white">Octarq Plan Comparison</h3>
          <p className="text-xs text-white/50">Compare feature availability across tiers in the self-hosted environment.</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {plans.map((p) => (
            <GlassCard key={p.name} className={`p-5 flex flex-col justify-between border-t-2 relative ${p.current ? 'border-t-indigo-500 bg-indigo-500/[0.02]' : 'border-t-white/10'}`}>
              {p.popular && (
                <span className="absolute -top-3 right-4 bg-indigo-500 text-white text-[9px] font-bold uppercase px-2 py-0.5 rounded-full shadow-glow">
                  Popular
                </span>
              )}
              <div className="space-y-4">
                <div>
                  <h4 className="font-bold text-white">{p.name}</h4>
                  <p className="text-xs text-white/40 mt-1 min-h-[32px]">{p.description}</p>
                </div>
                <div className="flex items-baseline gap-1">
                  <span className="text-3xl font-extrabold text-white">{p.price}</span>
                  <span className="text-xs text-white/40">/ {p.period}</span>
                </div>
                <ul className="space-y-2 border-t border-white/[0.06] pt-4">
                  {p.features.map((f, i) => (
                    <li key={i} className="text-xs text-white/70 flex items-center gap-2">
                      <span className="h-1 w-1 rounded-full bg-indigo-400" />
                      {f}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="mt-6 pt-2">
                {p.current ? (
                  <Button variant="subtle" className="w-full text-xs cursor-default" disabled>
                    Current Plan
                  </Button>
                ) : (
                  <a
                    href={`https://octarq.com/pricing/${
                      p.name.toLowerCase().includes("elite")
                        ? "?plan=elite"
                        : p.name.toLowerCase().includes("pro")
                        ? "?plan=pro"
                        : ""
                    }`}
                    target="_blank"
                    rel="noreferrer"
                    className={`block w-full text-center text-xs font-semibold py-2 px-3 rounded-xl transition-colors ${
                      p.popular
                        ? "bg-indigo-500 hover:bg-indigo-600 text-white"
                        : "bg-transparent border border-white/20 hover:bg-white/5 text-white"
                    }`}
                  >
                    {p.price === "$0" ? "Downgrade" : "Upgrade Plan"}
                  </a>
                )}
              </div>
            </GlassCard>
          ))}
        </div>
      </div>
    </div>
  );
}
