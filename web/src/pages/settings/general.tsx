import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

export function GeneralSettings() {
  const { t } = useTranslation();
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspaceBusy, setWorkspaceBusy] = useState(false);
  const [workspaceSaved, setWorkspaceSaved] = useState(false);
  const [retention, setRetention] = useState(90);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  const [appName, setAppName] = useState("");
  const [rlAuth, setRlAuth] = useState(60);
  const [rlApi, setRlApi] = useState(600);
  const [rlRedirect, setRlRedirect] = useState(6000);
  const [metricsToken, setMetricsToken] = useState("");
  const [metricsTokenSet, setMetricsTokenSet] = useState(false);
  const [instanceBusy, setInstanceBusy] = useState(false);
  const [instanceSaved, setInstanceSaved] = useState(false);

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
      setAppName(v.appName ?? "");
      setRlAuth(v.ratelimitAuthRpm ?? 60);
      setRlApi(v.ratelimitApiRpm ?? 600);
      setRlRedirect(v.ratelimitRedirectRpm ?? 6000);
      setMetricsTokenSet(v.metricsTokenSet ?? false);
    });
  }, []);

  async function saveInstance(extra: { metricsToken?: string } = {}) {
    setInstanceBusy(true);
    try {
      const payload: Parameters<typeof api.updateSettings>[0] = {
        appName,
        ratelimitAuthRpm: rlAuth,
        ratelimitApiRpm: rlApi,
        ratelimitRedirectRpm: rlRedirect,
        ...( "metricsToken" in extra ? extra : metricsToken ? { metricsToken } : {}),
      };
      const v = await api.updateSettings(payload);
      setMetricsTokenSet(v.metricsTokenSet);
      setMetricsToken("");
      setInstanceSaved(true);
      setTimeout(() => setInstanceSaved(false), 2000);
    } finally {
      setInstanceBusy(false);
    }
  }

  async function renameWorkspace(e: React.FormEvent) {
    e.preventDefault();
    if (!workspaceName.trim()) return;
    setWorkspaceBusy(true);
    try {
      await api.updateOrg({ name: workspaceName });
      setWorkspaceSaved(true);
      setTimeout(() => window.location.reload(), 800);
    } catch (err: any) { alert(err.message || t("settings.renameFailed")); } finally { setWorkspaceBusy(false); }
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
      alert(err.message || t("settings.exportFailed"));
    } finally {
      setExporting(false);
    }
  }

  async function handlePurge() {
    if (deleteConfirmationText !== "DELETE MY DATA") {
      alert(t("settings.typeToConfirm", { phrase: "DELETE MY DATA" }));
      return;
    }
    setPurging(true);
    try {
      await api.purgeAccountData();
      alert(t("settings.workspaceDeleted"));
      setShowDeleteModal(false);
      setDeleteConfirmationText("");
      window.location.reload();
    } catch (err: any) {
      alert(err.message || t("settings.deleteWorkspaceFailed"));
    } finally {
      setPurging(false);
    }
  }

  const isAdminOrOwner = role === "admin" || role === "owner";

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.generalTitle")} description={t("settings.generalDescription")} />

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">{t("settings.workspaceProfile")}</h2>
          {workspaceSaved && <Badge tone="green">{t("settings.updated")}</Badge>}
        </div>
        <form onSubmit={renameWorkspace} className="max-w-md">
          <Field label={t("settings.workspaceNameLabel")} hint={t("settings.workspaceNameHint")}>
            <div className="flex gap-2">
              <input className="input flex-1 text-sm" value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)} placeholder="Acme Production" required />
              <Button type="submit" variant="primary" disabled={workspaceBusy || !workspaceName.trim()} className="shrink-0">
                {workspaceBusy ? t("settings.updating") : t("settings.update")}
              </Button>
            </div>
          </Field>
        </form>
      </GlassCard>

      <GlassCard className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">{t("settings.dataRetention")}</h2>
          <SavedBadge on={saved} />
        </div>
        <Field label={t("settings.retentionLabel")} hint={t("settings.retentionHint")}>
          <input type="number" min={0} className="input w-32 font-mono text-sm" value={retention} onChange={(e) => setRetention(Number(e.target.value))} />
        </Field>
        <div className="border-t border-white/[0.06] pt-6">
          <Button variant="primary" onClick={save} disabled={busy}>{busy ? t("settings.saving") : t("settings.save")}</Button>
        </div>
      </GlassCard>

      {isAdminOrOwner && (
        <GlassCard className="p-6 space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-base font-bold text-white">{t("settings.instanceTitle")}</h2>
              <p className="text-xs text-white/50 mt-1">{t("settings.instanceDesc")}</p>
            </div>
            <SavedBadge on={instanceSaved} />
          </div>
          <Field label={t("settings.instanceAppName")} hint={t("settings.instanceAppNameHint")}>
            <input className="input w-full max-w-md text-sm" value={appName} onChange={(e) => setAppName(e.target.value)} placeholder="led" />
          </Field>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 max-w-2xl">
            <Field label={t("settings.instanceRlAuth")} hint={t("settings.instanceRlHint")}>
              <input type="number" min={0} className="input w-full font-mono text-sm" value={rlAuth} onChange={(e) => setRlAuth(Number(e.target.value))} />
            </Field>
            <Field label={t("settings.instanceRlApi")}>
              <input type="number" min={0} className="input w-full font-mono text-sm" value={rlApi} onChange={(e) => setRlApi(Number(e.target.value))} />
            </Field>
            <Field label={t("settings.instanceRlRedirect")}>
              <input type="number" min={0} className="input w-full font-mono text-sm" value={rlRedirect} onChange={(e) => setRlRedirect(Number(e.target.value))} />
            </Field>
          </div>
          <Field
            label={t("settings.instanceMetricsToken")}
            hint={metricsTokenSet ? t("settings.instanceMetricsTokenSetHint") : t("settings.instanceMetricsTokenHint")}
          >
            <div className="flex gap-2 max-w-md">
              <input className="input w-full font-mono text-sm" type="password" value={metricsToken} onChange={(e) => setMetricsToken(e.target.value)} placeholder={metricsTokenSet ? "••••••••" : ""} />
              {metricsTokenSet && (
                <Button variant="ghost" className="shrink-0 text-xs" onClick={() => saveInstance({ metricsToken: "" })} disabled={instanceBusy}>
                  {t("settings.instanceMetricsClear")}
                </Button>
              )}
            </div>
          </Field>
          <div className="border-t border-white/[0.06] pt-6">
            <Button variant="primary" onClick={() => saveInstance()} disabled={instanceBusy}>
              {instanceBusy ? t("settings.saving") : t("settings.save")}
            </Button>
          </div>
        </GlassCard>
      )}

      {isAdminOrOwner && (
        <GlassCard className="p-6 space-y-4">
          <div>
            <h2 className="text-base font-bold text-white">{t("settings.exportWorkspaceData")}</h2>
            <p className="text-xs text-white/50 mt-1">
              {t("settings.exportWorkspaceDesc")}
            </p>
          </div>
          <div className="pt-2">
            <Button variant="outline" onClick={handleExport} disabled={exporting}>
              {exporting ? t("settings.preparing") : t("settings.downloadMyData")}
            </Button>
          </div>
        </GlassCard>
      )}

      {isAdminOrOwner && (
        <>
          <GlassCard className="p-6 border-red-500/20 bg-red-950/5 space-y-6">
            <div className="flex items-center gap-2 text-rose-400">
              <ShieldAlert size={20} />
              <h2 className="text-base font-bold">{t("settings.dangerZone")}</h2>
            </div>
            <p className="text-xs text-white/60">
              {t("settings.dangerZoneDesc")}
            </p>
            <div className="pt-2">
              <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
                {t("settings.deleteWorkspace")}
              </Button>
            </div>
          </GlassCard>
        </>
      )}

      {showDeleteModal && (
        <Modal title={t("settings.deleteWorkspaceModalTitle")} onClose={() => { setShowDeleteModal(false); setDeleteConfirmationText(""); }}>
          <div className="space-y-4">
            <p className="text-sm text-white/70">
              {t("settings.deleteWorkspaceModalDesc")}
            </p>
            <p className="text-sm text-white/70">
              {t("settings.confirmTypePre")}<span className="font-mono font-bold text-red-400 select-all">DELETE MY DATA</span>{t("settings.confirmTypePost")}
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
                {t("settings.cancel")}
              </Button>
              <Button
                variant="danger"
                disabled={deleteConfirmationText !== "DELETE MY DATA" || purging}
                onClick={handlePurge}
              >
                {purging ? t("settings.deleting") : t("settings.permanentlyDelete")}
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
