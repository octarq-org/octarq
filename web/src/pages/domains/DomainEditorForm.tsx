import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus, Domain, HostEntry, ProviderAccount } from "../../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "../Settings";
import { useTranslation } from "../../i18n";

export function DomainEditorForm({ domain, accounts, onCancel, onSaved }: { domain: Domain | null; accounts: ProviderAccount[]; onCancel: () => void; onSaved: (d?: any) => void; }) {
  const { t } = useTranslation();
  const [name, setName] = useState(domain?.name ?? "");
  const [providerAccountId, setProviderAccountId] = useState(domain?.providerAccountId || accounts[0]?.id || 0);
  const [zoneId, setZoneId] = useState(domain?.zoneId ?? "");
  const [note, setNote] = useState(domain?.note ?? "");
  const [linkHosts, setLinkHosts] = useState<HostEntry[]>(domain?.linkHosts ?? []);
  const [mailHosts, setMailHosts] = useState<HostEntry[]>(domain?.mailHosts ?? []);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const linkSubs = name ? [`go.${name}`, `s.${name}`, `link.${name}`, name] : [];
  const mailSubs = name ? [name, `mail.${name}`] : [];

  async function save() {
    setErr(""); setBusy(true);
    const payload: any = { name, providerAccountId, zoneId, note, linkHosts, mailHosts };
    try {
      let res;
      if (domain) res = await api.updateDomain(domain.id, payload);
      else res = await api.createDomain(payload);
      onSaved(res);
    } catch (e: any) { setErr(e.message ?? t("domains.saveFailed")); }
    finally { setBusy(false); }
  }

  return (
    <div className="space-y-4">
      <Field label={t("domains.domainName")}>
        <input className="input w-full font-mono" value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" disabled={!!domain} required />
      </Field>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label={t("domains.dnsProviderConnection")}>
          <select className="input w-full" value={providerAccountId} onChange={(e) => setProviderAccountId(Number(e.target.value))}>
            {accounts.map((a) => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            {accounts.length === 0 && <option value={0}>{t("domains.noAccountsAvailable")}</option>}
          </select>
        </Field>
        <Field label={t("domains.zoneIdentifier")}>
          <input className="input w-full font-mono text-xs" value={zoneId} onChange={(e) => setZoneId(e.target.value)} placeholder={t("domains.zoneIdPlaceholder")} />
        </Field>
      </div>
      <Field label={t("domains.internalAdminNote")}>
        <textarea className="input w-full" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("domains.notePlaceholder")} />
      </Field>
      {!domain && (
        <>
          <Field label={t("domains.shortLinkSubdomains")}>
            <HostList hosts={linkHosts} onChange={setLinkHosts} suggestions={linkSubs} placeholder="go.example.com" baseDomain={name || undefined} emptyText={t("domains.noShortlinkSubdomains")} />
          </Field>
          <Field label={t("domains.inboundMailSubdomains")}>
            <HostList hosts={mailHosts} onChange={setMailHosts} suggestions={mailSubs} placeholder="mail.example.com" baseDomain={name || undefined} emptyText={t("domains.noMailboxSubdomains")} />
          </Field>
        </>
      )}
      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
      <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
        {onCancel && (
          <Button type="button" variant="ghost" onClick={onCancel}>{t("domains.cancel")}</Button>
        )}
        <Button variant="primary" onClick={save} disabled={busy || !name}>{busy ? t("domains.saving") : t("domains.saveBasicInfo")}</Button>
      </div>
    </div>
  );
}

