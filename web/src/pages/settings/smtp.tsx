import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useSettingsData, SavedBadge } from "./shared";

export function SMTPSenders() {
  const { t } = useTranslation();
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
    if (!confirm(t("settings.confirmRemoveSmtp"))) return;
    try {
      await api.deleteSMTPSender(id);
      load();
    } catch (e: any) {
      alert(e.message || t("settings.failedRemove"));
    }
  }


  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-white/70">
          {t("settings.smtpOutgoingGateways")}
          <div className="text-[10px] text-white/50 font-normal mt-0.5">{t("settings.smtpOutgoingGatewaysDesc")}</div>
        </div>
        <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setCreating(true)}>
          {t("settings.addSmtp")}
        </Button>
      </div>
      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">{t("settings.loadingLower")}</div>
      ) : senders.length === 0 ? (
        <Empty>
          <Send className="h-8 w-8 text-white/50 mb-1" />
          <div className="text-xs text-white/50">{t("settings.noSmtpSenders")}</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {senders.map(s => (
            <div key={s.id} className="flex items-center justify-between p-4 group">
              <div>
                <div className="font-semibold text-sm text-white flex items-center gap-1.5">
                  {s.name}
                  {s.passSet
                    ? <Badge tone="green" className="text-[9px]"><KeyRound className="h-2.5 w-2.5 mr-0.5 inline" />{t("settings.smtpPasswordSet")}</Badge>
                    : <Badge tone="amber" className="text-[9px]">{t("settings.smtpNoPassword")}</Badge>}
                </div>
                <div className="text-xs text-white/40 mt-1 font-mono">
                  {s.fromEmail} via {s.host}:{s.port}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="subtle" onClick={() => setEditing(s)} className="text-xs py-1 px-2.5">
                  {t("settings.editSmtp")}
                </Button>
                <Button
                  variant="danger"
                  onClick={() => remove(s.id)}
                  className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  {t("settings.remove")}
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
  const { t } = useTranslation();
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
      setErr(e.message || t("settings.failedGeneric"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={sender ? t("settings.modifySmtpRelay") : t("settings.configureSmtpRelay")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("settings.senderConnectionName")}>
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)} placeholder={t("settings.senderConnectionPlaceholder")} required autoFocus />
        </Field>

        <div className="flex gap-4">
          <div className="flex-[3]">
            <Field label={t("settings.smtpHostLabel")}>
              <input className="input w-full font-mono text-xs" value={host} onChange={e => setHost(e.target.value)} placeholder="smtp.mailgun.org" required />
            </Field>
          </div>
          <div className="flex-1">
            <Field label={t("settings.smtpPortLabel")}>
              <input type="number" className="input w-full font-mono text-xs" value={port} onChange={e => setPort(e.target.value)} placeholder="587" required />
            </Field>
          </div>
        </div>

        <Field label={t("settings.smtpUsernameLabel")}>
          <input className="input w-full font-mono text-xs" value={user} onChange={e => setUser(e.target.value)} placeholder={t("settings.smtpUsernamePlaceholder")} required />
        </Field>

        <Field label={t("settings.smtpPasswordLabel")} hint={sender ? t("settings.smtpPasswordHint") : ""}>
          <input type="password" className="input w-full font-mono text-xs" value={pass} onChange={e => setPass(e.target.value)} placeholder="••••••••" required={!sender} />
        </Field>

        <Field label={t("settings.defaultFromAddress")} hint={t("settings.defaultFromHint")}>
          <input className="input w-full font-mono text-xs" value={fromEmail} onChange={e => setFromEmail(e.target.value)} placeholder="noreply@domain.com" required />
        </Field>

        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("settings.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim() || !host.trim() || !port || !user.trim()}>
            {busy ? t("settings.savingDots") : t("settings.saveRelay")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

