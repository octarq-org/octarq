import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useSettingsData, SavedBadge } from "./shared";

export function ProviderAccounts({ embed }: { embed?: boolean }) {
  const { t } = useTranslation();
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
    if (!confirm(t("settings.confirmRemoveProvider"))) return;
    try {
      await api.deleteProviderAccount(id);
      load();
    } catch (e: any) {
      alert(e.message || t("settings.failedRemove"));
    }
  }


  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-white/70">
          {t("settings.dnsApiAccounts")}
          <div className="text-[10px] text-white/50 font-normal mt-0.5">{t("settings.dnsApiAccountsDesc")}</div>
        </div>
        <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setCreating(true)}>
          {t("settings.addProvider")}
        </Button>
      </div>

      {loading ? (
        <div className="text-white/40 text-sm py-4 text-center">{t("settings.loadingLower")}</div>
      ) : accounts.length === 0 ? (
        <Empty>
          <Cloud className="h-8 w-8 text-white/50 mb-1" />
          <div className="text-xs text-white/50">{t("settings.noDnsProviders")}</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {accounts.map(a => (
            <div key={a.id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-semibold text-sm text-white">{a.name}</div>
                <div className="text-xs text-white/40 mt-1 flex items-center gap-1.5">
                  <Badge tone={a.type === "cloudflare" ? "indigo" : "cyan"} className="uppercase tracking-wider text-[9px]">
                    {a.type}
                  </Badge>
                  {a.hasCredentials
                    ? <Badge tone="green" className="text-[9px]"><KeyRound className="h-2.5 w-2.5 mr-0.5 inline" />{t("settings.credentialsSet")}</Badge>
                    : <Badge tone="amber" className="text-[9px]">{t("settings.noCredentials")}</Badge>}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="subtle" onClick={() => setEditing(a)} className="text-xs py-1 px-2.5">
                  {t("settings.edit")}
                </Button>
                <Button
                  variant="danger"
                  onClick={() => remove(a.id)}
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
  const { t } = useTranslation();
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
      setErr(e.message || t("settings.failedGeneric"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={account ? t("settings.editProviderAccount") : t("settings.registerProviderAccount")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("settings.providerLabelName")}>
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)} placeholder={t("settings.providerLabelPlaceholder")} required autoFocus />
        </Field>
        {!account && (
          <Field label={t("settings.dnsProviderType")}>
            <select className="input w-full text-sm" value={type} onChange={e => setType(e.target.value)}>
              {types.map(ty => <option key={ty} value={ty} className="capitalize">{ty}</option>)}
            </select>
          </Field>
        )}
        <Field label={t("settings.apiKeysCredentials")} hint={account ? t("settings.apiKeysHintExisting") : t("settings.apiKeysHintNew")}>
          <input className="input w-full font-mono text-xs" type="password" value={config} onChange={e => setConfig(e.target.value)} placeholder={account ? "••••••••" : t("settings.apiKeysPlaceholderNew")} required={!account} />
        </Field>
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("settings.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? t("settings.savingDots") : t("settings.saveConnection")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

