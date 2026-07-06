import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus, Domain, HostEntry, ProviderAccount } from "../../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "../Settings";
import { useTranslation } from "../../i18n";

export function SyncModal({ accounts, onClose, onSynced }: { accounts: ProviderAccount[]; onClose: () => void; onSynced: () => void }) {
  const { t } = useTranslation();
  const [accountId, setAccountId] = useState<number>(accounts[0]?.id || 0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ total: number; created: number; updated: number } | null>(null);

  async function run() {
    if (!accountId) return setErr(t("domains.selectProviderAccount"));
    setBusy(true); setErr("");
    try { const r = await api.syncDomains(accountId); setResult(r); }
    catch (e: any) { setErr(e.message ?? t("domains.syncFailed")); }
    finally { setBusy(false); }
  }

  return (
    <Modal title={t("domains.syncDnsZones")} onClose={onClose}>
      {result ? (
        <div className="py-4 text-center space-y-4">
          <div className="h-12 w-12 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400 mx-auto">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <div>
            <p className="text-white font-semibold">{t("domains.zonesDetected", { count: result.total })}</p>
            <p className="text-xs text-white/55 mt-1">
              {t("domains.createdPrefix")} <span className="text-emerald-400 font-bold">{result.created}</span> {t("domains.updatedMid")} <span className="text-indigo-400 font-bold">{result.updated}</span> {t("domains.recordsSuffix")}
            </p>
          </div>
          <p className="text-[11px] text-white/40 max-w-xs mx-auto">{t("domains.syncToggleHint")}</p>
          <Button variant="primary" onClick={onSynced} className="w-full">{t("domains.done")}</Button>
        </div>
      ) : accounts.length === 0 ? (
        <div className="py-4 text-center space-y-2 text-white/55">
          <p className="font-semibold">{t("domains.noProviderAccounts")}</p>
          <p className="text-xs text-white/40">{t("domains.noProviderAccountsHint")}</p>
        </div>
      ) : (
        <>
          <p className="mb-4 text-xs text-white/55 leading-relaxed">{t("domains.syncIntro")}</p>
          <Field label={t("domains.dnsProviderConnection")}>
            <select className="input w-full" value={accountId} onChange={e => setAccountId(Number(e.target.value))}>
              <option value={0}>{t("domains.selectAccount")}</option>
              {accounts.map(a => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            </select>
          </Field>
          {err && <p className="mb-4 text-sm text-rose-400 font-medium">{err}</p>}
          <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
            <Button variant="ghost" onClick={onClose}>{t("domains.cancel")}</Button>
            <Button variant="primary" onClick={run} disabled={busy || !accountId}>{busy ? t("domains.queryingApi") : t("domains.syncZones")}</Button>
          </div>
        </>
      )}
    </Modal>
  );
}

