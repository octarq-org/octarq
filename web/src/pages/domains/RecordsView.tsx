import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus, Domain, HostEntry, ProviderAccount } from "../../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "../Settings";
import { useTranslation } from "../../i18n";

const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "CAA"];

export function RecordsView({ domain }: { domain: Domain }) {
  const { t } = useTranslation();
  const [records, setRecords] = useState<DNSRecord[] | null>(null);
  const [err, setErr] = useState("");
  const [editing, setEditing] = useState<DNSRecord | "new" | "subdomain" | null>(null);
  const [typeFilter, setTypeFilter] = useState("");
  const [search, setSearch] = useState("");

  async function load() {
    setErr("");
    try { setRecords(await api.records(domain.id)); }
    catch (e: any) { setErr(e.message ?? t("domains.loadRecordsFailed")); setRecords([]); }
  }
  useEffect(() => { load(); }, [domain.id]);

  const filtered = (records ?? []).filter((r) => {
    if (typeFilter && r.type !== typeFilter) return false;
    if (search) {
      const s = search.toLowerCase();
      return r.name.toLowerCase().includes(s) || r.content.toLowerCase().includes(s) || (r.comment ?? "").toLowerCase().includes(s);
    }
    return true;
  });
  const presentTypes = Array.from(new Set((records ?? []).map((r) => r.type)));

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <select className="input min-w-[120px] text-xs py-1" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
          <option value="">{t("domains.allTypes")}</option>
          {presentTypes.map((rt) => <option key={rt} value={rt}>{rt}</option>)}
        </select>
        <input className="input flex-1 min-w-[140px] text-xs py-1" placeholder={t("domains.filterPlaceholder")} value={search} onChange={(e) => setSearch(e.target.value)} />
        <Button variant="subtle" onClick={() => setEditing("subdomain")} className="py-1 px-3 text-xs">{t("domains.presetButton")}</Button>
        <Button variant="primary" onClick={() => setEditing("new")} className="py-1 px-3 text-xs">{t("domains.customButton")}</Button>
      </div>

      <p className="text-[11px] text-white/50">{t("domains.recordsNote", { shown: filtered.length, total: records?.length ?? 0 })}</p>
      
      {err && <p className="rounded bg-rose-500/10 p-3 text-xs text-rose-400 font-medium">{err}</p>}
      
      {records === null ? (
        <p className="text-white/40 p-6 text-center text-xs">{t("domains.loadingRecords")}</p>
      ) : filtered.length === 0 ? (
        <p className="text-white/40 p-6 text-center text-xs">{t("domains.noRecordsMatching")}</p>
      ) : (
        <div className="bg-black/20 rounded-2xl border border-white/[0.05] overflow-hidden">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-white/40 border-b border-white/[0.06] bg-white/[0.01]">
                <th className="py-2.5 pl-4 font-semibold uppercase tracking-wider">{t("domains.thType")}</th>
                <th className="py-2.5 font-semibold uppercase tracking-wider">{t("domains.thName")}</th>
                <th className="py-2.5 font-semibold uppercase tracking-wider">{t("domains.thContent")}</th>
                <th className="py-2.5 font-semibold uppercase tracking-wider">{t("domains.thNote")}</th>
                <th className="py-2.5 pr-4 text-right" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {filtered.map((r) => (
                <tr key={r.id} className="hover:bg-white/[0.01] transition-colors">
                  <td className="py-3 pl-4 font-semibold text-white/80">
                    <span className="inline-flex items-center gap-1.5">
                      <span className="font-mono text-white/85 bg-white/5 px-2 py-0.5 rounded-lg border border-white/[0.04]">{r.type}</span>
                      {r.proxied && (
                        <span title={t("domains.cloudflareProxied")}>
                          <Cloud className="h-3 w-3 text-amber-500 fill-amber-500/10" />
                        </span>
                      )}
                    </span>
                  </td>
                  <td className="max-w-[120px] truncate font-mono text-white/80">{r.name}</td>
                  <td className="max-w-[180px] truncate text-white/55 font-mono">{r.content}</td>
                  <td className="max-w-[120px] truncate text-indigo-300/80">{r.comment}</td>
                  <td className="py-3 pr-4 text-right">
                    <div className="flex gap-1.5 justify-end">
                      <Button variant="ghost" className="px-2 py-0.5 text-[10px]" onClick={() => setEditing(r)}>{t("domains.edit")}</Button>
                      <Button variant="danger" className="px-2 py-0.5 text-[10px] text-rose-400 bg-rose-500/0 hover:bg-rose-500/10 border-0" onClick={async () => { if (confirm(t("domains.deleteRecordConfirm", { type: r.type, name: r.name }))) { await api.deleteRecord(domain.id, r.id); load(); } }}>{t("domains.del")}</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && (
        <RecordEditor
          domainId={domain.id} domainName={domain.name}
          linkHost={domain.linkHosts?.[0]?.host ?? ""}
          record={typeof editing === "string" ? null : editing}
          subdomain={editing === "subdomain"}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function RecordEditor({ domainId, domainName, linkHost, record, subdomain, onClose, onSaved }: { domainId: number; domainName: string; linkHost?: string; record: DNSRecord | null; subdomain?: boolean; onClose: () => void; onSaved: () => void; }) {
  const { t } = useTranslation();
  const [type, setType] = useState(record?.type ?? "A");
  const [name, setName] = useState(record?.name ?? "");
  const [content, setContent] = useState(record?.content ?? "");
  const [comment, setComment] = useState(record?.comment ?? "");
  const [proxied, setProxied] = useState(record?.proxied ?? false);
  const [priority, setPriority] = useState<number>(record?.priority ?? 10);
  const [err, setErr] = useState("");

  const needsPriority = ["MX", "SRV", "URI"].includes(type.toUpperCase());
  const canProxy = ["A", "AAAA", "CNAME"].includes(type.toUpperCase());
  const contentHint: Record<string, string> = { A: t("domains.hintIpv4"), AAAA: t("domains.hintIpv6"), CNAME: t("domains.hintCname"), TXT: t("domains.hintTxt"), MX: t("domains.hintMx"), NS: t("domains.hintNs"), CAA: '0 issue "letsencrypt.org"' };

  const linkSub = linkHost && linkHost.endsWith("." + domainName) ? linkHost.slice(0, -(domainName.length + 1)) : linkHost === domainName ? "@" : "go";

  function preset(kind: "link" | "mail") {
    if (kind === "link") { setType("CNAME"); setName(name || linkSub); setContent(domainName); setComment("octarq short-link host"); setProxied(true); }
    else { setType("MX"); setName(name || "mail"); setContent("route1.mx.cloudflare.net"); setComment("octarq mailbox (Cloudflare Email Routing)"); setProxied(false); setPriority(10); }
  }

  async function save() {
    setErr("");
    const payload: Partial<DNSRecord> = { type, name, content, comment, proxied: canProxy ? proxied : false, ttl: 1 };
    if (needsPriority) payload.priority = Number(priority);
    try {
      if (record) await api.updateRecord(domainId, record.id, payload);
      else await api.createRecord(domainId, payload);
      onSaved();
    } catch (e: any) { setErr(e.message ?? t("domains.saveFailed")); }
  }

  return (
    <Modal title={record ? t("domains.modifyRecord") : subdomain ? t("domains.presetConfigurator") : t("domains.createRecord")} onClose={onClose}>
      {subdomain && (
        <div className="mb-4 flex gap-2.5">
          <Button variant="subtle" className="flex-1 py-1.5 text-xs gap-1.5" onClick={() => preset("link")}>
            <LinkIcon className="h-3.5 w-3.5 text-indigo-400" />
            {t("domains.setLinkCname")}
          </Button>
          <Button variant="subtle" className="flex-1 py-1.5 text-xs gap-1.5" onClick={() => preset("mail")}>
            <Mail className="h-3.5 w-3.5 text-emerald-400" />
            {t("domains.setMxRecords")}
          </Button>
        </div>
      )}
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <Field label={t("domains.recordType")}>
            <select className="input w-full" value={type} onChange={(e) => setType(e.target.value)}>
              {RECORD_TYPES.map((rt) => <option key={rt}>{rt}</option>)}
            </select>
          </Field>
          <Field label={t("domains.nameHost")}>
            <input className="input w-full font-mono" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("domains.namePlaceholder")} />
          </Field>
        </div>
        
        <div className={needsPriority ? "grid grid-cols-[1fr_120px] gap-4" : ""}>
          <Field label={t("domains.targetValue")} hint={contentHint[type.toUpperCase()]}>
            <input className="input w-full font-mono text-xs" value={content} onChange={(e) => setContent(e.target.value)} required />
          </Field>
          {needsPriority && (
            <Field label={t("domains.priority")}>
              <input type="number" min={0} className="input w-full" value={priority} onChange={(e) => setPriority(Number(e.target.value))} />
            </Field>
          )}
        </div>

        <Field label={t("domains.metadataComment")}>
          <input className="input w-full text-xs" value={comment} onChange={(e) => setComment(e.target.value)} placeholder={t("domains.commentPlaceholder")} />
        </Field>

        {canProxy && (
          <div className="flex items-center gap-2 pt-1">
            <Toggle on={proxied} onChange={setProxied} />
            <span className="text-xs text-white/60 select-none">{t("domains.proxiedLabel")}</span>
          </div>
        )}

        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button variant="ghost" onClick={onClose}>{t("domains.cancel")}</Button>
          <Button variant="primary" onClick={save}>{t("domains.saveRecord")}</Button>
        </div>
      </div>
    </Modal>
  );
}
