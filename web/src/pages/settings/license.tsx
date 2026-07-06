import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

export function LicenseSettings() {
  const { t } = useTranslation();
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
          ? t("settings.savedEnvOverride", { tier: r.tier })
          : t("settings.savedLicense", { tier: r.tier, email: r.email }),
      });
      load();
    } catch (e) {
      setMsg({ kind: "err", text: (e as ApiError).message });
    } finally {
      setBusy(false);
    }
  }

  async function deactivate() {
    if (!confirm(t("settings.confirmRemoveLicense"))) return;
    setBusy(true);
    setMsg(null);
    try {
      await api.deactivateLicense();
      setMsg({ kind: "ok", text: t("settings.licenseRemoved") });
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
        <PageHeader title={t("settings.licenseTitle")} description={t("settings.licenseDescManage")} />
        <GlassCard className="p-6 text-sm text-white/55">
          {t("settings.ossNotePre")}<span className="text-white/80">Octarq</span>.{" "}
          <a className="text-indigo-300 hover:underline" href="https://octarq.com/pricing/" target="_blank" rel="noreferrer">
            {t("settings.seePlans")}
          </a>
        </GlassCard>
      </div>
    );
  }

  return (
    <div>
      <PageHeader title={t("settings.licenseTitle")} description={t("settings.licenseDescActivate")} />

      {status && (
        <GlassCard className="mb-4 p-5">
          <div className="flex items-center gap-3">
            <KeyRound className="h-5 w-5 text-indigo-300" />
            {status.licensed ? (
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone="green">{(status.tier || "").toUpperCase()}</Badge>
                <span className="text-sm text-white/70">{status.email}</span>
                <span className="text-xs text-white/40">
                  {status.expiresAt ? t("settings.expiresOn", { date: status.expiresAt.slice(0, 10) }) : t("settings.neverExpires")} · {t("settings.fromSource", { source: status.source })}
                </span>
              </div>
            ) : (
              <span className="text-sm text-white/60">{t("settings.noActiveLicense")}</span>
            )}
          </div>
          {status.envOverride && (
            <p className="mt-3 text-xs text-amber-300/90">
              {t("settings.envVarNotePre")}<code>LED_PRO_LICENSE</code>{t("settings.envVarNotePost")}
            </p>
          )}
        </GlassCard>
      )}

      <GlassCard className="p-5">
        <Field label={t("settings.licenseKeyLabel")} hint={t("settings.licenseKeyHint")}>
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
            {busy ? t("settings.saving") : t("settings.activate")}
          </Button>
          {status?.licensed && status.source === "file" && (
            <Button variant="danger" onClick={deactivate} disabled={busy}>
              {t("settings.removeLicense")}
            </Button>
          )}
        </div>
        {msg && (
          <p className={`mt-3 text-sm ${msg.kind === "ok" ? "text-emerald-300" : "text-rose-300"}`}>{msg.text}</p>
        )}
        <p className="mt-4 text-xs text-white/35">
          {t("settings.licenseRestartNote")}
        </p>
      </GlassCard>
    </div>
  );
}

