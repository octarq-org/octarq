import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

// PluginsSettings lets an owner/admin turn Pro plugins on or off for this
// workspace. Plugins are opt-in: everything is disabled until enabled here, and
// a disabled plugin's sidebar items and API routes are both hidden.
// Descriptions keyed by feature key (backend supplies the title). Unknown keys
// fall back to just the title + its menu list.
const PLUGIN_DESC: Record<string, string> = {
  ai: "LLM-powered email summaries, sorting, and OTP extraction.",
  infra: "VPS server monitoring and the SSH credentials vault.",
  finance: "Subscription tracking and expense bookkeeping.",
  commerce: "Sell software end-to-end — storefront, license issuance, and checkout billing.",
};

export function PluginsSettings() {
  const { t } = useTranslation();
  const [plugins, setPlugins] = useState<PluginInfo[] | null>(null);
  const [err, setErr] = useState("");

  function load() {
    api.plugins().then(setPlugins).catch((e: ApiError) => setErr(e.message || t("settings.failedLoadPlugins")));
  }
  useEffect(load, []);

  async function toggle(key: string, enabled: boolean) {
    setErr("");
    // optimistic; revert on failure
    setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled } : p)) ?? prev);
    try {
      await api.updatePlugin(key, enabled);
    } catch (e) {
      setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled: !enabled } : p)) ?? prev);
      setErr(e instanceof ApiError ? e.message : t("settings.failedUpdatePlugin"));
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.pluginsTitle")} description={t("settings.pluginsDescription")} />

      {err && (
        <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
          <ShieldAlert className="h-4 w-4 shrink-0" /><span>{err}</span>
        </div>
      )}

      {plugins === null ? (
        <GlassCard className="p-6 text-sm text-white/50">{t("settings.loadingPlugins")}</GlassCard>
      ) : plugins.length === 0 ? (
        <GlassCard className="p-6 text-sm text-white/55">
          {t("settings.noPluginsPre")}<span className="text-white/80">Octarq</span>.
        </GlassCard>
      ) : (
        <div className="space-y-3">
          {plugins.map((p) => {
            const description = PLUGIN_DESC[p.key] ? t("settings.pluginDesc." + p.key) : "";
            return (
              <GlassCard key={p.key} className="p-5 flex items-center justify-between gap-4">
                <div className="flex items-start gap-3 min-w-0">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-white/[0.04] text-indigo-300">
                    <Puzzle className="h-4.5 w-4.5" />
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <h3 className="text-sm font-bold text-white">{p.title}</h3>
                      {p.enabled ? <Badge tone="green">{t("settings.badgeOn")}</Badge> : <Badge tone="neutral">{t("settings.badgeOff")}</Badge>}
                    </div>
                    {description && <p className="text-xs text-white/45 mt-0.5">{description}</p>}
                    {p.menus.length > 0 && (
                      <p className="text-[10px] text-white/50 mt-1">
                        {t("settings.pluginAdds", { items: p.menus.map((m) => m.label).join(" · ") })}
                      </p>
                    )}
                  </div>
                </div>
                <Toggle on={p.enabled} onChange={(v) => toggle(p.key, v)} />
              </GlassCard>
            );
          })}
        </div>
      )}
    </div>
  );
}

// LicenseSettings is where a customer pastes the octarq-pro key they bought. The
// backing API is the octarq-pro `licensing` plugin; in the OSS build it 404s and we
// show a neutral note instead.
