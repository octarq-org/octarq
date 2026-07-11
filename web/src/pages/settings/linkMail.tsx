import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useSettingsData, useInstanceSettingsData, SavedBadge } from "./shared";

export function LinkSettings() {
  const { t } = useTranslation();
  const { s: wS } = useSettingsData();
  const { s } = useInstanceSettingsData();
  const [reservedSlugs, setReservedSlugs] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => { if (s) { setReservedSlugs(s.reservedSlugs); } }, [s]);

  if (!wS?.isInstanceAdmin) return null;

  async function save() {
    setBusy(true);
    try { await api.updateInstanceSettings({ reservedSlugs }); setSaved(true); setTimeout(() => setSaved(false), 2000); }
    finally { setBusy(false); }
  }
  if (!s) return <div className="text-sm text-white/40">{t("settings.loadingLower")}</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white/90">{t("settings.shortLinksSettings")}</h2>
        <SavedBadge on={saved} />
      </div>
      <Field label={t("settings.reservedSlugsLabel")} hint={t("settings.reservedSlugsHint", { list: s.builtinReserved.join(", ") })}>
        <textarea className="input w-full font-mono text-xs" rows={3} value={reservedSlugs} onChange={(e) => setReservedSlugs(e.target.value)} placeholder="pricing&#10;login&#10;about" />
      </Field>
      <div className="border-t border-white/[0.06] pt-4 flex justify-end">
        <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>{busy ? t("settings.saving") : t("settings.saveSettings")}</Button>
      </div>
    </div>
  );
}

export function MailSettings() {
  const { t } = useTranslation();
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
  if (!s) return <div className="text-sm text-white/40">{t("settings.loadingLower")}</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white/90">{t("settings.inboundMailboxesSettings")}</h2>
        <SavedBadge on={saved} />
      </div>
      <Field label={t("settings.reservedMailboxesLabel")} hint={t("settings.reservedMailboxesHint")}>
        <textarea className="input w-full font-mono text-xs" rows={2} value={reservedMailboxes} onChange={(e) => setReservedMailboxes(e.target.value)} placeholder="admin&#10;postmaster" />
      </Field>
      <Field label={t("settings.inboundWebhookUrlLabel")} hint={t("settings.inboundWebhookUrlHint")}>
        <input
          readOnly
          className="input w-full font-mono text-xs"
          value={`${location.origin}/api/v1/webhook/${s?.orgSlug || ""}/email/inbound/${inboundToken}`}
          onFocus={(e) => e.currentTarget.select()}
        />
      </Field>
      <Field label={t("settings.inboundTokenLabel")} hint={t("settings.inboundTokenHint")}>
        <input className="input w-full font-mono text-xs" value={inboundToken} onChange={(e) => setInboundToken(e.target.value)} placeholder={t("settings.inboundTokenPlaceholder")} />
      </Field>
      <div className="flex items-center gap-3 border-t border-white/[0.04] pt-4">
        <Toggle on={catchAll} onChange={setCatchAll} />
        <div>
          <span className="block select-none text-xs font-semibold text-white/70">{t("settings.enableCatchAll")}</span>
          <span className="select-none text-[10px] text-white/40">{t("settings.enableCatchAllDesc")}</span>
        </div>
      </div>
      <div className="flex items-center gap-3 border-t border-white/[0.04] pt-4">
        <Toggle on={autoWrap} onChange={setAutoWrap} />
        <div>
          <span className="block select-none text-xs font-semibold text-white/70">{t("settings.autoWrapLinks")}</span>
          <span className="select-none text-[10px] text-white/40">{t("settings.autoWrapLinksDesc")}</span>
        </div>
      </div>
      <div className="border-t border-white/[0.06] pt-4 flex justify-end">
        <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>{busy ? t("settings.saving") : t("settings.saveSettings")}</Button>
      </div>
    </div>
  );
}


